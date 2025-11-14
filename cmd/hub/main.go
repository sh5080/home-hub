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

	"github.com/sh5080/home-hub/internal/automation"
	"github.com/sh5080/home-hub/internal/bus"
	"github.com/sh5080/home-hub/internal/config"
	"github.com/sh5080/home-hub/internal/domain"
	"github.com/sh5080/home-hub/internal/driver"
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

func main() {
	cfgPath := flag.String("config", "configs/devices.yaml", "path to config file")
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
	zb := zigbee.New(cfg.Zigbee.Port, b, log)
	mq := mqtt.New(cfg.MQTT.Listen, b, log)
	hk := homekit.New(b, reg, log)

	// Matter devices are delegated to HomeKit via virtual trigger switches.
	for _, dc := range cfg.Devices {
		if dc.Integration != domain.Matter {
			continue
		}
		pressOpen := hk.RegisterTrigger(dc.Triggers["open"])
		pressClose := hk.RegisterTrigger(dc.Triggers["close"])
		_ = matter.NewDelegated(pressOpen, pressClose, log)
		// TODO: register the delegated driver so automations can target it.
		log.Info("matter device delegated to homekit", "id", dc.ID)
	}

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
				hk.OnEvent(e)
			}
		}
	}()

	// Start long-running components.
	var wg sync.WaitGroup
	for _, r := range []runnable{zb, mq, hk, auto} {
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
	log.Info("home hub stopped")
}
