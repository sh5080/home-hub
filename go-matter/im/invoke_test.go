package im

import (
	"bytes"
	"testing"

	"github.com/sh5080/go-matter/tlv"
)

func TestInvokeRequestRoundTrip(t *testing.T) {
	// WindowCovering GoToLiftPercentage fields = { 0: uint16 liftPercent100ths }.
	fw := tlv.NewWriter()
	fw.PutUint(tlv.Context(0), 3700)
	fields, err := fw.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	cmds := []InvokeCommand{
		{Path: CommandPath{Endpoint: 1, Cluster: 0x0006, Command: 0x01}},                 // OnOff.On (no fields)
		{Path: CommandPath{Endpoint: 2, Cluster: 0x0102, Command: 0x05}, Fields: fields}, // GoToLiftPercentage
	}
	b, err := EncodeInvokeRequest(cmds, false, true)
	if err != nil {
		t.Fatal(err)
	}
	got, suppress, timed, err := DecodeInvokeRequest(b)
	if err != nil {
		t.Fatal(err)
	}
	if suppress || !timed {
		t.Fatalf("flags: suppress=%v timed=%v", suppress, timed)
	}
	if len(got) != 2 {
		t.Fatalf("got %d commands", len(got))
	}
	if got[0].Path != cmds[0].Path || got[0].Fields != nil {
		t.Fatalf("cmd0 = %+v", got[0])
	}
	if got[1].Path != cmds[1].Path || !bytes.Equal(got[1].Fields, fields) {
		t.Fatalf("cmd1 = %+v", got[1])
	}
}

func TestInvokeResponseRoundTrip(t *testing.T) {
	results := []InvokeResult{
		{Status: &CommandStatus{Path: CommandPath{Endpoint: 1, Cluster: 0x0006, Command: 0x01}, Status: StatusSuccess}},
		{Status: &CommandStatus{Path: CommandPath{Endpoint: 2, Cluster: 0x0102, Command: 0x05}, Status: StatusUnsupportedCommand}},
	}
	b, err := EncodeInvokeResponse(results, false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeInvokeResponse(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results", len(got))
	}
	if got[0].Status == nil || got[0].Status.Status != StatusSuccess || got[0].Status.Path != results[0].Status.Path {
		t.Fatalf("result0 = %+v", got[0].Status)
	}
	if got[1].Status == nil || got[1].Status.Status != StatusUnsupportedCommand {
		t.Fatalf("result1 = %+v", got[1].Status)
	}
}

func TestInvokeResponseWithCommandData(t *testing.T) {
	rw := tlv.NewWriter()
	rw.PutUint(tlv.Context(0), 42)
	payload, _ := rw.Bytes()

	results := []InvokeResult{
		{Command: &InvokeCommand{Path: CommandPath{Endpoint: 1, Cluster: 0x0102, Command: 0x00}, Fields: payload}},
	}
	b, err := EncodeInvokeResponse(results, false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeInvokeResponse(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Command == nil {
		t.Fatalf("expected command data, got %+v", got)
	}
	if !bytes.Equal(got[0].Command.Fields, payload) {
		t.Fatalf("fields = %x", got[0].Command.Fields)
	}
}
