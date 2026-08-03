// Command hub is the home automation hub entrypoint. It wires the protocol
// adapters around an in-process event bus and serves HomeKit.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sh5080/home-hub/internal/automation"
	"github.com/sh5080/home-hub/internal/bus"
	"github.com/sh5080/home-hub/internal/config"
	"github.com/sh5080/home-hub/internal/domain"
	"github.com/sh5080/home-hub/internal/driver"
	"github.com/sh5080/home-hub/internal/health"
	"github.com/sh5080/home-hub/internal/homekit"
	"github.com/sh5080/home-hub/internal/matter"
	"github.com/sh5080/home-hub/internal/mqtt"
	"github.com/sh5080/home-hub/internal/registry"
	"github.com/sh5080/home-hub/internal/rf"
	"github.com/sh5080/home-hub/internal/zigbee"
	"github.com/sh5080/home-hub/internal/zigbee/ezsp"
)

// runnable is anything with a blocking Start bound to a context.
type runnable interface {
	Start(ctx context.Context) error
}

// applyMatter maps a domain command onto a Matter driver. All values are in
// domain orientation (position 0 = closed, 100 = open); the driver translates
// to Matter's inverted lift percent internally.
func applyMatter(d matter.Driver, c domain.Command) error {
	switch c.Action {
	case domain.ActionSetOn:
		if on, _ := c.Value.(bool); on {
			return d.Open()
		}
		return d.Close()
	case domain.ActionSetPosition:
		p, _ := c.Value.(int)
		switch {
		case p >= 100:
			return d.Open()
		case p <= 0:
			return d.Close()
		default:
			return d.SetLiftPercent(p)
		}
	default:
		return nil
	}
}

// gmConfig converts a device's config block into a matter.GoMatterConfig.
func gmConfig(dc config.DeviceConfig) matter.GoMatterConfig {
	return matter.GoMatterConfig{
		FabricStore: dc.GoMatter.FabricStore,
		NodeID:      dc.GoMatter.NodeID,
		Address:     dc.GoMatter.Address,
		Endpoint:    dc.GoMatter.Endpoint,
	}
}

