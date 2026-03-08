package casesession

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func decodeHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestGenerateDestinationID_SpecVector checks the destination identifier against
// the worked example from the CASE section of the Matter spec (reproduced in
// CHIP's DestinationIdTest).
func TestGenerateDestinationID_SpecVector(t *testing.T) {
	ipk := decodeHex(t, "9bc61cd9c62a2df6d64dfcaa9dc472d4")
	initRandom := decodeHex(t, "7e171231568dfa17206b3accf8faec2f4d21b580113196f47c7c4deb810a73dc")
	rootPub := decodeHex(t, "044a9f42b1ca4840d37292bbc7f6a7e11e22200c976fc900dbc98a7a383a641cb8254a2e56d4e295a847943b4e3897c4a773e930277b4d9fbede8a052686bfacfa")
	want := "dc35dd5fc9134cc5544538c9c3fc4297c1ec3370c839136a80e10796451d4c53"

	got, err := GenerateDestinationID(ipk, initRandom, rootPub, 0x2906C908D115D362, 0xCD5544AA7B13EF14)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got) != want {
		t.Fatalf("destinationId = %x, want %s", got, want)
	}

	// Changing the node id must change the result.
	other, _ := GenerateDestinationID(ipk, initRandom, rootPub, 0x2906C908D115D362, 0xCD5544AA7B13EF15)
	if bytes.Equal(got, other) {
		t.Fatal("destinationId did not depend on node id")
	}
}

func TestNonceLengths(t *testing.T) {
	// AES-CCM requires a 13-byte nonce; the TBE nonce strings must match.
	if len(nonceTBE2) != 13 || len(nonceTBE3) != 13 {
		t.Fatalf("TBE nonce lengths: %d, %d", len(nonceTBE2), len(nonceTBE3))
	}
}

func TestSigmaRoundTrips(t *testing.T) {
	s1 := Sigma1{
		InitiatorRandom:    bytes.Repeat([]byte{0x11}, 32),
		InitiatorSessionID: 0x1234,
		DestinationID:      bytes.Repeat([]byte{0x22}, 32),
		InitiatorEphPubKey: append([]byte{0x04}, bytes.Repeat([]byte{0x33}, 64)...),
	}
	b, err := s1.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got1, err := DecodeSigma1(b)
	if err != nil {
		t.Fatal(err)
	}
	if got1.InitiatorSessionID != s1.InitiatorSessionID || !bytes.Equal(got1.DestinationID, s1.DestinationID) ||
		!bytes.Equal(got1.InitiatorEphPubKey, s1.InitiatorEphPubKey) {
		t.Fatalf("sigma1 mismatch: %+v", got1)
	}

	s2 := Sigma2{
		ResponderRandom:    bytes.Repeat([]byte{0x44}, 32),
		ResponderSessionID: 0xABCD,
		ResponderEphPubKey: append([]byte{0x04}, bytes.Repeat([]byte{0x55}, 64)...),
		Encrypted2:         bytes.Repeat([]byte{0x66}, 80),
	}
	b, _ = s2.Encode()
	got2, err := DecodeSigma2(b)
	if err != nil {
		t.Fatal(err)
	}
	if got2.ResponderSessionID != s2.ResponderSessionID || !bytes.Equal(got2.Encrypted2, s2.Encrypted2) {
		t.Fatalf("sigma2 mismatch: %+v", got2)
	}

	s3 := Sigma3{Encrypted3: bytes.Repeat([]byte{0x77}, 100)}
	b, _ = s3.Encode()
	got3, err := DecodeSigma3(b)
	if err != nil || !bytes.Equal(got3.Encrypted3, s3.Encrypted3) {
		t.Fatalf("sigma3 mismatch: %+v (%v)", got3, err)
	}
}

func TestTBEDataRoundTrip(t *testing.T) {
	d := tbeData{
		NOC:       bytes.Repeat([]byte{0x01}, 40),
		ICAC:      bytes.Repeat([]byte{0x02}, 40),
		Signature: bytes.Repeat([]byte{0x03}, 64),
	}
	b, err := encodeTBEData(d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeTBEData(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.NOC, d.NOC) || !bytes.Equal(got.ICAC, d.ICAC) || !bytes.Equal(got.Signature, d.Signature) {
		t.Fatalf("tbeData mismatch: %+v", got)
	}

	// The ICAC is optional and must be omitted cleanly when absent.
	d.ICAC = nil
	b, _ = encodeTBEData(d)
	got, _ = decodeTBEData(b)
	if len(got.ICAC) != 0 {
		t.Fatalf("expected no ICAC, got %x", got.ICAC)
	}
}
