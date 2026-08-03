package ezsp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sh5080/home-hub/internal/bus"
	"github.com/sh5080/home-hub/internal/domain"
	"github.com/sh5080/home-hub/internal/registry"
	"github.com/sh5080/home-hub/internal/zigbee/ezsp/ash"
)

// requestTimeout bounds a command/response round trip. EZSP replies in
// milliseconds; a timeout here means the NCP is wedged or we sent a frame it
// could not parse, and failing fast is more useful than hanging the adapter.
const requestTimeout = 5 * time.Second

// networkUpTimeout bounds how long we wait for the stack to report the network
// up after formNetwork is accepted. Forming involves an energy scan, so it is
// far slower than a command round trip.
const networkUpTimeout = 30 * time.Second

// Config configures the EZSP ("E" dongle) Zigbee backend. It mirrors the
// zstack backend's config so the two are interchangeable via config selection.
type Config struct {
	Port       string
	Storage    string // network/key persistence (network form on first run)
	PermitJoin bool
	Channel    uint8 // 802.15.4 channel for a NEW network; 0 = default
	TxPower    uint8 // coordinator radio power in dBm; 0 = default
	// ForceForm discards any network already in the dongle and forms a fresh
	// one. DESTRUCTIVE: every paired device is orphaned and must re-join. It
	// is the only way to change channel, so it is a deliberate config switch
	// rather than something inferred from a changed channel value.
	ForceForm bool
}

// pending is the in-flight command awaiting its response. EZSP multiplexes
// responses and unsolicited callbacks onto one stream, so the reader has to
// know which frame belongs to whom.
type pending struct {
	seq uint8
	id  uint16
	ch  chan Frame
}

// Driver is the EZSP Zigbee adapter. It implements driver.Driver alongside the
// zstack driver; the server picks one via config `zigbee.backend`.
//
// STATUS: M1 — extended frame format and the response/callback demultiplexer.
// The NCP's network state is read but not yet created or joined.
// TODO (next milestones): M2 networkInit/formNetwork with persisted key and
// addEndpoint; M3 permitJoining + trustCenterJoinHandler and the Aqara
// decoupled-mode write; M4 sendUnicast of ZCL on/off + window covering; M5
// decode incomingMessageHandler into bus events, mirroring the zstack driver.
type Driver struct {
	cfg  Config
	bus  *bus.Bus
	reg  *registry.Registry
	log  *slog.Logger
	conn *ash.Conn
	seq  uint8

	// reqMu serializes command/response round trips. EZSP tolerates only a
	// shallow command queue and the NCP echoes our sequence number, so one
	// outstanding request keeps the demultiplexer unambiguous.
	reqMu sync.Mutex

	mu      sync.Mutex
	pending *pending

	// stackStatus carries EMBER_NETWORK_UP/DOWN from the NCP's unsolicited
	// stackStatusHandler. Buffered so a status arriving while nobody waits —
	// the usual case once the network is up — is not lost mid-callback.
	stackStatus chan uint8
}

// New builds an EZSP Zigbee driver.
func New(cfg Config, b *bus.Bus, reg *registry.Registry, log *slog.Logger) *Driver {
	return &Driver{cfg: cfg, bus: b, reg: reg, log: log, stackStatus: make(chan uint8, 4)}
}

// Name identifies the adapter. It reports "zigbee" so command routing (which
// keys on the domain integration) is identical to the zstack backend.
func (d *Driver) Name() string { return "zigbee" }

// nextSeq returns the next EZSP frame sequence number.
func (d *Driver) nextSeq() uint8 { d.seq++; return d.seq }

