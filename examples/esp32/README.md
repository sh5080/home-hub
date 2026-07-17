# ESP32 device examples

The hub's MQTT adapter bridges ESP32 devices with a two-topic convention. The
topic base is the device's `addr` in `configs/devices.yaml` (or `home/<id>`
when `addr` is omitted):

| Topic | Direction | Payload |
|---|---|---|
| `<base>/state` | device → hub | `{"on":bool,"level":0..100,"value":float}` |
| `<base>/set`   | hub → device | `{"on":bool,"position":0..100,"level":0..100}` |

All fields are optional; a device reports and reacts to only the fields it has.
Point each device's `mqtt` broker at the hub.

- **`ceiling_fan.yaml`** — ESPHome config for a light-integrated RF ceiling fan.
  One ESP32, but exposed as **two independent devices** (`ceiling_light` type
  light + `ceiling_fan` type fan) so the light and fan are controlled
  separately from HomeKit or from an Aqara wall button — not conflated into one
  switch. This is the "안방 조명일체형 실링팬" path, replacing the vendor app.
- **`humidity.yaml`** — ESPHome config for a DHT22/SHT3x humidity sensor
  (`type: humidity`, so HomeKit shows relative humidity, not a bogus
  temperature) that publishes to its `state` topic. Pair it with a `threshold`
  rule (humidity → dehumidifier).

## Capturing the RF codes

The fan config has placeholder codes. Capture yours once with the vendor
remote and an RF receiver (or `rpitx`/an SDR), then paste the raw timings into
`ceiling_fan.yaml`. The hub does not need to know the codes — it only publishes
intent (`on`, `level`), and the ESP32 maps intent → RF.
