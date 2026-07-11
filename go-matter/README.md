# go-matter

A from-scratch, pure-Go implementation of a **Matter controller** — the side
that commissions and controls Matter devices (a role otherwise only available
in C++ (`connectedhomeip`), Python (`python-matter-server`), and TypeScript
(`matter.js`)).

> **Status: self-contained commissioning + operational control, verified in
> software.** The library can generate a fabric, commission a factory-fresh
> device onto it (PASE → CSR → issue NOC → AddNOC → CASE →
> CommissioningComplete), then resolve, connect, read attributes and invoke
> commands over On/Off, Window Covering, and Level Control — no chip-tool
> required. Everything is validated against spec/RFC test vectors and loopback
> tests; it has NOT yet been validated against physical hardware, and device
> attestation (DAC chain verification) is not enforced.

## Scope

- Controller / commissioner role only (this library does not implement devices)
- Matter-over-IP (Wi-Fi/Ethernet) transport, UDP 5540
- Target clusters: On/Off, Window Covering, Level Control

## Layers

| Package | Purpose | Status |
|---|---|---|
| `tlv` | Matter TLV codec (Spec Appendix A) | ✅ |
| `message` | Message / protocol header codec, counters | ✅ |
| `crypto` | AES-CCM, HKDF/PBKDF2, ECDSA raw | ✅ |
| `spake2` | SPAKE2+ (RFC 9383), for PASE | ✅ |
| `session` | Secure session encrypt/decrypt | ✅ |
| `cert` | Matter operational certificates (TLV ↔ X.509 DER) | ✅ |
| `casesession` | CASE (Sigma1/2/3) session establishment | ✅ |
| `im` | Interaction Model: Invoke / Read / Subscribe | ✅ |
| `cluster` | Typed cluster commands + commissioning clusters | ✅ (On/Off, Window Covering, Level Control, GeneralCommissioning, OpCreds) |
| `discovery` | mDNS operational + commissionable discovery | ✅ (IPv4 + IPv6 link-local zone scoping) |
| `transport` | UDP transport (with minimal MRP) + in-memory pipe | ✅ |
| `pase` | PASE handshake + QR / manual pairing-code parsing | ✅ |
| `controller` | Connect, dial, invoke, read, subscribe, **commission** | ✅ |

## Quick start

Control a device already commissioned to your fabric:

```go
fabric, _ := stored.Fabric()          // load fabric + controller identity
ctrl := controller.New(fabric, stored.Identity())

// Resolve by node id over mDNS and establish a CASE session.
sess, err := ctrl.Dial(ctx, nodeID)   // or ctrl.DialAddr(ctx, nodeID, "192.168.1.20:5540")
if err != nil { /* ... */ }
defer sess.Close()

// Invoke a command and read an attribute.
sess.Invoke(ctx, cluster.OnOffOn(1))
rep, _ := sess.ReadAttribute(ctx, cluster.OnOffAttribute(1))
on, _ := cluster.DecodeOnOff(rep.Data)
```

Subscribe for push updates instead of polling:

```go
sub, _ := sess.Subscribe(ctx, []im.AttributePath{cluster.LiftPositionAttribute(1)}, 1, 60)
for _, r := range sub.Initial { /* priming values */ }
go sub.Listen(ctx, func(reports []im.AttributeReport) { /* streamed updates */ })
```

Commission a new device onto a fresh fabric (no chip-tool):

```go
store, _ := controller.GenerateFabric(fabricID, controllerNodeID)
payload, _ := pase.ParseOnboarding("MT:...")   // QR or manual pairing code
// transport t reaches the device (commissionable mDNS gives its address)
controller.Commission(ctx, t, payload.Passcode, store, newNodeID, adminVendorID)
// then CASE to the device and finish:
sess, _ := controller.New(fabric, store.Identity()).Dial(ctx, newNodeID)
controller.CompleteCommissioning(ctx, sess)
```

Fabric credentials (root cert + CA key, controller NOC + key, IPK) are
persisted via `controller.StoredFabric` (JSON, `0600`).

## Not yet done

- Device attestation: the DAC/PAI chain is not verified, so an untrusted
  device could impersonate one. Acceptable for commissioning your own devices
  on a private LAN; add CSA-root-store verification before trusting unknown
  hardware.
- Thread/BLE commissioning transports (IP/on-network only).
- Validation against physical hardware.

## Design principles

- Minimal dependencies: Go standard library + `golang.org/x/net` (mDNS wire
  format) + `filippo.io/nistec` (P-256 point ops)
- Byte-level fidelity to the Matter Core Specification; spec section numbers
  are cited in comments
- Every cryptographic primitive is verified against published test vectors
  (RFC 3610/5869/7914/9383, Matter spec appendices) and certificate handling
  against real `connectedhomeip` reference certs

## License

MIT
