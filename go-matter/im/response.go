package im

import (
	"fmt"

	"github.com/sh5080/go-matter/tlv"
)

// Interaction Model status codes (Spec 8.10.1, Table of Status Codes). Only the
// codes a command invoker commonly encounters are named here; any uint8 is a
// valid status value on the wire.
const (
	StatusSuccess            uint8 = 0x00
	StatusFailure            uint8 = 0x01
	StatusInvalidAction      uint8 = 0x80
	StatusUnsupportedCommand uint8 = 0x81
	StatusInvalidCommand     uint8 = 0x85
	StatusConstraintError    uint8 = 0x87
	StatusUnsupportedCluster uint8 = 0xC3
)

// CommandStatus is a per-command status result (CommandStatusIB + StatusIB).
type CommandStatus struct {
	Path   CommandPath
	Status uint8
}

// InvokeResult is one entry of an InvokeResponse: either returned command data
// or a status. Exactly one of Command/Status is non-nil.
type InvokeResult struct {
	Command *InvokeCommand
	Status  *CommandStatus
}

// EncodeInvokeResponse builds an InvokeResponseMessage.
func EncodeInvokeResponse(results []InvokeResult, suppressResponse bool) ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	w.PutBool(tlv.Context(0), suppressResponse)
	w.StartArray(tlv.Context(1))
	for _, res := range results {
		w.StartStructure(tlv.Anonymous()) // InvokeResponseIB (array member)
		switch {
		case res.Command != nil:
			encodeCommandData(w, tlv.Context(0), *res.Command)
		case res.Status != nil:
			w.StartStructure(tlv.Context(1)) // CommandStatusIB
			encodePath(w, 0, res.Status.Path)
			w.StartStructure(tlv.Context(1)) // StatusIB
			w.PutUint(tlv.Context(0), uint64(res.Status.Status))
			w.EndContainer()
			w.EndContainer()
		default:
			return nil, fmt.Errorf("im: invoke result has neither command nor status")
		}
		w.EndContainer()
	}
	w.EndContainer()
	w.PutUint(tlv.Context(255), InteractionModelRevision)
	w.EndContainer()
	return w.Bytes()
}

// DecodeInvokeResponse parses an InvokeResponseMessage.
func DecodeInvokeResponse(b []byte) ([]InvokeResult, error) {
	r := tlv.NewReader(b)
	if !r.Next() || r.Type() != tlv.TypeStructure {
		return nil, fmt.Errorf("im: expected top-level structure")
	}
	if err := r.Enter(); err != nil {
		return nil, err
	}
	var results []InvokeResult
	for r.Next() {
		if r.Tag().Num != 1 { // invokeResponses array; ignore suppress(0)/revision(255)
			continue
		}
		if err := r.Enter(); err != nil {
			return nil, err
		}
		for r.Next() {
			res, err := decodeResponseIB(r)
			if err != nil {
				return nil, err
			}
			results = append(results, res)
		}
	}
	return results, r.Err()
}

func decodeResponseIB(r *tlv.Reader) (InvokeResult, error) {
	var res InvokeResult
	if err := r.Enter(); err != nil {
		return res, err
	}
	for r.Next() {
		switch r.Tag().Num {
		case 0:
			c, err := decodeCommandData(r)
			if err != nil {
				return res, err
			}
			res.Command = &c
		case 1:
			cs, err := decodeCommandStatus(r)
			if err != nil {
				return res, err
			}
			res.Status = &cs
		}
	}
	return res, nil
}

func decodeCommandStatus(r *tlv.Reader) (CommandStatus, error) {
	var cs CommandStatus
	if err := r.Enter(); err != nil {
		return cs, err
	}
	for r.Next() {
		switch r.Tag().Num {
		case 0:
			p, err := decodePath(r)
			if err != nil {
				return cs, err
			}
			cs.Path = p
		case 1: // StatusIB
			if err := r.Enter(); err != nil {
				return cs, err
			}
			for r.Next() {
				if r.Tag().Num == 0 {
					v, err := r.Uint()
					if err != nil {
						return cs, err
					}
					cs.Status = uint8(v)
				}
			}
		}
	}
	return cs, nil
}
