// Package pase implements PASE — passcode-authenticated session establishment
// (Matter Spec 4.14.1) — over SPAKE2+. It is the pairing used during
// commissioning, before the device holds any fabric credentials:
//
//	→ PBKDFParamRequest   (SC opcode 0x20)
//	← PBKDFParamResponse  (0x21)
//	→ Pake1 {pA}          (0x22)
//	← Pake2 {pB, cB}      (0x23)
//	→ Pake3 {cA}          (0x24)
//	← StatusReport SUCCESS → secure session active
//
// Like casesession, this package is transport-agnostic: it produces and
// consumes message payloads; framing/sending is the controller's concern.
// Wire constants are validated against CHIP PASESession.cpp.
package pase

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"

	"github.com/sh5080/go-matter/crypto"
	"github.com/sh5080/go-matter/session"
	"github.com/sh5080/go-matter/tlv"
)

// spake2pContext is the seed of the PASE transcript-context hash
// (CHIP kSpake2pContext).
const spake2pContext = "CHIP PAKE V1 Commissioning"

// paseKeysInfo is the HKDF info for session-key derivation (CHIP SEKeysInfo).
var paseKeysInfo = []byte("SessionKeys")

// PBKDFParams are the SPAKE2+ registration parameters a device announces.
type PBKDFParams struct {
	Iterations int
	Salt       []byte
}

// paramRequest is the decoded PBKDFParamRequest.
type paramRequest struct {
	InitiatorRandom    []byte // 32 bytes
	InitiatorSessionID uint16
	PasscodeID         uint16 // 0 = the standard commissioning passcode
	HasParams          bool   // initiator already knows iterations+salt
}

// encodeParamRequest builds a PBKDFParamRequest payload.
func encodeParamRequest(r paramRequest) ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	w.PutBytes(tlv.Context(1), r.InitiatorRandom)
	w.PutUint(tlv.Context(2), uint64(r.InitiatorSessionID))
	w.PutUint(tlv.Context(3), uint64(r.PasscodeID))
	w.PutBool(tlv.Context(4), r.HasParams)
	w.EndContainer()
	return w.Bytes()
}

func decodeParamRequest(b []byte) (paramRequest, error) {
	var r paramRequest
	rd := tlv.NewReader(b)
	if !rd.Next() || rd.Type() != tlv.TypeStructure {
		return r, fmt.Errorf("pase: PBKDFParamRequest: expected structure")
	}
	if err := rd.Enter(); err != nil {
		return r, err
	}
	for rd.Next() {
		switch rd.Tag().Num {
		case 1:
			v, err := rd.Bytes()
			if err != nil {
				return r, err
			}
			r.InitiatorRandom = v
		case 2:
			v, err := rd.Uint()
			if err != nil {
				return r, err
			}
			r.InitiatorSessionID = uint16(v)
		case 3:
			v, err := rd.Uint()
			if err != nil {
				return r, err
			}
			r.PasscodeID = uint16(v)
		case 4:
			v, err := rd.Bool()
			if err != nil {
				return r, err
			}
			r.HasParams = v
			// 5: initiatorSessionParams (MRP) — tolerated and ignored.
		}
	}
	if len(r.InitiatorRandom) != 32 {
		return r, fmt.Errorf("pase: initiatorRandom is %d bytes, want 32", len(r.InitiatorRandom))
	}
	return r, rd.Err()
}

// paramResponse is the decoded PBKDFParamResponse.
type paramResponse struct {
	InitiatorRandom    []byte
	ResponderRandom    []byte
	ResponderSessionID uint16
	Params             *PBKDFParams // nil when the initiator said HasParams
}

func encodeParamResponse(r paramResponse) ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	w.PutBytes(tlv.Context(1), r.InitiatorRandom)
	w.PutBytes(tlv.Context(2), r.ResponderRandom)
	w.PutUint(tlv.Context(3), uint64(r.ResponderSessionID))
	if r.Params != nil {
		w.StartStructure(tlv.Context(4))
		w.PutUint(tlv.Context(1), uint64(r.Params.Iterations))
		w.PutBytes(tlv.Context(2), r.Params.Salt)
		w.EndContainer()
	}
	w.EndContainer()
	return w.Bytes()
}

