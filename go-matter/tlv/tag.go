package tlv

// TagKind classifies how an element tag is encoded (Spec A.7.1).
type TagKind uint8

const (
	// KindAnonymous is an element with no tag (e.g. array members).
	KindAnonymous TagKind = iota
	// KindContext is a context-specific tag (0..255), the most common form.
	KindContext
	// KindCommonProfile is a Matter common-profile tag (2- or 4-byte number).
	KindCommonProfile
	// KindImplicitProfile is a tag in the message's implicit profile.
	KindImplicitProfile
	// KindFullyQualified carries an explicit vendor and profile.
	KindFullyQualified
)

// Tag identifies a TLV element within its container.
type Tag struct {
	Kind    TagKind
	Vendor  uint16
	Profile uint16
	Num     uint32
}

// Anonymous returns the empty tag used for untagged elements.
func Anonymous() Tag { return Tag{Kind: KindAnonymous} }

// Context returns a context-specific tag (structure/list field numbers).
func Context(n uint8) Tag { return Tag{Kind: KindContext, Num: uint32(n)} }

// CommonProfile returns a Matter common-profile tag.
func CommonProfile(num uint32) Tag { return Tag{Kind: KindCommonProfile, Num: num} }

// Implicit returns an implicit-profile tag.
func Implicit(num uint32) Tag { return Tag{Kind: KindImplicitProfile, Num: num} }

// FullyQualified returns a vendor/profile-qualified tag.
func FullyQualified(vendor, profile uint16, num uint32) Tag {
	return Tag{Kind: KindFullyQualified, Vendor: vendor, Profile: profile, Num: num}
}

// Tag-control values occupying the top three bits of the control byte.
const (
	tagCtlAnonymous byte = 0x00
	tagCtlContext   byte = 0x20
	tagCtlCommon2   byte = 0x40
	tagCtlCommon4   byte = 0x60
	tagCtlImplicit2 byte = 0x80
	tagCtlImplicit4 byte = 0xA0
	tagCtlFull6     byte = 0xC0
	tagCtlFull8     byte = 0xE0
)

// Element-type values occupying the low five bits of the control byte.
const (
	etInt8       byte = 0x00 // 0x00..0x03: signed 1/2/4/8 bytes
	etUint8      byte = 0x04 // 0x04..0x07: unsigned 1/2/4/8 bytes
	etBoolFalse  byte = 0x08
	etBoolTrue   byte = 0x09
	etFloat32    byte = 0x0A
	etFloat64    byte = 0x0B
	etUTF8Len1   byte = 0x0C // 0x0C..0x0F: UTF-8, length field 1/2/4/8 bytes
	etBytesLen1  byte = 0x10 // 0x10..0x13: octets, length field 1/2/4/8 bytes
	etNull       byte = 0x14
	etStructure  byte = 0x15
	etArray      byte = 0x16
	etList       byte = 0x17
	etEndOfCont  byte = 0x18
)
