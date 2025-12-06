# home-hub

A pure-Go, single-binary home automation hub that bridges **Zigbee**, **MQTT/ESP32**,
and **HomeKit (HAP)** in one process, with a seam for a from-scratch Go Matter
controller (`go-matter`).

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full design.

## Layout

- `cmd/hub` — entrypoint; wires adapters around an in-process event bus
- `internal/domain` — protocol-agnostic core types (no external deps)
- `internal/bus` — in-process pub/sub event bus
- `internal/registry` — device + state store
- `internal/driver` — protocol adapter port interface
- `internal/zigbee` — Zigbee adapter (shimmeringbee, planned)
- `internal/mqtt` — embedded MQTT broker + bridge (mochi-mqtt, planned)
- `internal/homekit` — HAP bridge (brutella/hap, planned)
- `internal/matter` — Matter driver interface + HomeKit-delegated stub + registry
- `internal/automation` — rule engine
- `internal/health` — HTTP health endpoint

## Build & run

    make tidy      # resolve modules (writes go.sum)
    make build     # -> ./hub
    make run       # runs with configs/devices.yaml
    make test
    make pi        # ARMv7 cross-compile for Raspberry Pi

## Status

Skeleton stage: core + adapter scaffolds are in place; the protocol adapters are
stubs pending real library integration. Roadmap in `docs/ARCHITECTURE.md` §11.
