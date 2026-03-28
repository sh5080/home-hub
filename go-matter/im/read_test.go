package im

import (
	"testing"

	"github.com/sh5080/go-matter/tlv"
)

func TestReadRequestRoundTrip(t *testing.T) {
	paths := []AttributePath{
		{Endpoint: 1, Cluster: 0x0006, Attribute: 0x0000},   // OnOff
		{Endpoint: 2, Cluster: 0x0102, Attribute: 0x000E},   // CurrentPositionLiftPercent100ths
	}
	b, err := EncodeReadRequest(paths, true)
	if err != nil {
		t.Fatal(err)
	}
	got, fabricFiltered, err := DecodeReadRequest(b)
	if err != nil {
		t.Fatal(err)
	}
	if !fabricFiltered {
		t.Fatal("fabricFiltered lost")
	}
	if len(got) != 2 || got[0] != paths[0] || got[1] != paths[1] {
		t.Fatalf("paths = %+v", got)
	}
}

func TestSubscribeResponseRoundTrip(t *testing.T) {
	b, err := EncodeSubscribeResponse(0xDEADBEEF, 60)
	if err != nil {
		t.Fatal(err)
	}
	id, maxInterval, err := DecodeSubscribeResponse(b)
	if err != nil {
		t.Fatal(err)
	}
	if id != 0xDEADBEEF || maxInterval != 60 {
		t.Fatalf("subscriptionId=%#x maxInterval=%d", id, maxInterval)
	}
}

func TestSubscribeRequestEncodes(t *testing.T) {
	// Round-trip the attribute paths via the read decoder is not applicable
	// (different tag), so just assert it produces valid, non-empty TLV.
	b, err := EncodeSubscribeRequest([]AttributePath{{Endpoint: 1, Cluster: 0x0102, Attribute: 0x000E}}, 1, 60, false, true)
	if err != nil {
		t.Fatal(err)
	}
	r := tlv.NewReader(b)
	if !r.Next() || r.Type() != tlv.TypeStructure {
		t.Fatal("subscribe request is not a structure")
	}
}

func TestReportDataRoundTrip(t *testing.T) {
	// A CurrentPositionLiftPercent100ths report carrying uint16 3700.
	vw := tlv.NewWriter()
	vw.PutUint(tlv.Anonymous(), 3700)
	value, _ := vw.Bytes()

	path := AttributePath{Endpoint: 2, Cluster: 0x0102, Attribute: 0x000E}
	reports := []AttributeReport{{Path: path, DataVersion: 7, Data: value}}

	b, err := EncodeReportData(0x1234, reports, false)
	if err != nil {
		t.Fatal(err)
	}
	subID, got, err := DecodeReportData(b)
	if err != nil {
		t.Fatal(err)
	}
	if subID != 0x1234 {
		t.Fatalf("subscriptionId = %#x", subID)
	}
	if len(got) != 1 {
		t.Fatalf("got %d reports", len(got))
	}
	if got[0].Path != path || got[0].DataVersion != 7 {
		t.Fatalf("report = %+v", got[0])
	}
	v, err := DecodeUint(got[0].Data)
	if err != nil || v != 3700 {
		t.Fatalf("value = %d (%v)", v, err)
	}
}

func TestReportDataStatus(t *testing.T) {
	status := StatusUnsupportedCluster
	reports := []AttributeReport{{Path: AttributePath{Endpoint: 9, Cluster: 0x0006, Attribute: 0}, Status: &status}}
	b, err := EncodeReportData(0, reports, false)
	if err != nil {
		t.Fatal(err)
	}
	_, got, err := DecodeReportData(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Status == nil || *got[0].Status != StatusUnsupportedCluster {
		t.Fatalf("status report = %+v", got[0])
	}
}
