package pase

import (
	"crypto/elliptic"
	"fmt"
	"math/big"

	"github.com/sh5080/go-matter/session"
	"github.com/sh5080/go-matter/spake2"
)

// Responder is the device side of PASE, used by the loopback testkit (and by
// any future commissionee role). It knows the SPAKE2+ verifier record — w0 and
// L — plus the PBKDF parameters it announces; it never needs w1 itself.
type Responder struct {
	w0             *big.Int
	l              []byte
	params         PBKDFParams
	localSessionID uint16

	context []byte
	keys    *spake2.MatterKeys
	peerSID uint16
	done    bool
}

// NewResponder derives the verifier record from a passcode (as a factory
// provisions a device) and prepares the responder.
func NewResponder(passcode uint32, params PBKDFParams, localSessionID uint16) (*Responder, error) {
	w0, w1, err := spake2.DeriveW(passcode, params.Salt, params.Iterations)
	if err != nil {
		return nil, err
	}
	l, err := spake2.ComputeL(w1)
	if err != nil {
		return nil, err
	}
	return &Responder{w0: w0, l: l, params: params, localSessionID: localSessionID}, nil
}

// HandleParamRequest consumes a PBKDFParamRequest and returns the
// PBKDFParamResponse payload.
func (rd *Responder) HandleParamRequest(raw []byte) ([]byte, error) {
	req, err := decodeParamRequest(raw)
	if err != nil {
		return nil, err
	}
	if req.PasscodeID != 0 {
		return nil, fmt.Errorf("pase: unsupported passcode id %d", req.PasscodeID)
	}
	rd.peerSID = req.InitiatorSessionID

	responderRandom, err := random32()
	if err != nil {
		return nil, err
	}
	resp, err := encodeParamResponse(paramResponse{
		InitiatorRandom:    req.InitiatorRandom,
		ResponderRandom:    responderRandom,
		ResponderSessionID: rd.localSessionID,
		Params:             &rd.params,
	})
	if err != nil {
		return nil, err
	}
	rd.context = contextHash(raw, resp)
	return resp, nil
}

// HandlePake1 consumes Pake1 {pA} and returns Pake2 {pB, cB}.
func (rd *Responder) HandlePake1(raw []byte) ([]byte, error) {
	if rd.context == nil {
		return nil, fmt.Errorf("pase: HandlePake1 before HandleParamRequest")
	}
	var pA []byte
	if err := decodeFields(raw, map[uint32]*[]byte{1: &pA}); err != nil {
		return nil, err
	}
	y, err := randomScalar(elliptic.P256().Params().N)
	if err != nil {
		return nil, err
	}
	verifier, err := spake2.NewVerifier(rd.w0, rd.l, y)
	if err != nil {
		return nil, err
	}
	keys, err := verifier.ConfirmMatter(pA, rd.context)
	if err != nil {
		return nil, err
	}
	rd.keys = keys
	return encodePake2(verifier.Share(), keys.CB)
}

// HandlePake3 consumes Pake3 {cA} and verifies the initiator's confirmation.
// On success the caller answers with a SUCCESS StatusReport and the session is
// live.
func (rd *Responder) HandlePake3(raw []byte) error {
	if rd.keys == nil {
		return fmt.Errorf("pase: HandlePake3 before HandlePake1")
	}
	var cA []byte
	if err := decodeFields(raw, map[uint32]*[]byte{1: &cA}); err != nil {
		return err
	}
	if !rd.keys.VerifyCA(cA) {
		return fmt.Errorf("pase: initiator confirmation failed (wrong passcode?)")
	}
	rd.done = true
	return nil
}

// SecureSession returns the established session (keys swapped relative to the
// initiator).
func (rd *Responder) SecureSession() (*session.Secure, error) {
	if !rd.done {
		return nil, fmt.Errorf("pase: handshake not complete")
	}
	i2r, r2i, _, err := sessionKeys(rd.keys.Ke)
	if err != nil {
		return nil, err
	}
	return newSecure(rd.localSessionID, rd.peerSID, r2i, i2r)
}
