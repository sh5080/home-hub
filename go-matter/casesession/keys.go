package casesession

import "github.com/sh5080/go-matter/crypto"

// Key-derivation info labels (CHIP CASESession.cpp kKDFSR2Info/kKDFSR3Info and
// CryptoContext.cpp SEKeysInfo).
var (
	infoSigma2      = []byte("Sigma2")
	infoSigma3      = []byte("Sigma3")
	infoSessionKeys = []byte("SessionKeys")
)

// AEAD nonces for the Sigma2/Sigma3 encrypted payloads (CHIP kTBEData2_Nonce /
// kTBEData3_Nonce). These are exactly 13 bytes (the AES-CCM nonce size).
var (
	nonceTBE2 = []byte("NCASE_Sigma2N")
	nonceTBE3 = []byte("NCASE_Sigma3N")
)

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// saltSigma2 = IPK || responderRandom || responderEphPubKey || SHA256(Sigma1)
// (CHIP ConstructSaltSigma2).
func saltSigma2(ipk, responderRandom, responderEphPub, sigma1Hash []byte) []byte {
	return concat(ipk, responderRandom, responderEphPub, sigma1Hash)
}

// saltSigma3 = IPK || SHA256(Sigma1 || Sigma2) (CHIP ConstructSaltSigma3).
func saltSigma3(ipk, transcriptHash []byte) []byte {
	return concat(ipk, transcriptHash)
}

// deriveSigmaKey = HKDF-SHA256(sharedSecret, salt, info) truncated to a 16-byte
// AES-128 key (CHIP DeriveSigmaKey -> DeriveKey).
func deriveSigmaKey(sharedSecret, salt, info []byte) ([]byte, error) {
	return crypto.HKDF(sharedSecret, salt, info, 16)
}

// SessionKeys are the directional keys established by CASE.
type SessionKeys struct {
	I2R                  []byte // initiator-to-responder (controller transmit)
	R2I                  []byte // responder-to-initiator (controller receive)
	AttestationChallenge []byte
}

// deriveSessionKeys = HKDF(sharedSecret, IPK || SHA256(Sigma1||Sigma2||Sigma3),
// "SessionKeys", 48) split into I2R || R2I || AttestationChallenge (CHIP
// CryptoContext::InitFromSecret with SEKeysInfo).
func deriveSessionKeys(sharedSecret, ipk, transcriptHash []byte) (*SessionKeys, error) {
	out, err := crypto.HKDF(sharedSecret, concat(ipk, transcriptHash), infoSessionKeys, 48)
	if err != nil {
		return nil, err
	}
	return &SessionKeys{I2R: out[0:16], R2I: out[16:32], AttestationChallenge: out[32:48]}, nil
}
