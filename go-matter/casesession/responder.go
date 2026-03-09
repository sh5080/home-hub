package casesession

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"hash"

	"github.com/sh5080/go-matter/cert"
	"github.com/sh5080/go-matter/crypto"
)

// Responder implements the device side of the CASE handshake. The hub is only
// ever an initiator; this type exists to validate the initiator end-to-end via
// loopback and as a precise, self-checking reference for the protocol.
type Responder struct {
	fabric         Fabric
	self           Identity
	localSessionID uint16

	responderRandom []byte
	eph             *ecdh.PrivateKey
	hash            hash.Hash
	initiatorEph    []byte
	sharedSecret    []byte
}

// NewResponder prepares a device-side handshake. localSessionID is the id the
// initiator will stamp on messages sent to us.
func NewResponder(fabric Fabric, self Identity, localSessionID uint16) (*Responder, error) {
	if len(fabric.IPK) != 16 {
		return nil, fmt.Errorf("casesession: IPK must be 16 bytes")
	}
	return &Responder{fabric: fabric, self: self, localSessionID: localSessionID, hash: sha256.New()}, nil
}

// HandleSigma1 processes Sigma1 and returns Sigma2.
func (rd *Responder) HandleSigma1(sigma1Bytes []byte) ([]byte, error) {
	s1, err := DecodeSigma1(sigma1Bytes)
	if err != nil {
		return nil, err
	}
	rd.initiatorEph = s1.InitiatorEphPubKey
	rd.hash.Write(sigma1Bytes) // transcript covers Sigma1

	// TODO(responder): verify s1.DestinationID resolves to one of our
	// fabric/node pairs via GenerateDestinationID (single test fabric here).

	rd.responderRandom = make([]byte, 32)
	if _, err = rand.Read(rd.responderRandom); err != nil {
		return nil, err
	}
	if rd.eph, err = ecdh.P256().GenerateKey(rand.Reader); err != nil {
		return nil, err
	}
	initEph, err := ecdh.P256().NewPublicKey(s1.InitiatorEphPubKey)
	if err != nil {
		return nil, fmt.Errorf("casesession: initiator eph key: %w", err)
	}
	if rd.sharedSecret, err = rd.eph.ECDH(initEph); err != nil {
		return nil, err
	}
	respEph := rd.eph.PublicKey().Bytes()

	tbs, err := encodeTBSData(rd.self.NOC, rd.self.ICAC, respEph, s1.InitiatorEphPubKey)
	if err != nil {
		return nil, err
	}
	sig, err := crypto.SignECDSA(rd.self.OpKey, tbs)
	if err != nil {
		return nil, err
	}
	tbe2, err := encodeTBEData(tbeData{NOC: rd.self.NOC, ICAC: rd.self.ICAC, Signature: sig})
	if err != nil {
		return nil, err
	}

	sigma1Hash := rd.hash.Sum(nil) // SHA256(Sigma1)
	sr2k, err := deriveSigmaKey(rd.sharedSecret,
		saltSigma2(rd.fabric.IPK, rd.responderRandom, respEph, sigma1Hash), infoSigma2)
	if err != nil {
		return nil, err
	}
	enc2, err := encryptTBE(sr2k, nonceTBE2, tbe2)
	if err != nil {
		return nil, err
	}

	msg, err := Sigma2{
		ResponderRandom:    rd.responderRandom,
		ResponderSessionID: rd.localSessionID,
		ResponderEphPubKey: respEph,
		Encrypted2:         enc2,
	}.Encode()
	if err != nil {
		return nil, err
	}
	rd.hash.Write(msg) // transcript covers Sigma1||Sigma2
	return msg, nil
}

// HandleSigma3 processes the final message and authenticates the initiator.
func (rd *Responder) HandleSigma3(sigma3Bytes []byte) error {
	s3, err := DecodeSigma3(sigma3Bytes)
	if err != nil {
		return err
	}
	sigma12Hash := rd.hash.Sum(nil) // SHA256(Sigma1||Sigma2)
	sr3k, err := deriveSigmaKey(rd.sharedSecret, saltSigma3(rd.fabric.IPK, sigma12Hash), infoSigma3)
	if err != nil {
		return err
	}
	tbe3, err := decryptTBE(sr3k, nonceTBE3, s3.Encrypted3)
	if err != nil {
		return fmt.Errorf("casesession: decrypt sigma3: %w", err)
	}

	initNOC, err := cert.Decode(tbe3.NOC)
	if err != nil {
		return err
	}
	var initICAC *cert.Cert
	if len(tbe3.ICAC) > 0 {
		if initICAC, err = cert.Decode(tbe3.ICAC); err != nil {
			return err
		}
	}
	if err := cert.VerifyChain(initNOC, initICAC, rd.fabric.RCAC); err != nil {
		return fmt.Errorf("casesession: initiator chain: %w", err)
	}
	tbs, err := encodeTBSData(tbe3.NOC, tbe3.ICAC, rd.initiatorEph, rd.eph.PublicKey().Bytes())
	if err != nil {
		return err
	}
	if err := crypto.VerifyECDSA(initNOC.PublicKey, tbs, tbe3.Signature); err != nil {
		return fmt.Errorf("casesession: initiator signature: %w", err)
	}

	rd.hash.Write(sigma3Bytes) // transcript covers Sigma1||Sigma2||Sigma3
	return nil
}

// SessionKeys returns the operational session keys once the handshake completes.
func (rd *Responder) SessionKeys() (*SessionKeys, error) {
	if rd.sharedSecret == nil {
		return nil, fmt.Errorf("casesession: handshake not complete")
	}
	return deriveSessionKeys(rd.sharedSecret, rd.fabric.IPK, rd.hash.Sum(nil))
}
