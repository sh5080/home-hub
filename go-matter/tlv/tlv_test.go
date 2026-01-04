package tlv

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// specVectors are encoding examples from Matter Core Specification Appendix A
// (A.11.1 – A.11.4).
func TestSpecVectors(t *testing.T) {
	cases := []struct {
		name string
		enc  func(w *Writer)
		hex  string
	}{
		{"uint8 42 anonymous", func(w *Writer) { w.PutUint(Anonymous(), 42) }, "042a"},
		{"int8 -17", func(w *Writer) { w.PutInt(Anonymous(), -17) }, "00ef"},
		{"uint16 4660", func(w *Writer) { w.PutUint(Anonymous(), 0x1234) }, "053412"},
		{"bool false", func(w *Writer) { w.PutBool(Anonymous(), false) }, "08"},
		{"bool true", func(w *Writer) { w.PutBool(Anonymous(), true) }, "09"},
		{"float32 17.9", func(w *Writer) { w.PutFloat32(Anonymous(), 17.9) }, "0a33338f41"},
		{"utf8 Hello!", func(w *Writer) { w.PutString(Anonymous(), "Hello!") }, "0c0648656c6c6f21"},
		{"octets 00..04", func(w *Writer) { w.PutBytes(Anonymous(), []byte{0, 1, 2, 3, 4}) }, "10050001020304"},
		{"null", func(w *Writer) { w.PutNull(Anonymous()) }, "14"},
		{"empty structure", func(w *Writer) { w.StartStructure(Anonymous()); w.EndContainer() }, "1518"},
		{"empty array", func(w *Writer) { w.StartArray(Anonymous()); w.EndContainer() }, "1618"},
		{"struct{0:false,1:true}", func(w *Writer) {
			w.StartStructure(Anonymous())
			w.PutBool(Context(0), false)
			w.PutBool(Context(1), true)
			w.EndContainer()
		}, "152800290118"},
		{"array of ints 0..4", func(w *Writer) {
			w.StartArray(Anonymous())
			for i := int64(0); i <= 4; i++ {
				w.PutInt(Anonymous(), i)
			}
			w.EndContainer()
		}, "160000000100020003000418"},
		{"context tag 1, uint 1", func(w *Writer) { w.PutUint(Context(1), 1) }, "240101"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := NewWriter()
			tc.enc(w)
			got, err := w.Bytes()
			if err != nil {
				t.Fatalf("Bytes: %v", err)
			}
			if hex.EncodeToString(got) != tc.hex {
				t.Fatalf("encoded %x, want %s", got, tc.hex)
			}
		})
	}
}

func TestMinimalIntegerWidths(t *testing.T) {
	cases := []struct {
		enc func(w *Writer)
		hex string
	}{
		{func(w *Writer) { w.PutUint(Anonymous(), 255) }, "04ff"},
		{func(w *Writer) { w.PutUint(Anonymous(), 256) }, "050001"},
		{func(w *Writer) { w.PutUint(Anonymous(), 65536) }, "0600000100"},
		{func(w *Writer) { w.PutUint(Anonymous(), 1<<32) }, "070000000001000000"},
		{func(w *Writer) { w.PutInt(Anonymous(), -128) }, "0080"},
		{func(w *Writer) { w.PutInt(Anonymous(), -129) }, "017fff"},
		{func(w *Writer) { w.PutInt(Anonymous(), 1 << 40) }, "030000000000010000"},
	}
	for _, tc := range cases {
		w := NewWriter()
		tc.enc(w)
		got, err := w.Bytes()
		if err != nil {
			t.Fatalf("Bytes: %v", err)
		}
		if hex.EncodeToString(got) != tc.hex {
			t.Fatalf("encoded %x, want %s", got, tc.hex)
		}
	}
}

