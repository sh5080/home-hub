// Package rf drives remote-controlled devices behind an ESP32+CC1101 447 MHz
// bridge (the rf-remote-analyzer rf_bridge firmware). The bridge replays the
// blind remote's captured codebook and generates the fan remote's counted
// frames on its own; this adapter only translates domain commands onto the
// bridge's MQTT topics, served by the hub's embedded broker:
//
//	<base>/blind/<ch>/set    OPEN | CLOSE | STOP
//	<base>/fan/<n>/press     press fan-remote button n (1..15)
//
// where <base> is the firmware's DEVICE_ID (config addr, e.g. "rf447").
//
// Everything here is transmit-only, exactly like the original remotes: the
// receivers never report back, so after each transmission the driver publishes
// an optimistic state event to keep HomeKit settled instead of spinning.
//
// Transmissions are paced, not fired as they arrive. A blind full-travel
// command keeps the bridge's radio busy for ~3.6 s and the firmware asks for
// 4 s between commands, while HomeKit happily emits a burst when a slider is
// dragged. Commands therefore go through a queue that keeps only the LATEST
// pending command per device and enforces a cooldown after each send.
package rf

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sh5080/home-hub/internal/bus"
	"github.com/sh5080/home-hub/internal/domain"
)

// Publisher sends a message to devices; satisfied by the mqtt driver, whose
// embedded broker the ESP32 bridge is connected to.
type Publisher interface {
	Publish(topic string, payload []byte, retain bool) error
}

// FanButtons maps fan functions onto the remote's button numbers (1..15).
// The buttons' meanings are not derivable from the protocol — identify them by
// pressing (bridge serial console `fan N`, or these MQTT topics) and watching
// the fan, then record the numbers in config.
type FanButtons struct {
	Off    int         // button that turns the fan off
	Speeds map[int]int // level percent -> button (e.g. 33: btn for low speed)
}

// Device is one remote-controlled device behind the bridge.
type Device struct {
	ID      string
	Type    domain.DeviceType // TypeFan or TypeCover
	Base    string            // bridge base topic = firmware DEVICE_ID
	Channel int               // cover only: blind channel on the bridge (0..15)
	Buttons FanButtons        // fan only
}

// Cooldown after each transmission kind: how long the bridge is busy on air
// plus the spacing its firmware asks for between commands.
const (
	coolFan       = time.Second             // 8 repeats × ~41 ms ≈ 0.33 s on air
	coolBlindStop = 1500 * time.Millisecond // 6 repeats × 116 ms
	coolBlindHold = 4 * time.Second         // 31 repeats × 116 ms ≈ 3.6 s
)

// tx is one queued transmission.
type tx struct {
	topic    string
	payload  string
	event    domain.Event  // optimistic state, published after a successful send
	cooldown time.Duration // pause before the next transmission may start
}

// Driver implements driver.Driver for RF-bridged devices.
type Driver struct {
	devices map[string]Device
	pub     Publisher
	bus     *bus.Bus
	log     *slog.Logger

	mu      sync.Mutex
	pending map[string]tx // latest command per device id
	order   []string      // FIFO of device ids with a pending tx
	wake    chan struct{}

	pace func(ctx context.Context, d time.Duration) // cooldown sleep; swapped in tests
}

// New builds an RF driver for the given devices, transmitting through pub.
func New(devices []Device, pub Publisher, b *bus.Bus, log *slog.Logger) *Driver {
	m := make(map[string]Device, len(devices))
	for _, d := range devices {
		m[d.ID] = d
	}
	return &Driver{
		devices: m,
		pub:     pub,
		bus:     b,
		log:     log,
		pending: make(map[string]tx),
		wake:    make(chan struct{}, 1),
		pace:    waitFor,
	}
}

// Name identifies the adapter.
func (d *Driver) Name() string { return "rf" }

// Apply translates cmd into a bridge transmission and queues it. The device's
// previous still-pending command, if any, is superseded — for a transmit-only
// radio only the latest intent matters.
func (d *Driver) Apply(cmd domain.Command) error {
	dev, ok := d.devices[cmd.DeviceID]
	if !ok {
		return fmt.Errorf("rf: unknown device %s", cmd.DeviceID)
	}
	var t tx
	switch dev.Type {
	case domain.TypeCover:
		t = coverTx(dev, cmd)
	case domain.TypeFan:
		t = fanTx(dev, cmd)
	default:
		return fmt.Errorf("rf: device %s has unsupported type %q", dev.ID, dev.Type)
	}
	if t.topic == "" {
		return nil // action this device cannot express; ignore
	}
	d.enqueue(cmd.DeviceID, t)
	return nil
}

