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
	"github.com/sh5080/home-hub/internal/zigbee"
)

// runnable is anything with a blocking Start bound to a context.
type runnable interface {
	Start(ctx context.Context) error
}

// applyMatter maps a domain command onto a Matter driver.
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

func main() {
	cfgPath := flag.String("config", "configs/devices.yaml", "path to config file")
	healthAddr := flag.String("health", ":8086", "health endpoint listen address")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

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

	// Protocol adapters.
	zb := zigbee.New(cfg.Zigbee.Port, b, reg, log)
	mq := mqtt.New(cfg.MQTT.Listen, b, log)
	hk := homekit.New(homekit.Config{
		Name:    cfg.HomeKit.Name,
		Pin:     cfg.HomeKit.Pin,
		Addr:    ":" + cfg.HomeKit.Port,
		Storage: cfg.HomeKit.Storage,
	}, b, reg, log)

	// Matter devices are either controlled natively by the hub (driver:
	// go-matter, over a CASE session) or delegated to HomeKit via virtual
	// trigger switches. Both are tracked in a registry so automations/HomeKit
	// can target them uniformly.
	matterReg := matter.NewRegistry()
	var gmDrivers []*matter.GoMatterDriver
	for _, dc := range cfg.Devices {
		if dc.Integration != domain.Matter {
			continue
		}
		if dc.Driver == "go-matter" && dc.GoMatter != nil {
			// Bound the commissioning-time dial so a missing device cannot stall startup.
			dialCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			gm, err := matter.DialGoMatter(dialCtx, matter.GoMatterConfig{
				FabricStore: dc.GoMatter.FabricStore,
				NodeID:      dc.GoMatter.NodeID,
				Address:     dc.GoMatter.Address,
				Endpoint:    dc.GoMatter.Endpoint,
			})
			cancel()
			if err == nil {
				matterReg.Set(dc.ID, gm)
				gmDrivers = append(gmDrivers, gm)
				log.Info("matter device natively controlled", "id", dc.ID, "addr", dc.GoMatter.Address)
				continue
			}
			// Fall back to delegation so a single unreachable device does not take
			// the hub down; it can still be driven through HomeKit triggers.
			log.Error("go-matter dial failed; falling back to delegated", "id", dc.ID, "err", err)
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
		if r.Type == "mirror" {
			auto.Add(automation.MirrorRule(r.Src, r.Dst))
		}
	}
	log.Info("automation rules loaded", "count", len(cfg.Rules))
	hz := health.New(*healthAddr, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	// Reflect device events into HomeKit.
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
	for _, r := range []runnable{zb, mq, hk, auto, hz} {
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
	for _, gm := range gmDrivers {
		if err := gm.Shutdown(); err != nil {
			log.Error("close matter session", "err", err)
		}
	}
	log.Info("home hub stopped")
}
