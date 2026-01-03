package tlv

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Writer is a streaming TLV encoder. Errors are accumulated internally and
// reported by Bytes, so call sites can chain Put calls without checking each.
type Writer struct {
	buf   []byte
	depth int
	err   error
}

// NewWriter returns an empty Writer.
func NewWriter() *Writer { return &Writer{} }

func (w *Writer) fail(format string, args ...any) {
	if w.err == nil {
		w.err = fmt.Errorf("tlv: "+format, args...)
	}
}

// control appends the control byte (tag control | element type) and tag bytes.
func (w *Writer) control(t Tag, elemType byte) {
	if w.err != nil {
		return
	}
	switch t.Kind {
	case KindAnonymous:
		w.buf = append(w.buf, tagCtlAnonymous|elemType)
	case KindContext:
		if t.Num > 0xFF {
			w.fail("context tag %d out of range", t.Num)
			return
		}
		w.buf = append(w.buf, tagCtlContext|elemType, byte(t.Num))
	case KindCommonProfile:
		if t.Num <= 0xFFFF {
			w.buf = append(w.buf, tagCtlCommon2|elemType)
			w.buf = binary.LittleEndian.AppendUint16(w.buf, uint16(t.Num))
		} else {
			w.buf = append(w.buf, tagCtlCommon4|elemType)
			w.buf = binary.LittleEndian.AppendUint32(w.buf, t.Num)
		}
	case KindImplicitProfile:
		if t.Num <= 0xFFFF {
			w.buf = append(w.buf, tagCtlImplicit2|elemType)
			w.buf = binary.LittleEndian.AppendUint16(w.buf, uint16(t.Num))
		} else {
			w.buf = append(w.buf, tagCtlImplicit4|elemType)
			w.buf = binary.LittleEndian.AppendUint32(w.buf, t.Num)
		}
	case KindFullyQualified:
		if t.Num <= 0xFFFF {
			w.buf = append(w.buf, tagCtlFull6|elemType)
			w.buf = binary.LittleEndian.AppendUint16(w.buf, t.Vendor)
			w.buf = binary.LittleEndian.AppendUint16(w.buf, t.Profile)
			w.buf = binary.LittleEndian.AppendUint16(w.buf, uint16(t.Num))
		} else {
			w.buf = append(w.buf, tagCtlFull8|elemType)
			w.buf = binary.LittleEndian.AppendUint16(w.buf, t.Vendor)
			w.buf = binary.LittleEndian.AppendUint16(w.buf, t.Profile)
			w.buf = binary.LittleEndian.AppendUint32(w.buf, t.Num)
		}
	default:
		w.fail("unknown tag kind %d", t.Kind)
	}
}

func (w *Writer) appendLE(v uint64, width int) {
	for i := 0; i < width; i++ {
		w.buf = append(w.buf, byte(v>>(8*i)))
	}
}

// PutUint writes an unsigned integer using the minimal width (Spec A.11.2).
func (w *Writer) PutUint(t Tag, v uint64) {
	var et byte
	var width int
	switch {
	case v <= math.MaxUint8:
		et, width = etUint8, 1
	case v <= math.MaxUint16:
		et, width = etUint8+1, 2
	case v <= math.MaxUint32:
		et, width = etUint8+2, 4
	default:
		et, width = etUint8+3, 8
	}
	w.control(t, et)
	w.appendLE(v, width)
}

// PutInt writes a signed integer using the minimal width.
func (w *Writer) PutInt(t Tag, v int64) {
	var et byte
	var width int
	switch {
	case v >= math.MinInt8 && v <= math.MaxInt8:
		et, width = etInt8, 1
	case v >= math.MinInt16 && v <= math.MaxInt16:
		et, width = etInt8+1, 2
	case v >= math.MinInt32 && v <= math.MaxInt32:
		et, width = etInt8+2, 4
	default:
		et, width = etInt8+3, 8
	}
	w.control(t, et)
	w.appendLE(uint64(v), width) // two's-complement truncation
}

// PutBool writes a boolean (value is carried in the element type).
func (w *Writer) PutBool(t Tag, v bool) {
	if v {
		w.control(t, etBoolTrue)
	} else {
		w.control(t, etBoolFalse)
	}
}

// PutFloat32 writes a single-precision float.
func (w *Writer) PutFloat32(t Tag, v float32) {
	w.control(t, etFloat32)
	w.appendLE(uint64(math.Float32bits(v)), 4)
}

// PutFloat64 writes a double-precision float.
func (w *Writer) PutFloat64(t Tag, v float64) {
	w.control(t, etFloat64)
	w.appendLE(math.Float64bits(v), 8)
}

func (w *Writer) putBlob(t Tag, baseType byte, b []byte) {
	n := uint64(len(b))
	var et byte
	var lenWidth int
	switch {
	case n <= math.MaxUint8:
		et, lenWidth = baseType, 1
	case n <= math.MaxUint16:
		et, lenWidth = baseType+1, 2
	case n <= math.MaxUint32:
		et, lenWidth = baseType+2, 4
	default:
		et, lenWidth = baseType+3, 8
	}
	w.control(t, et)
	w.appendLE(n, lenWidth)
	w.buf = append(w.buf, b...)
}

// PutString writes a UTF-8 string.
func (w *Writer) PutString(t Tag, s string) { w.putBlob(t, etUTF8Len1, []byte(s)) }

// PutBytes writes an octet string.
func (w *Writer) PutBytes(t Tag, b []byte) { w.putBlob(t, etBytesLen1, b) }

// PutNull writes a null element.
func (w *Writer) PutNull(t Tag) { w.control(t, etNull) }

// StartStructure opens a structure container.
func (w *Writer) StartStructure(t Tag) {
	w.control(t, etStructure)
	w.depth++
}

// StartArray opens an array container. Array members must be anonymous.
func (w *Writer) StartArray(t Tag) {
	w.control(t, etArray)
	w.depth++
}

// StartList opens a list container.
func (w *Writer) StartList(t Tag) {
	w.control(t, etList)
	w.depth++
}

// EndContainer closes the innermost open container.
func (w *Writer) EndContainer() {
	if w.err != nil {
		return
	}
	if w.depth == 0 {
		w.fail("EndContainer without open container")
		return
	}
	w.buf = append(w.buf, etEndOfCont)
	w.depth--
}

// Bytes returns the encoding, failing if containers are unbalanced or any
// earlier Put call failed.
func (w *Writer) Bytes() ([]byte, error) {
	if w.err != nil {
		return nil, w.err
	}
	if w.depth != 0 {
		return nil, fmt.Errorf("tlv: %d container(s) left open", w.depth)
	}
	return w.buf, nil
}
