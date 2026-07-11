# ESP32 device examples

The hub's MQTT adapter bridges ESP32 devices with a two-topic convention
(`<id>` is the device id from `configs/devices.yaml`):

| Topic | Direction | Payload |
|---|---|---|
| `home/<id>/state` | device → hub | `{"on":bool,"level":0..100,"value":float}` |
| `home/<id>/set`   | hub → device | `{"on":bool,"position":0..100,"level":0..100}` |

All fields are optional; a device reports and reacts to only the fields it has.
Point each device's `mqtt` broker at the hub (`home/<id>` is the device's
`addr` in the config).

- **`ceiling_fan.yaml`** — ESPHome config for an RF ceiling fan bridge. It
  subscribes to its `set` topic and retransmits the captured RF codes for the
  light and the fan motor, and mirrors state back. This is the "안방 조명일체형
  실링팬" path: the wall switch (Aqara, decoupled) or the iPhone drives the fan
  through the hub instead of the vendor app.
- **`humidity.yaml`** — ESPHome config for a DHT22/SHT3x temperature+humidity
  sensor that publishes to its `state` topic. Pair it with a `threshold`
  automation rule (humidity → dehumidifier).

## Capturing the RF codes

The fan config has placeholder codes. Capture yours once with the vendor
remote and an RF receiver (or `rpitx`/an SDR), then paste the raw timings into
`ceiling_fan.yaml`. The hub does not need to know the codes — it only publishes
intent (`on`, `level`), and the ESP32 maps intent → RF.
