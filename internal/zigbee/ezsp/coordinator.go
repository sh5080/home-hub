package ezsp

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Coordinator endpoint. The hub is a controller: it drives On/Off, Level and
// Window Covering on other nodes (client side = output clusters) and serves
// only the two clusters every node must answer for (input clusters). Aqara
// switches in decoupled mode report presses on Multistate Input, which reaches
// us as a client of that cluster.
const (
	hubEndpoint  = 1
	profileHA    = 0x0104 // Home Automation
	deviceIDTool = 0x0005 // Configuration Tool — a controller, not an appliance

	clusterBasic          = 0x0000
	clusterIdentify       = 0x0003
	clusterOnOff          = 0x0006
	clusterLevelControl   = 0x0008
	clusterMultistateIn   = 0x0012
	clusterWindowCovering = 0x0102
)

var (
	hubInputClusters  = []uint16{clusterBasic, clusterIdentify}
	hubOutputClusters = []uint16{
		clusterBasic, clusterIdentify, clusterOnOff,
		clusterLevelControl, clusterMultistateIn, clusterWindowCovering,
	}
)

// Formation defaults. The channel matters: 802.15.4 and 2.4 GHz Wi-Fi share the
// band, and a coordinator parked under a busy Wi-Fi channel drops reports in a
// way that looks like flaky devices. Survey the local Wi-Fi and set
// zigbee.channel rather than trusting this default.
const (
	defaultChannel = 25 // 2475 MHz — above Wi-Fi ch11's upper edge
	defaultTxPower = 5  // dBm, matching zigbee-herdsman's default
)

// networkFile is where a formed network's identity is recorded, so the network
// key survives the dongle being lost. The dongle itself holds the authoritative
// copy in flash — this is a disaster-recovery note, not the runtime source.
const networkFile = "network.json"

// networkRecord is the on-disk form of networkFile.
type networkRecord struct {
	PanID         string `json:"panId"`
	ExtendedPanID string `json:"extendedPanId"`
	Channel       uint8  `json:"channel"`
	NetworkKey    string `json:"networkKey"`
	FormedAt      string `json:"formedAt"`
}

// bringUpNetwork registers the hub's endpoint and gets the coordinator onto a
// network: resuming the one in the dongle's flash, or forming a new one.
//
// Ordering is not free choice. addEndpoint must run before the stack starts —
// EmberZNet only accepts endpoint registration while the network is down — so
// it comes before networkInit even though it reads like application setup.
func (d *Driver) bringUpNetwork(ctx context.Context) (NetworkState, error) {
	if err := d.addEndpoint(ctx); err != nil {
		return NetworkState{}, fmt.Errorf("addEndpoint: %w", err)
	}

	initStatus, err := d.networkInit(ctx)
	if err != nil {
		return NetworkState{}, fmt.Errorf("networkInit: %w", err)
	}
	d.log.Debug("ezsp networkInit", "status", StatusName(initStatus))

	state, err := d.GetNetworkParameters(ctx)
	if err != nil {
		return NetworkState{}, err
	}
	if state.Joined() {
		return state, nil
	}

	d.log.Info("ezsp no stored network — forming a new one")
	if err := d.formNetwork(ctx); err != nil {
		return NetworkState{}, err
	}
	return d.GetNetworkParameters(ctx)
}

// addEndpoint registers the hub's application endpoint with the NCP.
func (d *Driver) addEndpoint(ctx context.Context) error {
	params := EncodeAddEndpoint(hubEndpoint, profileHA, deviceIDTool, 0, hubInputClusters, hubOutputClusters)
	f, err := d.request(ctx, IDAddEndpoint, params)
	if err != nil {
		return err
	}
	if len(f.Params) < 1 {
		return fmt.Errorf("empty response")
	}
	// EZSP_ERROR_DUPLICATE_ENDPOINT is not a failure: the NCP keeps endpoints
	// across host restarts, so a re-run legitimately finds ours already there.
	if f.Params[0] != StatusSuccess {
		d.log.Debug("ezsp addEndpoint non-zero status (already registered?)", "status", StatusName(f.Params[0]))
	}
	return nil
}

// networkInit asks the NCP to resume the network held in its flash. On a dongle
// that has never formed one this returns NOT_JOINED, which is the signal to
// form rather than an error.
func (d *Driver) networkInit(ctx context.Context) (uint8, error) {
	params := binary.LittleEndian.AppendUint16(nil, NetworkInitNoOptions)
	f, err := d.request(ctx, IDNetworkInit, params)
	if err != nil {
		return 0, err
	}
	if len(f.Params) < 1 {
		return 0, fmt.Errorf("empty response")
	}
	return f.Params[0], nil
}