func main() {
	cfgPath := flag.String("config", "configs/devices.yaml", "path to config file")
	healthAddr := flag.String("health", ":8086", "health endpoint listen address")
	logLevel := flag.String("log", "info", "log level: debug | info | warn | error")
	flag.Parse()

	// debug is what makes protocol bring-up legible: raw ASH/EZSP frames and
	// every NCP callback, including ones the adapter does not model yet.
	level := slog.LevelInfo
	switch *logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Error("load config", "err", err)
		os.Exit(1)
	}

	b := bus.New(64)
	reg := registry.New()
	for _, dc := range cfg.Devices {
		reg.Add(dc.Device)
	}
	log.Info("registry loaded", "devices", len(reg.List()))

	// Delegated Matter devices are owned by HomeKit directly: exclude them from
	// the HAP bridge (they would appear twice) and expose only their triggers.
	// Natively-controlled (go-matter) devices are published like any other.
	var delegated []string
	for _, dc := range cfg.Devices {
		if dc.Integration == domain.Matter && dc.Driver != "go-matter" {
			delegated = append(delegated, dc.ID)
		}
	}

	// Protocol adapters.
	// Zigbee backend: zstack (P dongle, default) or ezsp (E dongle). Both
	// implement driver.Driver and report integration "zigbee", so the rest of
	// the wiring is identical.
	var zb driver.Driver
	switch cfg.Zigbee.Backend {
	case "ezsp":
		zb = ezsp.New(ezsp.Config{
			Port:       cfg.Zigbee.Port,
			Storage:    cfg.Zigbee.Storage,
			PermitJoin: cfg.Zigbee.PermitJoin,
			Channel:    cfg.Zigbee.Channel,
			TxPower:    cfg.Zigbee.TxPower,
			ForceForm:  cfg.Zigbee.ForceForm,
		}, b, reg, log)
		log.Info("zigbee backend: ezsp (ZBDongle-E)")
	default:
		zb = zigbee.New(zigbee.Config{
			Port:       cfg.Zigbee.Port,
			Storage:    cfg.Zigbee.Storage,
			PermitJoin: cfg.Zigbee.PermitJoin,
		}, b, reg, log)
	}
	mq := mqtt.New(cfg.MQTT.Listen, b, reg, log)
	// RF devices transmit through the ESP32+CC1101 bridge, which is an MQTT
	// client of the embedded broker — the rf adapter publishes through mq.
	var rfDevs []rf.Device
	for _, dc := range cfg.Devices {
		if dc.Integration != domain.RF {
			continue
		}
		dev := rf.Device{ID: dc.ID, Type: dc.Type, Base: dc.Addr}
		if dc.RF != nil {
			if dc.RF.Channel != nil {
				dev.Channel = *dc.RF.Channel
			}
			if dc.RF.Buttons != nil {
				dev.Buttons = rf.FanButtons{Off: dc.RF.Buttons.Off, Speeds: dc.RF.Buttons.Speeds}
			}
		}
		rfDevs = append(rfDevs, dev)
	}
	rfd := rf.New(rfDevs, mq, b, log)
	hk := homekit.New(homekit.Config{
		Name:    cfg.HomeKit.Name,
		Pin:     cfg.HomeKit.Pin,
		Addr:    ":" + cfg.HomeKit.Port,
		Storage: cfg.HomeKit.Storage,
		Exclude: delegated,
	}, b, reg, log)

	// Matter devices: natively controlled through a reconnecting go-matter
	// driver (dials lazily, re-dials when the device drops), or delegated to
	// HomeKit via virtual trigger switches.
	matterReg := matter.NewRegistry()
	var matterDrivers []*matter.ReconnectingDriver
	var gmConfigs = map[string]matter.GoMatterConfig{} // native devices, for subscriptions
	for _, dc := range cfg.Devices {
		if dc.Integration != domain.Matter {
			continue
		}
		if dc.Driver == "go-matter" {
			gmc := gmConfig(dc)
			rd := matter.NewReconnecting(gmc, log)
			matterReg.Set(dc.ID, rd)
			matterDrivers = append(matterDrivers, rd)
			gmConfigs[dc.ID] = gmc
			log.Info("matter device natively controlled", "id", dc.ID, "node", gmc.NodeID)
			continue
		}
		pressOpen := hk.RegisterTrigger(dc.Triggers["open"])
		pressClose := hk.RegisterTrigger(dc.Triggers["close"])
		matterReg.Set(dc.ID, matter.NewDelegated(pressOpen, pressClose, log))
		log.Info("matter device delegated to homekit", "id", dc.ID)
	}
	log.Info("matter devices registered", "count", len(matterReg.IDs()))

	// Command routing: integration -> the adapter that owns it.
	owners := map[domain.Integration]driver.Driver{
		domain.Zigbee: zb,
		domain.MQTT:   mq,
		domain.RF:     rfd,
	}
	owner := func(id string) driver.Driver {
		d, ok := reg.Get(id)
		if !ok {
			return nil
		}
		return owners[d.Integration]
	}

	auto := automation.New(b, log)
	for _, r := range cfg.Rules {
		switch r.Type {
		case "mirror":
			auto.Add(automation.MirrorRule(r.Src, r.Dst))
		case "button":
			auto.Add(automation.ButtonRule(r.Src, r.Press, r.Dst, r.Action, r.Value, reg.State))
		case "cycle":
			auto.Add(automation.CycleRule(r.Src, r.Press, r.Dst, r.States, r.Power))
		case "threshold":
			auto.Add(automation.ThresholdRule(r.Src, r.Dst, *r.Above, *r.Below))
		}
	}
	log.Info("automation rules loaded", "count", len(cfg.Rules))
	hz := health.New(*healthAddr, log)

	// Poll native Matter devices so external state changes reach HomeKit even
	// when the device rejects subscriptions.
	matterPoller := matter.NewPoller(matterReg, b.PublishEvent, 30*time.Second, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Watch native devices for push state updates on dedicated sessions. Each
	// watcher re-subscribes with backoff when its stream drops.
	for id, gmc := range gmConfigs {
		go matter.WatchGoMatter(ctx, gmc, id, b.PublishEvent, log)
	}

	// Dispatch bus commands to the owning adapter.
	go func() {
		cmds := b.SubscribeCommands()
		for {
			select {
			case <-ctx.Done():
				return
			case c := <-cmds:
				if d := owner(c.DeviceID); d != nil {
					if err := d.Apply(c); err != nil {
						log.Error("apply", "device", c.DeviceID, "err", err)
					}
					continue
				}
				// Matter devices have no protocol adapter; route to their driver.
				if md, ok := matterReg.Get(c.DeviceID); ok {
					if err := applyMatter(md, c); err != nil {
						log.Error("matter apply", "device", c.DeviceID, "err", err)
					}
				}
			}
		}
	}()

	// Reflect device events into the state cache, HomeKit, and (via the
	// automation engine's own subscription) the rules.
	go func() {
		events := b.SubscribeEvents()
		for {
			select {
			case <-ctx.Done():
				return
			case e := <-events:
				reg.SetState(e.DeviceID, e.State)
				hk.OnEvent(e)
			}
		}
	}()

	// Start long-running components.
	var wg sync.WaitGroup
	for _, r := range []runnable{zb, mq, rfd, hk, auto, hz, matterPoller} {
		wg.Add(1)
		go func(r runnable) {
			defer wg.Done()
			if err := r.Start(ctx); err != nil && ctx.Err() == nil {
				log.Error("component stopped", "err", err)
			}
		}(r)
	}
	log.Info("home hub started")
	wg.Wait()
	for _, md := range matterDrivers {
		if err := md.Shutdown(); err != nil {
			log.Error("close matter session", "err", err)
		}
	}
	log.Info("home hub stopped")
}