// Start runs the transmit queue until ctx is cancelled.
func (d *Driver) Start(ctx context.Context) error {
	d.log.Info("rf bridge adapter started", "devices", len(d.devices))
	for {
		t, ok := d.pop()
		if !ok {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-d.wake:
				continue
			}
		}
		d.log.Info("rf transmit", "topic", t.topic, "payload", t.payload)
		if err := d.pub.Publish(t.topic, []byte(t.payload), false); err != nil {
			d.log.Error("rf publish", "topic", t.topic, "err", err)
			continue // nothing went on air; keep the old optimistic state
		}
		d.bus.PublishEvent(t.event)
		d.pace(ctx, t.cooldown)
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func (d *Driver) enqueue(id string, t tx) {
	d.mu.Lock()
	if _, queued := d.pending[id]; !queued {
		d.order = append(d.order, id)
	}
	d.pending[id] = t
	d.mu.Unlock()
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

func (d *Driver) pop() (tx, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.order) == 0 {
		return tx{}, false
	}
	id := d.order[0]
	d.order = d.order[1:]
	t := d.pending[id]
	delete(d.pending, id)
	return t, true
}

func waitFor(ctx context.Context, dur time.Duration) {
	if dur <= 0 {
		return
	}
	t := time.NewTimer(dur)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// coverTx maps a command onto the blind's OPEN/CLOSE/STOP set-topic. The ends
// of the position scale run a full travel; anything in between sends STOP —
// the remote has no position control, so dragging the slider mid-way means
// "stop where you are", and the dragged value is echoed back as the optimistic
// position so the Home app settles there.
func coverTx(dev Device, cmd domain.Command) tx {
	topic := fmt.Sprintf("%s/blind/%d/set", dev.Base, dev.Channel)
	at := func(p int) domain.Event { return stateEvent(dev.ID, domain.State{Position: &p}) }
	switch cmd.Action {
	case domain.ActionSetPosition:
		p, _ := cmd.Value.(int)
		switch {
		case p >= 100:
			return tx{topic, "OPEN", at(100), coolBlindHold}
		case p <= 0:
			return tx{topic, "CLOSE", at(0), coolBlindHold}
		default:
			return tx{topic, "STOP", at(p), coolBlindStop}
		}
	case domain.ActionSetOn: // from rules (open/close actions)
		if on, _ := cmd.Value.(bool); on {
			return tx{topic, "OPEN", at(100), coolBlindHold}
		}
		return tx{topic, "CLOSE", at(0), coolBlindHold}
	}
	return tx{}
}

// fanTx maps a command onto a fan-remote button press. Levels snap to the
// nearest configured speed button; a bare "on" presses the lowest speed.
func fanTx(dev Device, cmd domain.Command) tx {
	press := func(btn int) string { return fmt.Sprintf("%s/fan/%d/press", dev.Base, btn) }
	off := func() tx {
		f, zero := false, 0
		e := stateEvent(dev.ID, domain.State{On: &f, Level: &zero})
		return tx{press(dev.Buttons.Off), "PRESS", e, coolFan}
	}
	at := func(level, btn int) tx {
		on := true
		e := stateEvent(dev.ID, domain.State{On: &on, Level: &level})
		return tx{press(btn), "PRESS", e, coolFan}
	}
	switch cmd.Action {
	case domain.ActionSetOn:
		if on, _ := cmd.Value.(bool); !on {
			return off()
		}
		return at(lowestSpeed(dev.Buttons.Speeds))
	case domain.ActionSetLevel:
		v, _ := cmd.Value.(int)
		if v <= 0 {
			return off()
		}
		return at(nearestSpeed(dev.Buttons.Speeds, v))
	}
	return tx{}
}

func stateEvent(id string, s domain.State) domain.Event {
	return domain.Event{DeviceID: id, Kind: domain.EventStateChanged, State: s}
}

func lowestSpeed(speeds map[int]int) (level, btn int) {
	for l, b := range speeds {
		if level == 0 || l < level {
			level, btn = l, b
		}
	}
	return level, btn
}

// nearestSpeed picks the configured level closest to want (ties go up).
func nearestSpeed(speeds map[int]int, want int) (level, btn int) {
	best := -1
	for l, b := range speeds {
		d := l - want
		if d < 0 {
			d = -d
		}
		if best == -1 || d < best || (d == best && l > level) {
			best, level, btn = d, l, b
		}
	}
	return level, btn
}
