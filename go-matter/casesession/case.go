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

// Fabric identifies the operational fabric the controller belongs to.
type Fabric struct {
	IPK        []byte     // 16-byte identity protection key
	FabricID   uint64     // operational fabric id
	RootPubKey []byte     // RCAC public key (65-byte uncompressed point)
	RCAC       *cert.Cert // decoded root certificate (trust anchor)
}

// Identity is a node's operational credential: its NOC (and optional ICAC) in
// Matter TLV form plus the corresponding private key.
type Identity struct {
	NOC   []byte // TLV certificate
	ICAC  []byte // optional TLV certificate
	OpKey []byte // 32-byte P-256 private scalar
}

// Initiator drives the controller side of the CASE handshake (Spec 4.14.2).
// A handshake is: Sigma1() -> peer -> HandleSigma2() -> peer -> SessionKeys().
type Initiator struct {
	fabric         Fabric
	self           Identity
	peerNodeID     uint64
	localSessionID uint16

	initRandom []byte
	eph        *ecdh.PrivateKey
	hash       hash.Hash // running SHA-256 over Sigma1||Sigma2||Sigma3

	sharedSecret []byte
	sigma2       Sigma2
}

// NewInitiator prepares a handshake toward peerNodeID. localSessionID is the
// session id the peer will stamp on messages it sends back to us.
func NewInitiator(fabric Fabric, self Identity, peerNodeID uint64, localSessionID uint16) (*Initiator, error) {
	if len(fabric.IPK) != 16 {
		return nil, fmt.Errorf("casesession: IPK must be 16 bytes")
	}
	return &Initiator{
		fabric: fabric, self: self, peerNodeID: peerNodeID,
		localSessionID: localSessionID, hash: sha256.New(),
	}, nil
}

// Sigma1 produces the first handshake message and starts the transcript hash.
func (in *Initiator) Sigma1() ([]byte, error) {
	in.initRandom = make([]byte, 32)
	if _, err := rand.Read(in.initRandom); err != nil {
		return nil, err
	}
	eph, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	in.eph = eph

	destID, err := GenerateDestinationID(in.fabric.IPK, in.initRandom, in.fabric.RootPubKey, in.fabric.FabricID, in.peerNodeID)
	if err != nil {
		return nil, err
	}
	msg, err := Sigma1{
		InitiatorRandom:    in.initRandom,
		InitiatorSessionID: in.localSessionID,
		DestinationID:      destID,
		InitiatorEphPubKey: eph.PublicKey().Bytes(),
	}.Encode()
	if err != nil {
		return nil, err
	}
	in.hash.Write(msg) // transcript covers Sigma1
	return msg, nil
}

// HandleSigma2 validates the responder's message and returns Sigma3.
func (in *Initiator) HandleSigma2(sigma2Bytes []byte) ([]byte, error) {
	s2, err := DecodeSigma2(sigma2Bytes)
	if err != nil {
		return nil, err
	}
	in.sigma2 = s2

	peerEph, err := ecdh.P256().NewPublicKey(s2.ResponderEphPubKey)
	if err != nil {
		return nil, fmt.Errorf("casesession: responder eph key: %w", err)
	}
	if in.sharedSecret, err = in.eph.ECDH(peerEph); err != nil {
		return nil, err
	}

	sigma1Hash := in.hash.Sum(nil) // SHA256(Sigma1)
	sr2k, err := deriveSigmaKey(in.sharedSecret,
		saltSigma2(in.fabric.IPK, s2.ResponderRandom, s2.ResponderEphPubKey, sigma1Hash), infoSigma2)
	if err != nil {
		return nil, err
	}
	tbe2, err := decryptTBE(sr2k, nonceTBE2, s2.Encrypted2)
	if err != nil {
		return nil, fmt.Errorf("casesession: decrypt sigma2: %w", err)
	}

	respNOC, err := cert.Decode(tbe2.NOC)
	if err != nil {
		return nil, fmt.Errorf("casesession: responder NOC: %w", err)
	}
	// The responder signed TBSData over {NOC,[ICAC],responderEph,initiatorEph}.
	if err := in.verifyPeer(respNOC, tbe2, s2.ResponderEphPubKey, in.eph.PublicKey().Bytes()); err != nil {
		return nil, err
	}

	in.hash.Write(sigma2Bytes) // transcript covers Sigma1||Sigma2
	return in.buildSigma3()
}

