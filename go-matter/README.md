# go-matter

A from-scratch, pure-Go implementation of a **Matter controller** — the side
that commissions and controls Matter devices (a role otherwise only available
in C++ (`connectedhomeip`), Python (`python-matter-server`), and TypeScript
(`matter.js`)).

> **Status: operational control works; commissioning is not implemented yet.**
> The library can resolve, connect to (CASE), and control a device that has
> already been commissioned to a fabric — read attributes and invoke commands
> over On/Off, Window Covering, and Level Control. It cannot yet onboard a new
> device (PASE + the commissioning cluster flow are pending), and it has been
> validated against spec/RFC test vectors and loopback tests, not yet against
> physical hardware.

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
| `cluster` | Typed cluster commands and attributes | ✅ (On/Off, Window Covering, Level Control) |
| `discovery` | mDNS operational discovery + resolve | ✅ (IPv4; IPv6 link-local zones: pending) |
| `transport` | UDP transport + in-memory pipe for tests | ✅ |
| `controller` | High-level API: connect, dial, invoke, read | ✅ |
| `pase`, commissioning | PASE and the commissioning flow | planned |

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

Fabric credentials (root cert, controller NOC + key, IPK) are persisted via
`controller.StoredFabric` (JSON, `0600`).

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
