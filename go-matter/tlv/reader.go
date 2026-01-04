package tlv

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// Type classifies a decoded element.
type Type uint8

const (
	TypeInvalid Type = iota
	TypeInt
	TypeUint
	TypeBool
	TypeFloat
	TypeString
	TypeBytes
	TypeNull
	TypeStructure
	TypeArray
	TypeList
)

var typeNames = map[Type]string{
	TypeInvalid: "invalid", TypeInt: "int", TypeUint: "uint", TypeBool: "bool",
	TypeFloat: "float", TypeString: "string", TypeBytes: "bytes", TypeNull: "null",
	TypeStructure: "structure", TypeArray: "array", TypeList: "list",
}

func (t Type) String() string {
	if s, ok := typeNames[t]; ok {
		return s
	}
	return fmt.Sprintf("Type(%d)", uint8(t))
}

// maxDepth bounds container nesting to keep hostile input from exhausting the
// stack or looping excessively.
const maxDepth = 32

// Reader is a streaming TLV decoder.
//
// Usage: Next advances to the next element at the current depth and reports
// false at the end of the current container (or input). Enter steps into the
// current container element; iterating with Next until it returns false
// consumes the container's end marker and resumes at the parent level, so no
// explicit Exit is needed. A container element that is not entered is skipped
// entirely — including nested containers — by the following Next call.
//
// Containers must be iterated to completion once entered; abandoning a loop
// midway leaves the Reader positioned inside the container.
type Reader struct {
	data  []byte
	pos   int
	depth int
	err   error

	valid       bool
	skipPending bool // current element is a container that was not entered
	typ         Type
	tag         Tag
	u           uint64  // bool / uint storage
	i           int64   // int storage
	f           float64 // float storage
	b           []byte  // string / bytes view (copied out by getters)
}

// NewReader returns a Reader over data. The Reader does not modify data.
func NewReader(data []byte) *Reader { return &Reader{data: data} }

// Err returns the first error encountered while reading.
func (r *Reader) Err() error { return r.err }

func (r *Reader) fail(format string, args ...any) bool {
	if r.err == nil {
		r.err = fmt.Errorf("tlv: "+format+" (offset %d)", append(args, r.pos)...)
	}
	r.valid = false
	return false
}

// Next advances to the next element at the current depth. It returns false at
// the end of the current container, at the end of input, or on error (check
// Err to distinguish the latter).
func (r *Reader) Next() bool {
	if r.err != nil {
		return false
	}
	if r.valid && r.skipPending {
		if !r.skipContainerBody() {
			return false
		}
	}
	r.valid, r.skipPending = false, false

	if r.pos >= len(r.data) {
		if r.depth != 0 {
			return r.fail("input ended inside a container")
		}
		return false
	}
	ctl := r.data[r.pos]
	et := ctl & 0x1F
	if et == etEndOfCont {
		if r.depth == 0 {
			return r.fail("end-of-container at top level")
		}
		r.pos++
		r.depth--
		return false
	}
	r.pos++
	tag, ok := r.readTag(ctl >> 5)
	if !ok {
		return false
	}
	r.tag = tag
	if !r.readValue(et) {
		return false
	}
	r.valid = true
	if r.typ == TypeStructure || r.typ == TypeArray || r.typ == TypeList {
		r.skipPending = true
	}
	return true
}

// Enter steps into the current container element. Iterate its members with
// Next; when Next returns false the Reader has consumed the container's end
// marker and is positioned back at the parent level.
func (r *Reader) Enter() error {
	if r.err != nil {
		return r.err
	}
	if !r.valid || !r.skipPending {
		return errors.New("tlv: Enter: current element is not a container")
	}
	if r.depth >= maxDepth {
		r.fail("nesting deeper than %d", maxDepth)
		return r.err
	}
	r.depth++
	r.valid, r.skipPending = false, false
	return nil
}