func (in *Initiator) verifyPeer(respNOC *cert.Cert, tbe tbeData, respEph, initEph []byte) error {
	var respICAC *cert.Cert
	if len(tbe.ICAC) > 0 {
		var err error
		if respICAC, err = cert.Decode(tbe.ICAC); err != nil {
			return err
		}
	}
	if err := cert.VerifyChain(respNOC, respICAC, in.fabric.RCAC); err != nil {
		return fmt.Errorf("casesession: responder chain: %w", err)
	}
	tbs, err := encodeTBSData(tbe.NOC, tbe.ICAC, respEph, initEph)
	if err != nil {
		return err
	}
	if err := crypto.VerifyECDSA(respNOC.PublicKey, tbs, tbe.Signature); err != nil {
		return fmt.Errorf("casesession: responder signature: %w", err)
	}
	if id, ok := respNOC.Subject.NodeID(); !ok || id != in.peerNodeID {
		return fmt.Errorf("casesession: responder node id mismatch (got %#x want %#x)", id, in.peerNodeID)
	}
	if fid, ok := respNOC.Subject.FabricID(); !ok || fid != in.fabric.FabricID {
		return fmt.Errorf("casesession: responder fabric id mismatch")
	}
	return nil
}

func (in *Initiator) buildSigma3() ([]byte, error) {
	initEph := in.eph.PublicKey().Bytes()
	respEph := in.sigma2.ResponderEphPubKey

	tbs, err := encodeTBSData(in.self.NOC, in.self.ICAC, initEph, respEph)
	if err != nil {
		return nil, err
	}
	sig, err := crypto.SignECDSA(in.self.OpKey, tbs)
	if err != nil {
		return nil, err
	}
	tbe3, err := encodeTBEData(tbeData{NOC: in.self.NOC, ICAC: in.self.ICAC, Signature: sig})
	if err != nil {
		return nil, err
	}

	sigma12Hash := in.hash.Sum(nil) // SHA256(Sigma1||Sigma2)
	sr3k, err := deriveSigmaKey(in.sharedSecret, saltSigma3(in.fabric.IPK, sigma12Hash), infoSigma3)
	if err != nil {
		return nil, err
	}
	enc3, err := encryptTBE(sr3k, nonceTBE3, tbe3)
	if err != nil {
		return nil, err
	}
	msg, err := Sigma3{Encrypted3: enc3}.Encode()
	if err != nil {
		return nil, err
	}
	in.hash.Write(msg) // transcript covers Sigma1||Sigma2||Sigma3
	return msg, nil
}

// SessionKeys returns the operational session keys once the handshake is
// complete (after HandleSigma2 has returned Sigma3).
func (in *Initiator) SessionKeys() (*SessionKeys, error) {
	if in.sharedSecret == nil {
		return nil, fmt.Errorf("casesession: handshake not complete")
	}
	return deriveSessionKeys(in.sharedSecret, in.fabric.IPK, in.hash.Sum(nil))
}

// encryptTBE seals a TBE payload (empty additional data, per Matter).
func encryptTBE(key, nonce, plaintext []byte) ([]byte, error) {
	aead, err := crypto.NewCCM(key)
	if err != nil {
		return nil, err
	}
	return aead.Seal(nil, nonce, plaintext, nil), nil
}

func decryptTBE(key, nonce, ciphertext []byte) (tbeData, error) {
	aead, err := crypto.NewCCM(key)
	if err != nil {
		return tbeData{}, err
	}
	pt, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return tbeData{}, err
	}
	return decodeTBEData(pt)
}
