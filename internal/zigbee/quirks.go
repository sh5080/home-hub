package zigbee

import "github.com/shimmeringbee/zigbee"

// Xiaomi/Aqara (Lumi) devices share the 0x00158D IEEE OUI prefix and deviate
// from the Zigbee spec in places (manufacturer-specific attributes, non-standard
// reporting, and a "magic" bind/keep-alive dance). Quirk handling is isolated
// here so the core driver stays clean.
const lumiOUIPrefix uint64 = 0x00158D0000000000

// isLumi reports whether an IEEE address belongs to a Xiaomi/Aqara device.
func isLumi(addr zigbee.IEEEAddress) bool {
	const mask uint64 = 0xFFFFFF0000000000
	return uint64(addr)&mask == lumiOUIPrefix
}

// aqaraOnOff attempts to read the On/Off state from a Xiaomi manufacturer-
// specific report (attribute 0xFF01), which packs several tagged values.
// Best-effort: returns ok=false when the frame is not the expected shape.
func aqaraOnOff(data []byte) (on bool, ok bool) {
	if len(data) < 3 || data[2] != 0x0a { // ZCL Report Attributes
		return false, false
	}
	rec := data[3:]
	if len(rec) < 3 {
		return false, false
	}
	if attrID := uint16(rec[0]) | uint16(rec[1])<<8; attrID != 0xFF01 {
		return false, false
	}
	// Walk the tagged payload looking for tag 0x64 (state, boolean).
	for p := rec[3:]; len(p) >= 3; p = p[3:] {
		if p[0] == 0x64 && p[1] == 0x10 { // tag 0x64, type 0x10 = boolean
			return p[2] != 0x00, true
		}
	}
	return false, false
}
