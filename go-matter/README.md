# go-matter

A from-scratch, pure-Go implementation of a **Matter controller** — the side
that commissions and controls Matter devices (a role otherwise only available
in C++ (`connectedhomeip`), Python (`python-matter-server`), and TypeScript
(`matter.js`)).

> **Status: work in progress.** Layers are being built bottom-up and each layer
> ships with spec test vectors. Not yet usable against real devices.

## Scope

- Controller / commissioner role only (this library does not implement devices)
- Matter-over-IP (WiFi/Ethernet) transport, UDP 5540
- Target clusters: On/Off, Window Covering, Level Control, plus the clusters
  required for commissioning

## Layers

| Package | Purpose | Status |
|---|---|---|
| `tlv` | Matter TLV codec (Spec Appendix A) | ✅ |
| `message` | Message / protocol header codec, counters | planned |
| `crypto` | AES-CCM, SPAKE2+, HKDF/PBKDF2, ECDSA raw | planned |
| `session` | Secure session encrypt/decrypt, replay protection | planned |
| `cert` | Matter operational certificates (TLV form) | planned |
| `casesession` | CASE (Sigma1/2/3) session establishment | planned |
| `exchange` | Exchange layer + MRP reliability | planned |
| `im` | Interaction Model: Invoke / Read / Subscribe | planned |
| `cluster` | Typed cluster commands and attributes | planned |
| `discovery` | mDNS operational / commissionable discovery | planned |
| `pase`, `commission` | PASE and the commissioning flow | planned |
| `controller` | High-level public API | planned |

## Design principles

- Minimal dependencies: Go standard library + `golang.org/x/crypto`
- Byte-level fidelity to the Matter Core Specification; spec section numbers
  are cited in comments
- Parsers are hardened against malformed input and fuzz-tested
- Every cryptographic primitive is verified against published test vectors
  (NIST CAVP, RFC 9383, spec appendices)

## License

MIT
