package cluster

import (
	"testing"

	"github.com/sh5080/go-matter/tlv"
)

// TestDecodeNOCResponseWithDebugText verifies that a NOCResponse carrying the
// optional DebugText string (tag 2, common on the failure path) still decodes —
// the StatusCode must not be lost because a later field is not a uint.
func TestDecodeNOCResponseWithDebugText(t *testing.T) {
	// Command fields are the flat member list (the IM layer strips the struct
	// wrapper), matching how a device encodes NOCResponse.
	w := tlv.NewWriter()
	w.PutUint(tlv.Context(0), 9) // StatusCode = FabricConflict
	w.PutUint(tlv.Context(1), 2) // FabricIndex
	w.PutString(tlv.Context(2), "fabric already present")
	fields, err := w.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	resp, err := DecodeNOCResponse(fields)
	if err != nil {
		t.Fatalf("decode rejected DebugText: %v", err)
	}
	if resp.Status != 9 || resp.FabricIndex != 2 {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestDecodeCommissioningErrorWithDebugText(t *testing.T) {
	w := tlv.NewWriter()
	w.PutUint(tlv.Context(0), 3) // errorCode
	w.PutString(tlv.Context(1), "busy")
	fields, _ := w.Bytes()

	code, err := DecodeCommissioningError(fields)
	if err != nil || code != 3 {
		t.Fatalf("code = %d (%v)", code, err)
	}
}
