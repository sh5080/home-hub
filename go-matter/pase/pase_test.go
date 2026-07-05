package pase

import (
	"bytes"
	"testing"

	"github.com/sh5080/go-matter/message"
)

const (
	testPasscode   = uint32(20202021) // the well-known test setup code
	initSessionID  = uint16(0x1001)
	respSessionID  = uint16(0x2002)
	testIterations = 1000
)

func testParams() PBKDFParams {
	return PBKDFParams{Iterations: testIterations, Salt: bytes.Repeat([]byte{0x5A}, 16)}
}

// runHandshake drives a full PASE handshake between in-memory peers.
func runHandshake(t *testing.T, initiatorPasscode, responderPasscode uint32) (*Initiator, *Responder, error) {
	t.Helper()
	in := NewInitiator(initiatorPasscode, initSessionID)
	rd, err := NewResponder(responderPasscode, testParams(), respSessionID)
	if err != nil {
		t.Fatal(err)
	}

	req, err := in.ParamRequest()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := rd.HandleParamRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	pake1, err := in.HandleParamResponse(resp)
	if err != nil {
		t.Fatal(err)
	}
	pake2, err := rd.HandlePake1(pake1)
	if err != nil {
		t.Fatal(err)
	}
	pake3, err := in.HandlePake2(pake2)
	if err != nil {
		return in, rd, err
	}
	if err := rd.HandlePake3(pake3); err != nil {
		return in, rd, err
	}
	return in, rd, nil
}

func TestPASEHandshake(t *testing.T) {
	in, rd, err := runHandshake(t, testPasscode, testPasscode)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}

	inSess, err := in.SecureSession()
	if err != nil {
		t.Fatal(err)
	}
	rdSess, err := rd.SecureSession()
	if err != nil {
		t.Fatal(err)
	}

	// Round-trip an encrypted message in both directions.
	proto := message.ProtoHeader{Initiator: true, Opcode: 0x42, ExchangeID: 7, ProtocolID: message.ProtocolInteractionModel}
	payload := append(proto.Encode(), []byte("pase payload")...)
	frame, err := inSess.Encrypt(payload)
	if err != nil {
		t.Fatal(err)
	}
	got, err := rdSess.Decrypt(frame)
	if err != nil {
		t.Fatalf("responder decrypt: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("payload mismatch initiator→responder")
	}

	back, err := rdSess.Encrypt(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inSess.Decrypt(back); err != nil {
		t.Fatalf("initiator decrypt: %v", err)
	}

	// Both sides must derive the same attestation challenge.
	ch, err := in.AttestationChallenge()
	if err != nil || len(ch) != 16 {
		t.Fatalf("attestation challenge: %x (%v)", ch, err)
	}
}

func TestPASEWrongPasscode(t *testing.T) {
	_, _, err := runHandshake(t, testPasscode, testPasscode+1)
	if err == nil {
		t.Fatal("handshake with mismatched passcodes must fail")
	}
}

func TestPASEStateOrder(t *testing.T) {
	in := NewInitiator(testPasscode, initSessionID)
	if _, err := in.HandleParamResponse([]byte{0x15, 0x18}); err == nil {
		t.Fatal("HandleParamResponse before ParamRequest must fail")
	}
	if _, err := in.SecureSession(); err == nil {
		t.Fatal("SecureSession before completion must fail")
	}

	rd, err := NewResponder(testPasscode, testParams(), respSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rd.HandlePake1([]byte{0x15, 0x18}); err == nil {
		t.Fatal("HandlePake1 before HandleParamRequest must fail")
	}
}

func TestParamCodecRoundTrip(t *testing.T) {
	in := NewInitiator(testPasscode, initSessionID)
	raw, err := in.ParamRequest()
	if err != nil {
		t.Fatal(err)
	}
	req, err := decodeParamRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if req.InitiatorSessionID != initSessionID || req.PasscodeID != 0 || req.HasParams {
		t.Fatalf("request = %+v", req)
	}

	params := testParams()
	resp, err := encodeParamResponse(paramResponse{
		InitiatorRandom:    req.InitiatorRandom,
		ResponderRandom:    bytes.Repeat([]byte{0xBB}, 32),
		ResponderSessionID: respSessionID,
		Params:             &params,
	})
	if err != nil {
		t.Fatal(err)
	}
	dec, err := decodeParamResponse(resp)
	if err != nil {
		t.Fatal(err)
	}
	if dec.ResponderSessionID != respSessionID || dec.Params == nil ||
		dec.Params.Iterations != testIterations || !bytes.Equal(dec.Params.Salt, params.Salt) {
		t.Fatalf("response = %+v", dec)
	}
}