func decodeParamResponse(b []byte) (paramResponse, error) {
	var r paramResponse
	rd := tlv.NewReader(b)
	if !rd.Next() || rd.Type() != tlv.TypeStructure {
		return r, fmt.Errorf("pase: PBKDFParamResponse: expected structure")
	}
	if err := rd.Enter(); err != nil {
		return r, err
	}
	for rd.Next() {
		switch rd.Tag().Num {
		case 1:
			v, err := rd.Bytes()
			if err != nil {
				return r, err
			}
			r.InitiatorRandom = v
		case 2:
			v, err := rd.Bytes()
			if err != nil {
				return r, err
			}
			r.ResponderRandom = v
		case 3:
			v, err := rd.Uint()
			if err != nil {
				return r, err
			}
			r.ResponderSessionID = uint16(v)
		case 4:
			if err := rd.Enter(); err != nil {
				return r, err
			}
			var p PBKDFParams
			for rd.Next() {
				switch rd.Tag().Num {
				case 1:
					v, err := rd.Uint()
					if err != nil {
						return r, err
					}
					p.Iterations = int(v)
				case 2:
					v, err := rd.Bytes()
					if err != nil {
						return r, err
					}
					p.Salt = v
				}
			}
			r.Params = &p
		}
	}
	return r, rd.Err()
}

// encodePake1 builds Pake1 {1: pA}.
func encodePake1(pA []byte) ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	w.PutBytes(tlv.Context(1), pA)
	w.EndContainer()
	return w.Bytes()
}

// encodePake2 builds Pake2 {1: pB, 2: cB}.
func encodePake2(pB, cB []byte) ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	w.PutBytes(tlv.Context(1), pB)
	w.PutBytes(tlv.Context(2), cB)
	w.EndContainer()
	return w.Bytes()
}

// encodePake3 builds Pake3 {1: cA}.
func encodePake3(cA []byte) ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	w.PutBytes(tlv.Context(1), cA)
	w.EndContainer()
	return w.Bytes()
}

// decodeFields reads a flat structure of byte-string context fields into out
// (keyed by tag number). Missing keys stay nil.
func decodeFields(b []byte, out map[uint32]*[]byte) error {
	rd := tlv.NewReader(b)
	if !rd.Next() || rd.Type() != tlv.TypeStructure {
		return fmt.Errorf("pase: expected structure")
	}
	if err := rd.Enter(); err != nil {
		return err
	}
	for rd.Next() {
		if dst, ok := out[rd.Tag().Num]; ok {
			v, err := rd.Bytes()
			if err != nil {
				return err
			}
			*dst = v
		}
	}
	return rd.Err()
}

// contextHash builds the SPAKE2+ context: SHA-256 over the context string and
// the raw PBKDFParamRequest/Response payloads (CHIP mCommissioningHash).
func contextHash(reqRaw, respRaw []byte) []byte {
	h := sha256.New()
	h.Write([]byte(spake2pContext))
	h.Write(reqRaw)
	h.Write(respRaw)
	return h.Sum(nil)
}

// randomScalar returns a uniformly random scalar in [1, n-1].
func randomScalar(order *big.Int) (*big.Int, error) {
	for {
		k, err := rand.Int(rand.Reader, order)
		if err != nil {
			return nil, err
		}
		if k.Sign() > 0 {
			return k, nil
		}
	}
}

// random32 returns 32 cryptographically random bytes.
func random32() ([]byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// sessionKeys derives the PASE session keys from Ke:
// HKDF(Ke, salt=nil, "SessionKeys", 48) → I2R(16) ‖ R2I(16) ‖ AttestationChallenge(16).
func sessionKeys(ke []byte) (i2r, r2i, challenge []byte, err error) {
	keys, err := crypto.HKDF(ke, nil, paseKeysInfo, 48)
	if err != nil {
		return nil, nil, nil, err
	}
	return keys[:16], keys[16:32], keys[32:], nil
}

// newSecure builds the PASE secure session. PASE peers have no operational
// node ids (Spec 4.14: SourceNodeID absent → 0 in the nonce).
func newSecure(localID, peerID uint16, sendKey, recvKey []byte) (*session.Secure, error) {
	return session.NewSecure(localID, peerID, 0, 0, sendKey, recvKey, 0)
}