// Start opens the NCP, resets the ASH link, negotiates the EZSP version, and
// reads the NCP's network state. It then serves callbacks until ctx ends.
func (d *Driver) Start(ctx context.Context) error {
	conn, err := ash.Open(d.cfg.Port, func(msg string, kv ...any) { d.log.Debug("ash: "+msg, kv...) })
	if err != nil {
		return err
	}
	d.conn = conn
	defer conn.Close()

	if err := conn.Reset(ctx); err != nil {
		return fmt.Errorf("ezsp reset: %w", err)
	}

	// The version handshake runs before the reader loop: it is the one exchange
	// still in the legacy frame format, so the extended decoder must not see it.
	ver, err := d.negotiateVersion(ctx, 13)
	if err != nil {
		return fmt.Errorf("ezsp version: %w", err)
	}
	d.log.Info("ezsp NCP connected",
		"port", d.cfg.Port,
		"ezspVersion", ver.ProtocolVersion,
		"stackType", ver.StackType,
		"stackVersion", fmt.Sprintf("%#04x", ver.StackVersion))
	if ver.ProtocolVersion < 8 {
		return fmt.Errorf("ezsp: NCP speaks v%d, extended frame format needs v8+", ver.ProtocolVersion)
	}

	// From here every frame is extended format, so the reader owns the stream.
	errc := make(chan error, 1)
	go func() { errc <- d.readLoop(ctx) }()

	state, err := d.bringUpNetwork(ctx)
	if err != nil {
		return fmt.Errorf("ezsp network bring-up: %w", err)
	}
	if !state.Joined() {
		return fmt.Errorf("ezsp: network still not up after forming (status %s)", StatusName(state.Status))
	}
	d.log.Info("ezsp network up",
		"nodeType", NodeTypeName(state.NodeType),
		"panId", fmt.Sprintf("%#04x", state.Params.PanID),
		"channel", state.Params.RadioChannel,
		"txPower", state.Params.RadioTxPower)

	if d.cfg.PermitJoin {
		if err := d.openForJoining(ctx, PermitJoinForever); err != nil {
			return fmt.Errorf("ezsp permit join: %w", err)
		}
		go d.watchJoinWindow(ctx)
	}

	d.log.Warn("ezsp backend is M3 (joining); ZCL send/receive not yet implemented")
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// negotiateVersion sends the version command and decodes the reply. It reads
// the connection directly because it runs before readLoop starts.
func (d *Driver) negotiateVersion(ctx context.Context, desired uint8) (VersionResponse, error) {
	if err := d.conn.Send(ctx, VersionCommand(d.nextSeq(), desired)); err != nil {
		return VersionResponse{}, err
	}
	resp, err := d.conn.Recv(ctx)
	if err != nil {
		return VersionResponse{}, err
	}
	return DecodeVersionResponse(resp)
}

// GetNetworkParameters reads the NCP's current node type and radio/PAN
// settings. On a factory-fresh dongle it answers NOT_JOINED rather than failing.
func (d *Driver) GetNetworkParameters(ctx context.Context) (NetworkState, error) {
	f, err := d.request(ctx, IDGetNetworkParameters, nil)
	if err != nil {
		return NetworkState{}, err
	}
	return DecodeNetworkParameters(f.Params)
}

// request sends an extended-format command and waits for the matching response.
func (d *Driver) request(ctx context.Context, id uint16, params []byte) (Frame, error) {
	d.reqMu.Lock()
	defer d.reqMu.Unlock()

	seq := d.nextSeq()
	p := &pending{seq: seq, id: id, ch: make(chan Frame, 1)}
	d.mu.Lock()
	d.pending = p
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		d.pending = nil
		d.mu.Unlock()
	}()

	if err := d.conn.Send(ctx, Command(seq, id, params)); err != nil {
		return Frame{}, err
	}
	timer := time.NewTimer(requestTimeout)
	defer timer.Stop()
	select {
	case f := <-p.ch:
		return f, nil
	case <-timer.C:
		return Frame{}, fmt.Errorf("ezsp: no response to frame %#04x (seq %d) within %s", id, seq, requestTimeout)
	case <-ctx.Done():
		return Frame{}, ctx.Err()
	}
}

// readLoop decodes frames off the ASH link and routes each one: to the waiting
// requester when the sequence and frame id both match, otherwise to the
// callback handler. Matching on the pair (rather than the frame control bits)
// is what zigpy/bellows does, and it tolerates an NCP setting flags we do not
// model — including a callback that happens to reuse a sequence number.
func (d *Driver) readLoop(ctx context.Context) error {
	for {
		raw, err := d.conn.Recv(ctx)
		if err != nil {
			return err
		}
		f, err := DecodeFrame(raw)
		if err != nil {
			d.log.Warn("ezsp undecodable frame", "bytes", fmt.Sprintf("% X", raw), "err", err)
			continue
		}
		if d.deliver(f) {
			continue
		}
		d.handleCallback(f)
	}
}

// deliver hands f to the in-flight request when it is that request's response.
func (d *Driver) deliver(f Frame) bool {
	d.mu.Lock()
	p := d.pending
	d.mu.Unlock()
	if p == nil || p.seq != f.Seq || p.id != f.ID {
		return false
	}
	select {
	case p.ch <- f:
	default: // already answered; a duplicate response is not worth blocking on
	}
	return true
}

// handleCallback processes an NCP-initiated frame. M5 turns the message and
// join handlers into bus events; until then they are surfaced in the log so a
// misbehaving NCP is visible.
func (d *Driver) handleCallback(f Frame) {
	switch f.ID {
	case IDStackStatusHandler:
		if len(f.Params) > 0 {
			d.log.Info("ezsp stack status", "status", StatusName(f.Params[0]))
			select {
			case d.stackStatus <- f.Params[0]:
			default: // nobody waiting and the buffer is full; the log has it
			}
			return
		}
	case IDTrustCenterJoinHandler:
		j, err := DecodeTrustCenterJoin(f.Params)
		if err != nil {
			d.log.Warn("ezsp trust-center join undecodable", "params", fmt.Sprintf("% X", f.Params), "err", err)
			return
		}
		d.log.Info("zigbee node "+UpdateName(j.Update),
			"ieee", j.IEEEString(),
			"nodeId", fmt.Sprintf("%#04x", j.NodeID),
			"parent", fmt.Sprintf("%#04x", j.ParentID),
			"hint", "put this ieee in the device config addr")
		return
	case IDIncomingMessageHandler:
		d.log.Info("ezsp incoming message", "params", fmt.Sprintf("% X", f.Params))
		return
	case IDMessageSentHandler:
		d.log.Debug("ezsp message sent", "params", fmt.Sprintf("% X", f.Params))
		return
	}
	d.log.Debug("ezsp callback", "id", fmt.Sprintf("%#04x", f.ID), "params", fmt.Sprintf("% X", f.Params))
}

// Apply maps a domain command onto an EZSP unicast of a ZCL frame.
// TODO(M4): build the ZCL frame (reuse the zstack driver's on/off + window
// covering encoding) and send it via sendUnicast to the device's node id.
func (d *Driver) Apply(cmd domain.Command) error {
	return fmt.Errorf("ezsp: Apply not yet implemented (M1) for %s", cmd.DeviceID)
}
