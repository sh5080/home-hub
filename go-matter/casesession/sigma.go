package casesession

import (
	"fmt"

	"github.com/sh5080/go-matter/tlv"
)

// CASE wire structures (Spec 4.14.2.5; CHIP CASESession.cpp tag enums).
//
// Sigma1  = { 1:initiatorRandom, 2:initiatorSessionId, 3:destinationId,
//             4:initiatorEphPubKey, [5:sessionParams, 6:resumptionId, 7:resume1MIC] }
// Sigma2  = { 1:responderRandom, 2:responderSessionId, 3:responderEphPubKey,
//             4:encrypted2, [5:sessionParams] }
// Sigma3  = { 1:encrypted3 }
// TBEData = { 1:senderNOC, [2:senderICAC], 3:signature, [4:resumptionId] }   (encrypted)
// TBSData = { 1:senderNOC, [2:senderICAC], 3:senderPubKey, 4:receiverPubKey } (signed)

func enterStruct(b []byte) (*tlv.Reader, error) {
	r := tlv.NewReader(b)
	if !r.Next() || r.Type() != tlv.TypeStructure {
		return nil, fmt.Errorf("casesession: expected top-level structure")
	}
	if err := r.Enter(); err != nil {
		return nil, err
	}
	return r, nil
}

// Sigma1 is the first CASE message (from the initiator).
type Sigma1 struct {
	InitiatorRandom    []byte
	InitiatorSessionID uint16
	DestinationID      []byte
	InitiatorEphPubKey []byte
}

func (s Sigma1) Encode() ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	w.PutBytes(tlv.Context(1), s.InitiatorRandom)
	w.PutUint(tlv.Context(2), uint64(s.InitiatorSessionID))
	w.PutBytes(tlv.Context(3), s.DestinationID)
	w.PutBytes(tlv.Context(4), s.InitiatorEphPubKey)
	w.EndContainer()
	return w.Bytes()
}

func DecodeSigma1(b []byte) (Sigma1, error) {
	var s Sigma1
	r, err := enterStruct(b)
	if err != nil {
		return s, err
	}
	for r.Next() {
		switch r.Tag().Num {
		case 1:
			s.InitiatorRandom, err = r.Bytes()
		case 2:
			var v uint64
			if v, err = r.Uint(); err == nil {
				s.InitiatorSessionID = uint16(v)
			}
		case 3:
			s.DestinationID, err = r.Bytes()
		case 4:
			s.InitiatorEphPubKey, err = r.Bytes()
			// tags 5/6/7 (session params, resumption) are optional and ignored.
		}
		if err != nil {
			return s, err
		}
	}
	return s, r.Err()
}

// Sigma2 is the second CASE message (from the responder).
type Sigma2 struct {
	ResponderRandom    []byte
	ResponderSessionID uint16
	ResponderEphPubKey []byte
	Encrypted2         []byte
}

func (s Sigma2) Encode() ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	w.PutBytes(tlv.Context(1), s.ResponderRandom)
	w.PutUint(tlv.Context(2), uint64(s.ResponderSessionID))
	w.PutBytes(tlv.Context(3), s.ResponderEphPubKey)
	w.PutBytes(tlv.Context(4), s.Encrypted2)
	w.EndContainer()
	return w.Bytes()
}

func DecodeSigma2(b []byte) (Sigma2, error) {
	var s Sigma2
	r, err := enterStruct(b)
	if err != nil {
		return s, err
	}
	for r.Next() {
		switch r.Tag().Num {
		case 1:
			s.ResponderRandom, err = r.Bytes()
		case 2:
			var v uint64
			if v, err = r.Uint(); err == nil {
				s.ResponderSessionID = uint16(v)
			}
		case 3:
			s.ResponderEphPubKey, err = r.Bytes()
		case 4:
			s.Encrypted2, err = r.Bytes()
		}
		if err != nil {
			return s, err
		}
	}
	return s, r.Err()
}

// Sigma3 is the third CASE message (from the initiator).
type Sigma3 struct{ Encrypted3 []byte }

func (s Sigma3) Encode() ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	w.PutBytes(tlv.Context(1), s.Encrypted3)
	w.EndContainer()
	return w.Bytes()
}

func DecodeSigma3(b []byte) (Sigma3, error) {
	var s Sigma3
	r, err := enterStruct(b)
	if err != nil {
		return s, err
	}
	for r.Next() {
		if r.Tag().Num == 1 {
			if s.Encrypted3, err = r.Bytes(); err != nil {
				return s, err
			}
		}
	}
	return s, r.Err()
}

// tbeData is the plaintext of the encrypted Sigma2/Sigma3 payload.
type tbeData struct {
	NOC       []byte
	ICAC      []byte // optional
	Signature []byte
}

func encodeTBEData(d tbeData) ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	w.PutBytes(tlv.Context(1), d.NOC)
	if len(d.ICAC) > 0 {
		w.PutBytes(tlv.Context(2), d.ICAC)
	}
	w.PutBytes(tlv.Context(3), d.Signature)
	w.EndContainer()
	return w.Bytes()
}

func decodeTBEData(b []byte) (tbeData, error) {
	var d tbeData
	r, err := enterStruct(b)
	if err != nil {
		return d, err
	}
	for r.Next() {
		switch r.Tag().Num {
		case 1:
			d.NOC, err = r.Bytes()
		case 2:
			d.ICAC, err = r.Bytes()
		case 3:
			d.Signature, err = r.Bytes()
			// tag 4 (resumptionId) ignored.
		}
		if err != nil {
			return d, err
		}
	}
	return d, r.Err()
}

// encodeTBSData builds the to-be-signed payload (CHIP ConstructTBSData):
// Structure { 1:senderNOC, [2:senderICAC], 3:senderPubKey, 4:receiverPubKey }.
func encodeTBSData(noc, icac, senderPub, receiverPub []byte) ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	w.PutBytes(tlv.Context(1), noc)
	if len(icac) > 0 {
		w.PutBytes(tlv.Context(2), icac)
	}
	w.PutBytes(tlv.Context(3), senderPub)
	w.PutBytes(tlv.Context(4), receiverPub)
	w.EndContainer()
	return w.Bytes()
}