// Type returns the type of the current element (TypeInvalid before the first
// Next or after Next returned false).
func (r *Reader) Type() Type {
	if !r.valid {
		return TypeInvalid
	}
	return r.typ
}

// Tag returns the tag of the current element.
func (r *Reader) Tag() Tag { return r.tag }

// Uint returns the current element as an unsigned integer. Signed elements
// are accepted when non-negative.
func (r *Reader) Uint() (uint64, error) {
	switch {
	case r.valid && r.typ == TypeUint:
		return r.u, nil
	case r.valid && r.typ == TypeInt && r.i >= 0:
		return uint64(r.i), nil
	}
	return 0, r.typeError("uint")
}

// Int returns the current element as a signed integer. Unsigned elements are
// accepted when they fit.
func (r *Reader) Int() (int64, error) {
	switch {
	case r.valid && r.typ == TypeInt:
		return r.i, nil
	case r.valid && r.typ == TypeUint && r.u <= math.MaxInt64:
		return int64(r.u), nil
	}
	return 0, r.typeError("int")
}

// Bool returns the current element as a boolean.
func (r *Reader) Bool() (bool, error) {
	if r.valid && r.typ == TypeBool {
		return r.u != 0, nil
	}
	return false, r.typeError("bool")
}

// Float returns the current element as a float (single- or double-precision).
func (r *Reader) Float() (float64, error) {
	if r.valid && r.typ == TypeFloat {
		return r.f, nil
	}
	return 0, r.typeError("float")
}

// String returns the current element as a UTF-8 string.
func (r *Reader) String() (string, error) {
	if r.valid && r.typ == TypeString {
		return string(r.b), nil
	}
	return "", r.typeError("string")
}

// Bytes returns a copy of the current octet-string element.
func (r *Reader) Bytes() ([]byte, error) {
	if r.valid && r.typ == TypeBytes {
		out := make([]byte, len(r.b))
		copy(out, r.b)
		return out, nil
	}
	return nil, r.typeError("bytes")
}

func (r *Reader) typeError(want string) error {
	return fmt.Errorf("tlv: element is %s, not %s", r.Type(), want)
}

// readTag decodes the tag bytes for the given tag-control value (0..7).
func (r *Reader) readTag(tc byte) (Tag, bool) {
	need := func(n int) bool {
		if len(r.data)-r.pos < n {
			r.fail("truncated tag")
			return false
		}
		return true
	}
	switch tc {
	case 0:
		return Anonymous(), true
	case 1:
		if !need(1) {
			return Tag{}, false
		}
		t := Context(r.data[r.pos])
		r.pos++
		return t, true
	case 2, 4:
		if !need(2) {
			return Tag{}, false
		}
		n := uint32(binary.LittleEndian.Uint16(r.data[r.pos:]))
		r.pos += 2
		if tc == 2 {
			return CommonProfile(n), true
		}
		return Implicit(n), true
	case 3, 5:
		if !need(4) {
			return Tag{}, false
		}
		n := binary.LittleEndian.Uint32(r.data[r.pos:])
		r.pos += 4
		if tc == 3 {
			return CommonProfile(n), true
		}
		return Implicit(n), true
	case 6:
		if !need(6) {
			return Tag{}, false
		}
		v := binary.LittleEndian.Uint16(r.data[r.pos:])
		p := binary.LittleEndian.Uint16(r.data[r.pos+2:])
		n := uint32(binary.LittleEndian.Uint16(r.data[r.pos+4:]))
		r.pos += 6
		return FullyQualified(v, p, n), true
	default: // 7
		if !need(8) {
			return Tag{}, false
		}
		v := binary.LittleEndian.Uint16(r.data[r.pos:])
		p := binary.LittleEndian.Uint16(r.data[r.pos+2:])
		n := binary.LittleEndian.Uint32(r.data[r.pos+4:])
		r.pos += 8
		return FullyQualified(v, p, n), true
	}
}

