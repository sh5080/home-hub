package cluster

import (
	"testing"

	"github.com/sh5080/go-matter/im"
	"github.com/sh5080/go-matter/tlv"
)

func TestOnOffCommands(t *testing.T) {
	cmd := OnOffOn(3)
	if cmd.Path.Endpoint != 3 || cmd.Path.Cluster != 0x0006 || cmd.Path.Command != 0x01 {
		t.Fatalf("OnOffOn = %+v", cmd.Path)
	}
	// The command must survive an invoke encode/decode.
	b, err := im.EncodeInvokeRequest([]im.InvokeCommand{cmd}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	got, _, _, err := im.DecodeInvokeRequest(b)
	if err != nil || len(got) != 1 || got[0].Path != cmd.Path {
		t.Fatalf("round trip: %+v (%v)", got, err)
	}
	if OnOffOff(1).Path.Command != 0x00 || OnOffToggle(1).Path.Command != 0x02 {
		t.Fatal("off/toggle command ids")
	}
	if OnOffAttribute(1).Attribute != 0x0000 {
		t.Fatal("onoff attribute id")
	}
}

func TestGoToLiftPercentage(t *testing.T) {
	cmd, err := GoToLiftPercentage(2, 37)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path.Cluster != 0x0102 || cmd.Path.Command != 0x05 {
		t.Fatalf("path = %+v", cmd.Path)
	}
	// Field 0 must be liftPercent100ths = 3700.
	r := tlv.NewReader(cmd.Fields)
	if !r.Next() {
		t.Fatal("no fields")
	}
	if r.Tag().Num != 0 {
		t.Fatalf("field tag = %d", r.Tag().Num)
	}
	if v, _ := r.Uint(); v != 3700 {
		t.Fatalf("liftPercent100ths = %d", v)
	}

	if _, err := GoToLiftPercentage(2, 100.0001); err == nil {
		t.Fatal("out-of-range percent should be rejected")
	}
}

func TestDecodeReportValues(t *testing.T) {
	// Simulate a CurrentPositionLiftPercent100ths report value (37.00%).
	vw := tlv.NewWriter()
	vw.PutUint(tlv.Anonymous(), 3700)
	liftData, _ := vw.Bytes()
	pct, err := DecodeLiftPercent(liftData)
	if err != nil || pct != 37 {
		t.Fatalf("lift percent = %g (%v)", pct, err)
	}

	bw := tlv.NewWriter()
	bw.PutBool(tlv.Anonymous(), true)
	onData, _ := bw.Bytes()
	on, err := DecodeOnOff(onData)
	if err != nil || !on {
		t.Fatalf("onoff = %v (%v)", on, err)
	}
}