// formNetwork creates a new Zigbee network with this dongle as coordinator and
// trust centre, then waits for the stack to report the network up.
func (d *Driver) formNetwork(ctx context.Context) error {
	var networkKey [16]byte
	if _, err := rand.Read(networkKey[:]); err != nil {
		return fmt.Errorf("network key: %w", err)
	}
	var extPanID [8]byte
	if _, err := rand.Read(extPanID[:]); err != nil {
		return fmt.Errorf("extended pan id: %w", err)
	}
	panID, err := randomPanID()
	if err != nil {
		return err
	}

	sec := EncodeInitialSecurityState(formSecurityBitmask, TCLinkKey, networkKey, 0)
	f, err := d.request(ctx, IDSetInitialSecurityState, sec)
	if err != nil {
		return fmt.Errorf("setInitialSecurityState: %w", err)
	}
	if len(f.Params) < 1 || f.Params[0] != StatusSuccess {
		return fmt.Errorf("setInitialSecurityState rejected: %s", statusOf(f))
	}

	params := NetworkParameters{
		ExtendedPanID: extPanID,
		PanID:         panID,
		RadioTxPower:  d.txPower(),
		RadioChannel:  d.channel(),
		JoinMethod:    JoinMethodMACAssociation,
		NwkManagerID:  0x0000, // the coordinator is its own network manager
		NwkUpdateID:   0,
		Channels:      AllChannelsMask,
	}
	f, err = d.request(ctx, IDFormNetwork, EncodeNetworkParameters(params))
	if err != nil {
		return fmt.Errorf("formNetwork: %w", err)
	}
	if len(f.Params) < 1 || f.Params[0] != StatusSuccess {
		return fmt.Errorf("formNetwork rejected: %s", statusOf(f))
	}

	// formNetwork returns as soon as the request is accepted; the network is
	// only usable once the stack says so.
	if err := d.waitStackUp(ctx); err != nil {
		return err
	}
	d.log.Info("ezsp network formed",
		"panId", fmt.Sprintf("%#04x", panID),
		"channel", params.RadioChannel,
		"txPower", params.RadioTxPower)

	d.saveNetworkRecord(panID, extPanID, params.RadioChannel, networkKey)
	return nil
}

// waitStackUp blocks until the NCP reports EMBER_NETWORK_UP.
func (d *Driver) waitStackUp(ctx context.Context) error {
	timer := time.NewTimer(networkUpTimeout)
	defer timer.Stop()
	for {
		select {
		case status := <-d.stackStatus:
			if status == StatusNetworkUp {
				return nil
			}
			d.log.Debug("ezsp stack status while forming", "status", StatusName(status))
		case <-timer.C:
			return fmt.Errorf("ezsp: network did not come up within %s", networkUpTimeout)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// saveNetworkRecord writes the formed network's identity next to the hub's
// other Zigbee state. Best-effort: losing the note does not break the running
// network, which lives in the dongle.
func (d *Driver) saveNetworkRecord(panID uint16, extPanID [8]byte, channel uint8, key [16]byte) {
	if d.cfg.Storage == "" {
		d.log.Warn("ezsp storage not configured — network key not recorded; " +
			"losing the dongle would mean re-pairing every device")
		return
	}
	if err := os.MkdirAll(d.cfg.Storage, 0o700); err != nil {
		d.log.Warn("ezsp storage unusable", "err", err)
		return
	}
	rec := networkRecord{
		PanID:         fmt.Sprintf("%#04x", panID),
		ExtendedPanID: hex.EncodeToString(extPanID[:]),
		Channel:       channel,
		NetworkKey:    hex.EncodeToString(key[:]),
		FormedAt:      time.Now().Format(time.RFC3339),
	}
	blob, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		d.log.Warn("ezsp network record encode failed", "err", err)
		return
	}
	path := filepath.Join(d.cfg.Storage, networkFile)
	// 0600: this file contains the network key.
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		d.log.Warn("ezsp network record write failed", "path", path, "err", err)
		return
	}
	d.log.Info("ezsp network recorded", "path", path)
}

// randomPanID picks a PAN id, avoiding the two reserved values.
func randomPanID() (uint16, error) {
	var b [2]byte
	for i := 0; i < 8; i++ {
		if _, err := rand.Read(b[:]); err != nil {
			return 0, fmt.Errorf("pan id: %w", err)
		}
		if id := binary.LittleEndian.Uint16(b[:]); id != 0x0000 && id != 0xFFFF {
			return id, nil
		}
	}
	return 0, fmt.Errorf("ezsp: could not draw a usable pan id")
}

func (d *Driver) channel() uint8 {
	if d.cfg.Channel >= 11 && d.cfg.Channel <= 26 {
		return d.cfg.Channel
	}
	return defaultChannel
}

func (d *Driver) txPower() uint8 {
	if d.cfg.TxPower > 0 {
		return d.cfg.TxPower
	}
	return defaultTxPower
}

// statusOf renders a response frame's status byte for an error message.
func statusOf(f Frame) string {
	if len(f.Params) < 1 {
		return "empty response"
	}
	return StatusName(f.Params[0])
}
