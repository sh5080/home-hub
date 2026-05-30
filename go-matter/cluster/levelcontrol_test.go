package cluster

import (
	"testing"

	"github.com/sh5080/go-matter/tlv"
)

func TestMoveToLevel(t *testing.T) {
	cmd, err := MoveToLevel(4, 128, 20)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path.Endpoint != 4 || cmd.Path.Cluster != 0x0008 || cmd.Path.Command != 0x00 {
		t.Fatalf("path = %+v", cmd.Path)
	}

	// Verify the four command fields in order: Level, TransitionTime, masks.
	want := []uint64{128, 20, 0, 0}
	r := tlv.NewReader(cmd.Fields)
	for i, exp := range want {
		if !r.Next() {
			t.Fatalf("missing field %d", i)
		}
		if int(r.Tag().Num) != i {
			t.Fatalf("field %d tag = %d", i, r.Tag().Num)
		}
		got, err := r.Uint()
		if err != nil {
			t.Fatal(err)
		}
		if got != exp {
			t.Fatalf("field %d = %d, want %d", i, got, exp)
		}
	}

	if _, err := MoveToLevel(1, 255, 0); err == nil {
		t.Fatal("level 255 (reserved) should be rejected")
	}
}

func TestDecodeLevel(t *testing.T) {
	w := tlv.NewWriter()
	w.PutUint(tlv.Anonymous(), 200)
	data, _ := w.Bytes()
	lvl, err := DecodeLevel(data)
	if err != nil || lvl != 200 {
		t.Fatalf("level = %d (%v)", lvl, err)
	}

	// A null CurrentLevel decodes to -1.
	nw := tlv.NewWriter()
	nw.PutNull(tlv.Anonymous())
	nullData, _ := nw.Bytes()
	lvl, err = DecodeLevel(nullData)
	if err != nil || lvl != -1 {
		t.Fatalf("null level = %d (%v)", lvl, err)
	}
}

func TestCurrentLevelAttribute(t *testing.T) {
	if a := CurrentLevelAttribute(7); a.Cluster != 0x0008 || a.Attribute != 0x0000 || a.Endpoint != 7 {
		t.Fatalf("attribute = %+v", a)
	}
}