func TestTagFormsRoundTrip(t *testing.T) {
	tags := []Tag{
		Anonymous(),
		Context(0), Context(255),
		CommonProfile(1), CommonProfile(0x12345),
		Implicit(7), Implicit(0x10000),
		FullyQualified(0xFFF1, 0xDEED, 42), FullyQualified(1, 2, 0x12345678),
	}
	w := NewWriter()
	w.StartStructure(Anonymous())
	for i, tag := range tags {
		w.PutUint(tag, uint64(i))
	}
	w.EndContainer()
	b, err := w.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	r := NewReader(b)
	if !r.Next() {
		t.Fatalf("Next struct: %v", r.Err())
	}
	if err := r.Enter(); err != nil {
		t.Fatal(err)
	}
	for i, want := range tags {
		if !r.Next() {
			t.Fatalf("Next member %d: %v", i, r.Err())
		}
		if got := r.Tag(); got != want {
			t.Fatalf("member %d tag = %+v, want %+v", i, got, want)
		}
		v, err := r.Uint()
		if err != nil || v != uint64(i) {
			t.Fatalf("member %d value = %d (%v)", i, v, err)
		}
	}
	if r.Next() {
		t.Fatal("expected end of structure")
	}
	if r.Err() != nil {
		t.Fatal(r.Err())
	}
}

func TestNestedRoundTrip(t *testing.T) {
	w := NewWriter()
	w.StartStructure(Anonymous())
	w.PutUint(Context(0), 7)
	w.StartArray(Context(1))
	w.StartStructure(Anonymous())
	w.PutString(Context(0), "블라인드") // UTF-8 passthrough
	w.PutInt(Context(1), -42)
	w.EndContainer()
	w.PutBool(Anonymous(), true)
	w.EndContainer()
	w.StartList(Context(2))
	w.PutNull(Anonymous())
	w.PutFloat64(Context(3), 2.5)
	w.EndContainer()
	w.PutBytes(Context(4), []byte{0xde, 0xad})
	w.EndContainer()
	b, err := w.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	r := NewReader(b)
	if !r.Next() || r.Type() != TypeStructure {
		t.Fatalf("want structure, got %v (%v)", r.Type(), r.Err())
	}
	if err := r.Enter(); err != nil {
		t.Fatal(err)
	}

	if !r.Next() {
		t.Fatal("member 0")
	}
	if v, _ := r.Uint(); v != 7 {
		t.Fatalf("ctx0 = %d", v)
	}

	if !r.Next() || r.Type() != TypeArray {
		t.Fatal("member 1 should be array")
	}
	if err := r.Enter(); err != nil {
		t.Fatal(err)
	}
	if !r.Next() || r.Type() != TypeStructure {
		t.Fatal("array[0] should be struct")
	}
	if err := r.Enter(); err != nil {
		t.Fatal(err)
	}
	if !r.Next() {
		t.Fatal("struct member 0")
	}
	if s, _ := r.String(); s != "블라인드" {
		t.Fatalf("string = %q", s)
	}
	if !r.Next() {
		t.Fatal("struct member 1")
	}
	if v, _ := r.Int(); v != -42 {
		t.Fatalf("int = %d", v)
	}
	if r.Next() {
		t.Fatal("struct should end")
	}
	if !r.Next() {
		t.Fatal("array[1]")
	}
	if v, _ := r.Bool(); !v {
		t.Fatal("bool should be true")
	}
	if r.Next() {
		t.Fatal("array should end")
	}

	// Skip the list without entering it: Next must jump over it entirely.
	if !r.Next() || r.Type() != TypeList {
		t.Fatalf("member 2 should be list, got %v", r.Type())
	}
	if !r.Next() || r.Type() != TypeBytes {
		t.Fatalf("member 4 should be bytes after skipping list, got %v (%v)", r.Type(), r.Err())
	}
	got, _ := r.Bytes()
	if !bytes.Equal(got, []byte{0xde, 0xad}) {
		t.Fatalf("bytes = %x", got)
	}
	if r.Next() {
		t.Fatal("structure should end")
	}
	if r.Err() != nil {
		t.Fatal(r.Err())
	}
}

func TestIntUintCoercion(t *testing.T) {
	w := NewWriter()
	w.PutInt(Anonymous(), 5)
	b, _ := w.Bytes()
	r := NewReader(b)
	r.Next()
	if v, err := r.Uint(); err != nil || v != 5 {
		t.Fatalf("Uint from non-negative int = %d, %v", v, err)
	}

	w = NewWriter()
	w.PutInt(Anonymous(), -1)
	b, _ = w.Bytes()
	r = NewReader(b)
	r.Next()
	if _, err := r.Uint(); err == nil {
		t.Fatal("Uint from negative int should fail")
	}

	w = NewWriter()
	w.PutUint(Anonymous(), 9)
	b, _ = w.Bytes()
	r = NewReader(b)
	r.Next()
	if v, err := r.Int(); err != nil || v != 9 {
		t.Fatalf("Int from small uint = %d, %v", v, err)
	}
}

