package zigbee

// Some vendors (notably Aqara) deviate from the Zigbee spec: manufacturer-
// specific attributes, non-standard reporting, and join/keep-alive behavior.
// Per-model quirk handling is isolated here so the core driver stays clean.
//
// TODO: implement quirks as the concrete device set is finalized.
