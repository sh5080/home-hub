package im

import (
	"fmt"

	"github.com/sh5080/go-matter/tlv"
)

// AttributePath identifies an attribute on a cluster/endpoint (AttributePathIB,
// a TLV list with tags 2:endpoint, 3:cluster, 4:attribute; node, listIndex, and
// tag-compression are omitted for a concrete path).
type AttributePath struct {
	Endpoint  uint16
	Cluster   uint32
	Attribute uint32
}

func encodeAttrPath(w *tlv.Writer, tag tlv.Tag, p AttributePath) {
	w.StartList(tag)
	w.PutUint(tlv.Context(2), uint64(p.Endpoint))
	w.PutUint(tlv.Context(3), uint64(p.Cluster))
	w.PutUint(tlv.Context(4), uint64(p.Attribute))
	w.EndContainer()
}

func decodeAttrPath(r *tlv.Reader) (AttributePath, error) {
	var p AttributePath
	if r.Type() != tlv.TypeList {
		return p, fmt.Errorf("im: attribute path is not a list")
	}
	if err := r.Enter(); err != nil {
		return p, err
	}
	for r.Next() {
		v, err := r.Uint()
		if err != nil {
			return p, err
		}
		switch r.Tag().Num {
		case 2:
			p.Endpoint = uint16(v)
		case 3:
			p.Cluster = uint32(v)
		case 4:
			p.Attribute = uint32(v)
		}
	}
	return p, nil
}

// EncodeReadRequest builds a ReadRequestMessage for the given attribute paths.
func EncodeReadRequest(paths []AttributePath, fabricFiltered bool) ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	w.StartArray(tlv.Context(0)) // attributeRequests
	for _, p := range paths {
		encodeAttrPath(w, tlv.Anonymous(), p)
	}
	w.EndContainer()
	w.PutBool(tlv.Context(3), fabricFiltered)
	w.PutUint(tlv.Context(255), InteractionModelRevision)
	w.EndContainer()
	return w.Bytes()
}

// EncodeSubscribeRequest builds a SubscribeRequestMessage.
func EncodeSubscribeRequest(paths []AttributePath, minFloorSec, maxCeilingSec uint16, keepSubscriptions, fabricFiltered bool) ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	w.PutBool(tlv.Context(0), keepSubscriptions)
	w.PutUint(tlv.Context(1), uint64(minFloorSec))
	w.PutUint(tlv.Context(2), uint64(maxCeilingSec))
	w.StartArray(tlv.Context(3)) // attributeRequests
	for _, p := range paths {
		encodeAttrPath(w, tlv.Anonymous(), p)
	}
	w.EndContainer()
	w.PutBool(tlv.Context(7), fabricFiltered)
	w.PutUint(tlv.Context(255), InteractionModelRevision)
	w.EndContainer()
	return w.Bytes()
}

// DecodeSubscribeResponse parses a SubscribeResponseMessage.
func DecodeSubscribeResponse(b []byte) (subscriptionID uint32, maxIntervalSec uint16, err error) {
	r := tlv.NewReader(b)
	if !r.Next() || r.Type() != tlv.TypeStructure {
		return 0, 0, fmt.Errorf("im: expected structure")
	}
	if err = r.Enter(); err != nil {
		return
	}
	for r.Next() {
		var v uint64
		if v, err = r.Uint(); err != nil {
			return
		}
		switch r.Tag().Num {
		case 0:
			subscriptionID = uint32(v)
		case 2:
			maxIntervalSec = uint16(v)
		}
	}
	return subscriptionID, maxIntervalSec, r.Err()
}

// AttributeReport is one entry of a ReportData: either an attribute value or a
// status. Data holds the attribute value re-encoded as a single anonymous-tag
// TLV element (use DecodeUint for a scalar); Status is set for a status report.
type AttributeReport struct {
	Path        AttributePath
	DataVersion uint32
	Data        []byte
	Status      *uint8
}

// DecodeReportData parses a ReportDataMessage.
func DecodeReportData(b []byte) (subscriptionID uint32, reports []AttributeReport, err error) {
	r := tlv.NewReader(b)
	if !r.Next() || r.Type() != tlv.TypeStructure {
		return 0, nil, fmt.Errorf("im: expected structure")
	}
	if err = r.Enter(); err != nil {
		return
	}
	for r.Next() {
		switch r.Tag().Num {
		case 0:
			var v uint64
			if v, err = r.Uint(); err != nil {
				return
			}
			subscriptionID = uint32(v)
		case 1: // attributeReports array
			if err = r.Enter(); err != nil {
				return
			}
			for r.Next() {
				var rep AttributeReport
				if rep, err = decodeAttrReportIB(r); err != nil {
					return
				}
				reports = append(reports, rep)
			}
		}
	}
	return subscriptionID, reports, r.Err()
}

func decodeAttrReportIB(r *tlv.Reader) (AttributeReport, error) {
	var rep AttributeReport
	if err := r.Enter(); err != nil { // AttributeReportIB { 0:status | 1:data }
		return rep, err
	}
	for r.Next() {
		switch r.Tag().Num {
		case 0: // AttributeStatusIB { 0:path, 1:StatusIB{0:status} }
			if err := r.Enter(); err != nil {
				return rep, err
			}
			for r.Next() {
				switch r.Tag().Num {
				case 0:
					p, err := decodeAttrPath(r)
					if err != nil {
						return rep, err
					}
					rep.Path = p
				case 1:
					if err := r.Enter(); err != nil {
						return rep, err
					}
					for r.Next() {
						if r.Tag().Num == 0 {
							v, err := r.Uint()
							if err != nil {
								return rep, err
							}
							s := uint8(v)
							rep.Status = &s
						}
					}
				}
			}
		case 1: // AttributeDataIB { 0:dataVersion, 1:path, 2:data }
			if err := r.Enter(); err != nil {
				return rep, err
			}
			for r.Next() {
				switch r.Tag().Num {
				case 0:
					v, err := r.Uint()
					if err != nil {
						return rep, err
					}
					rep.DataVersion = uint32(v)
				case 1:
					p, err := decodeAttrPath(r)
					if err != nil {
						return rep, err
					}
					rep.Path = p
				case 2:
					data, err := captureAnon(r)
					if err != nil {
						return rep, err
					}
					rep.Data = data
				}
			}
		}
	}
	return rep, nil
}

// DecodeUint extracts a single unsigned integer from a captured attribute value.
func DecodeUint(data []byte) (uint64, error) {
	r := tlv.NewReader(data)
	if !r.Next() {
		return 0, fmt.Errorf("im: empty attribute data")
	}
	return r.Uint()
}