func TestFloat32RoundTrip(t *testing.T) {
	w := NewWriter()
	w.PutFloat32(Anonymous(), 17.9)
	b, _ := w.Bytes()
	r := NewReader(b)
	if !r.Next() {
		t.Fatal(r.Err())
	}
	f, err := r.Float()
	if err != nil {
		t.Fatal(err)
	}
	if float32(f) != 17.9 {
		t.Fatalf("float = %v", f)
	}
}

func TestWriterErrors(t *testing.T) {
	w := NewWriter()
	w.StartStructure(Anonymous())
	if _, err := w.Bytes(); err == nil {
		t.Fatal("unbalanced container should fail")
	}

	w = NewWriter()
	w.EndContainer()
	if _, err := w.Bytes(); err == nil {
		t.Fatal("EndContainer at depth 0 should fail")
	}
}

func TestReaderMalformedInput(t *testing.T) {
	cases := []struct {
		name string
		in   string // hex
	}{
		{"stray end-of-container", "18"},
		{"truncated string length", "0c"},
		{"string length beyond input", "0c0648"},
		{"truncated uint16", "0534"},
		{"unclosed structure", "1528"},
		{"truncated context tag", "24"},
		{"reserved element type", "1f"},
		{"huge 8-byte length", "0fffffffffffffffff"},
		{"container never closed", "15"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, err := hex.DecodeString(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			r := NewReader(in)
			for r.Next() {
				if r.Type() == TypeStructure || r.Type() == TypeArray || r.Type() == TypeList {
					if r.Enter() != nil {
						break
					}
				}
			}
			if r.Err() == nil {
				t.Fatal("malformed input should produce an error")
			}
		})
	}
}

func TestDeepNestingBounded(t *testing.T) {
	// 64 nested structures exceed maxDepth and must fail cleanly on Enter.
	var in []byte
	for i := 0; i < 64; i++ {
		in = append(in, 0x15)
	}
	for i := 0; i < 64; i++ {
		in = append(in, 0x18)
	}
	r := NewReader(in)
	depth := 0
	for r.Next() {
		if err := r.Enter(); err != nil {
			break
		}
		depth++
	}
	if r.Err() == nil {
		t.Fatal("expected depth-limit error")
	}
	if depth < maxDepth-1 || depth > maxDepth {
		t.Fatalf("stopped at depth %d, want ~%d", depth, maxDepth)
	}

	// Skipping (not entering) an over-deep container must also be bounded. The
	// first Next reads the outer container element; the second triggers the
	// deferred skip of its over-deep body, which must fail cleanly.
	r = NewReader(append(in, 0x04, 0x01))
	if !r.Next() {
		t.Fatal("first Next should read the outer container element")
	}
	if r.Next() {
		t.Fatal("skip of over-deep container body should fail")
	}
	if r.Err() == nil {
		t.Fatal("expected depth-limit error from skip")
	}
}

// walk exercises every element reachable from r, entering all containers.
func walk(r *Reader, depth int) {
	for r.Next() {
		switch r.Type() {
		case TypeStructure, TypeArray, TypeList:
			if depth < maxDepth+2 {
				if r.Enter() != nil {
					return
				}
				walk(r, depth+1)
			}
		case TypeString:
			_, _ = r.String()
		case TypeBytes:
			_, _ = r.Bytes()
		case TypeInt:
			_, _ = r.Int()
		case TypeUint:
			_, _ = r.Uint()
		case TypeBool:
			_, _ = r.Bool()
		case TypeFloat:
			_, _ = r.Float()
		}
	}
}

func FuzzReader(f *testing.F) {
	seeds := []string{
		"042a", "00ef", "152800290118", "160000000100020003000418",
		"0c0648656c6c6f21", "10050001020304", "1518", "240101",
		"15240007360115280018182402021824ff0118", // IM-ish nested shape
	}
	for _, s := range seeds {
		b, _ := hex.DecodeString(s)
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		r := NewReader(data) // must never panic
		walk(r, 0)
	})
}
