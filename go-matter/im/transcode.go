package im

import (
	"fmt"

	"github.com/sh5080/go-matter/tlv"
)

// captureAnon re-encodes the element the reader is positioned on as a single
// anonymous-tagged TLV element (handling nested containers), returning its
// bytes. Used to capture opaque attribute values.
//
// NOTE: TLV floats decode as float64, so a float32 value round-trips as float64
// (value preserved, not byte-identical). The clusters this controller targets
// do not use floats.
func captureAnon(r *tlv.Reader) ([]byte, error) {
	w := tlv.NewWriter()
	if err := writeElement(w, tlv.Anonymous(), r); err != nil {
		return nil, err
	}
	return w.Bytes()
}

// writeElement copies the current reader element into the writer under tag,
// recursing into containers.
func writeElement(w *tlv.Writer, tag tlv.Tag, r *tlv.Reader) error {
	switch r.Type() {
	case tlv.TypeUint:
		v, err := r.Uint()
		if err != nil {
			return err
		}
		w.PutUint(tag, v)
	case tlv.TypeInt:
		v, err := r.Int()
		if err != nil {
			return err
		}
		w.PutInt(tag, v)
	case tlv.TypeBool:
		v, err := r.Bool()
		if err != nil {
			return err
		}
		w.PutBool(tag, v)
	case tlv.TypeFloat:
		v, err := r.Float()
		if err != nil {
			return err
		}
		w.PutFloat64(tag, v)
	case tlv.TypeString:
		v, err := r.String()
		if err != nil {
			return err
		}
		w.PutString(tag, v)
	case tlv.TypeBytes:
		v, err := r.Bytes()
		if err != nil {
			return err
		}
		w.PutBytes(tag, v)
	case tlv.TypeNull:
		w.PutNull(tag)
	case tlv.TypeStructure, tlv.TypeArray, tlv.TypeList:
		switch r.Type() {
		case tlv.TypeStructure:
			w.StartStructure(tag)
		case tlv.TypeArray:
			w.StartArray(tag)
		default:
			w.StartList(tag)
		}
		if err := r.Enter(); err != nil {
			return err
		}
		for r.Next() {
			if err := writeElement(w, r.Tag(), r); err != nil {
				return err
			}
		}
		w.EndContainer()
	default:
		return fmt.Errorf("im: cannot transcode element type %v", r.Type())
	}
	return nil
}

// EncodeSubscribeResponse builds a SubscribeResponseMessage.
func EncodeSubscribeResponse(subscriptionID uint32, maxIntervalSec uint16) ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	w.PutUint(tlv.Context(0), uint64(subscriptionID))
	w.PutUint(tlv.Context(2), uint64(maxIntervalSec))
	w.PutUint(tlv.Context(255), InteractionModelRevision)
	w.EndContainer()
	return w.Bytes()
}

// EncodeReportData builds a ReportDataMessage. subscriptionID of 0 is omitted
// (as in a read response). Each report is either an attribute value or a status.
func EncodeReportData(subscriptionID uint32, reports []AttributeReport, suppressResponse bool) ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	if subscriptionID != 0 {
		w.PutUint(tlv.Context(0), uint64(subscriptionID))
	}
	w.StartArray(tlv.Context(1)) // attributeReportIBs
	for _, rep := range reports {
		w.StartStructure(tlv.Anonymous()) // AttributeReportIB
		if rep.Status != nil {
			w.StartStructure(tlv.Context(0)) // AttributeStatusIB
			encodeAttrPath(w, tlv.Context(0), rep.Path)
			w.StartStructure(tlv.Context(1)) // StatusIB
			w.PutUint(tlv.Context(0), uint64(*rep.Status))
			w.EndContainer()
			w.EndContainer()
		} else {
			w.StartStructure(tlv.Context(1)) // AttributeDataIB
			w.PutUint(tlv.Context(0), uint64(rep.DataVersion))
			encodeAttrPath(w, tlv.Context(1), rep.Path)
			dr := tlv.NewReader(rep.Data)
			if dr.Next() {
				if err := writeElement(w, tlv.Context(2), dr); err != nil {
					return nil, err
				}
			}
			w.EndContainer()
		}
		w.EndContainer()
	}
	w.EndContainer()
	if suppressResponse {
		w.PutBool(tlv.Context(4), true)
	}
	w.PutUint(tlv.Context(255), InteractionModelRevision)
	w.EndContainer()
	return w.Bytes()
}

// DecodeReadRequest parses a ReadRequestMessage (device-side / reference).
func DecodeReadRequest(b []byte) (paths []AttributePath, fabricFiltered bool, err error) {
	r := tlv.NewReader(b)
	if !r.Next() || r.Type() != tlv.TypeStructure {
		return nil, false, fmt.Errorf("im: expected structure")
	}
	if err = r.Enter(); err != nil {
		return
	}
	for r.Next() {
		switch r.Tag().Num {
		case 0:
			if err = r.Enter(); err != nil {
				return
			}
			for r.Next() {
				var p AttributePath
				if p, err = decodeAttrPath(r); err != nil {
					return
				}
				paths = append(paths, p)
			}
		case 3:
			if fabricFiltered, err = r.Bool(); err != nil {
				return
			}
		}
	}
	return paths, fabricFiltered, r.Err()
}
