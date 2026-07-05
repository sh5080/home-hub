package pase

import (
	"crypto/elliptic"
	"fmt"

	"github.com/sh5080/go-matter/session"
	"github.com/sh5080/go-matter/spake2"
)

// Initiator is the commissioner side of PASE. Construct with NewInitiator,
// then drive the message sequence:
//
//	ParamRequest → HandleParamResponse → (Pake1) → HandlePake2 → (Pake3)
//	→ [peer StatusReport SUCCESS] → SecureSession
type Initiator struct {
	passcode       uint32
	localSessionID uint16

	reqRaw  []byte // PBKDFParamRequest bytes (context hash input)
	context []byte // SHA-256(spake2pContext ‖ reqRaw ‖ respRaw)
	prover  *spake2.Prover
	keys    *spake2.MatterKeys
	peerSID uint16
	done    bool
}

// NewInitiator prepares a PASE initiator for the given setup passcode.
// localSessionID is the session id the peer must address us with.
func NewInitiator(passcode uint32, localSessionID uint16) *Initiator {
	return &Initiator{passcode: passcode, localSessionID: localSessionID}
}

// ParamRequest builds the PBKDFParamRequest payload (SC opcode 0x20).
func (in *Initiator) ParamRequest() ([]byte, error) {
	r, err := random32()
	if err != nil {
		return nil, err
	}
	raw, err := encodeParamRequest(paramRequest{
		InitiatorRandom:    r,
		InitiatorSessionID: in.localSessionID,
		PasscodeID:         0, // standard commissioning passcode
		HasParams:          false,
	})
	if err != nil {
		return nil, err
	}
	in.reqRaw = raw
	return raw, nil
}

// HandleParamResponse consumes the PBKDFParamResponse (0x21) and returns the
// Pake1 payload (0x22). This runs the PBKDF2 (deliberately slow) and the first
// SPAKE2+ point operations.
func (in *Initiator) HandleParamResponse(raw []byte) ([]byte, error) {
	if in.reqRaw == nil {
		return nil, fmt.Errorf("pase: HandleParamResponse before ParamRequest")
	}
	resp, err := decodeParamResponse(raw)
	if err != nil {
		return nil, err
	}
	if resp.Params == nil {
		return nil, fmt.Errorf("pase: responder omitted PBKDF parameters")
	}
	in.peerSID = resp.ResponderSessionID

	w0, w1, err := spake2.DeriveW(in.passcode, resp.Params.Salt, resp.Params.Iterations)
	if err != nil {
		return nil, err
	}
	x, err := randomScalar(elliptic.P256().Params().N)
	if err != nil {
		return nil, err
	}
	prover, err := spake2.NewProver(w0, w1, x)
	if err != nil {
		return nil, err
	}
	in.prover = prover
	in.context = contextHash(in.reqRaw, raw)
	return encodePake1(prover.Share())
}

// HandlePake2 consumes Pake2 (0x23), verifies the responder's confirmation cB,
// and returns the Pake3 payload (0x24). A wrong passcode on either side fails
// here.
func (in *Initiator) HandlePake2(raw []byte) ([]byte, error) {
	if in.prover == nil {
		return nil, fmt.Errorf("pase: HandlePake2 before HandleParamResponse")
	}
	var pB, cB []byte
	if err := decodeFields(raw, map[uint32]*[]byte{1: &pB, 2: &cB}); err != nil {
		return nil, err
	}
	keys, err := in.prover.ConfirmMatter(pB, in.context)
	if err != nil {
		return nil, err
	}
	if !keys.VerifyCB(cB) {
		return nil, fmt.Errorf("pase: responder confirmation failed (wrong passcode?)")
	}
	in.keys = keys
	in.done = true
	return encodePake3(keys.CA)
}

// SecureSession returns the established session after HandlePake2 succeeded
// and the peer confirmed with a SUCCESS StatusReport.
func (in *Initiator) SecureSession() (*session.Secure, error) {
	if !in.done {
		return nil, fmt.Errorf("pase: handshake not complete")
	}
	i2r, r2i, _, err := sessionKeys(in.keys.Ke)
	if err != nil {
		return nil, err
	}
	return newSecure(in.localSessionID, in.peerSID, i2r, r2i)
}

// AttestationChallenge returns the 16-byte challenge bound to this session,
// used by the device-attestation step of commissioning.
func (in *Initiator) AttestationChallenge() ([]byte, error) {
	if !in.done {
		return nil, fmt.Errorf("pase: handshake not complete")
	}
	_, _, challenge, err := sessionKeys(in.keys.Ke)
	return challenge, err
}