// readLE reads a little-endian unsigned value of the given byte width.
func (r *Reader) readLE(width int) (uint64, bool) {
	if len(r.data)-r.pos < width {
		r.fail("truncated value")
		return 0, false
	}
	var v uint64
	for i := 0; i < width; i++ {
		v |= uint64(r.data[r.pos+i]) << (8 * i)
	}
	r.pos += width
	return v, true
}

// readValue decodes the value for the given element type.
func (r *Reader) readValue(et byte) bool {
	switch {
	case et <= 0x03: // signed integer, width 1/2/4/8
		width := 1 << et
		v, ok := r.readLE(width)
		if !ok {
			return false
		}
		shift := uint(64 - 8*width)
		r.i = int64(v<<shift) >> shift // sign-extend
		r.typ = TypeInt
	case et <= 0x07: // unsigned integer
		v, ok := r.readLE(1 << (et - etUint8))
		if !ok {
			return false
		}
		r.u = v
		r.typ = TypeUint
	case et == etBoolFalse, et == etBoolTrue:
		if et == etBoolTrue {
			r.u = 1
		} else {
			r.u = 0
		}
		r.typ = TypeBool
	case et == etFloat32:
		v, ok := r.readLE(4)
		if !ok {
			return false
		}
		r.f = float64(math.Float32frombits(uint32(v)))
		r.typ = TypeFloat
	case et == etFloat64:
		v, ok := r.readLE(8)
		if !ok {
			return false
		}
		r.f = math.Float64frombits(v)
		r.typ = TypeFloat
	case et >= etUTF8Len1 && et <= 0x13: // string / bytes with length field
		n, ok := r.readLE(1 << ((et - etUTF8Len1) & 0x03))
		if !ok {
			return false
		}
		if n > uint64(len(r.data)-r.pos) {
			return r.fail("string length %d exceeds remaining input", n)
		}
		r.b = r.data[r.pos : r.pos+int(n)]
		r.pos += int(n)
		if et <= 0x0F {
			r.typ = TypeString
		} else {
			r.typ = TypeBytes
		}
	case et == etNull:
		r.typ = TypeNull
	case et == etStructure:
		r.typ = TypeStructure
	case et == etArray:
		r.typ = TypeArray
	case et == etList:
		r.typ = TypeList
	default:
		return r.fail("reserved element type 0x%02x", et)
	}
	return true
}

// skipContainerBody advances past the body (and end marker) of the current
// container without materializing values.
func (r *Reader) skipContainerBody() bool {
	level := 1
	for level > 0 {
		if r.pos >= len(r.data) {
			return r.fail("input ended inside a container")
		}
		ctl := r.data[r.pos]
		et := ctl & 0x1F
		r.pos++
		if et == etEndOfCont {
			level--
			continue
		}
		if _, ok := r.readTag(ctl >> 5); !ok {
			return false
		}
		switch {
		case et == etStructure, et == etArray, et == etList:
			if level >= maxDepth {
				return r.fail("nesting deeper than %d", maxDepth)
			}
			level++
		case et == etBoolFalse, et == etBoolTrue, et == etNull:
			// no value bytes
		case et <= 0x03:
			if _, ok := r.readLE(1 << et); !ok {
				return false
			}
		case et <= 0x07:
			if _, ok := r.readLE(1 << (et - etUint8)); !ok {
				return false
			}
		case et == etFloat32:
			if _, ok := r.readLE(4); !ok {
				return false
			}
		case et == etFloat64:
			if _, ok := r.readLE(8); !ok {
				return false
			}
		case et >= etUTF8Len1 && et <= 0x13:
			n, ok := r.readLE(1 << ((et - etUTF8Len1) & 0x03))
			if !ok {
				return false
			}
			if n > uint64(len(r.data)-r.pos) {
				return r.fail("string length %d exceeds remaining input", n)
			}
			r.pos += int(n)
		default:
			return r.fail("reserved element type 0x%02x", et)
		}
	}
	return true
}
