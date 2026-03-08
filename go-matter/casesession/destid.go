// Package casesession implements the initiator (controller) side of the Matter
// CASE handshake (Spec 4.14.2): establishing an operational secure session with
// an already-commissioned device using node operational certificates.
//
// All wire structures, key-derivation salts/labels, nonces, and the
// destination-identifier construction are taken verbatim from the Matter
// specification and CHIP (src/protocols/secure_channel/CASESession.cpp and
// CASEDestinationId.cpp) — not guessed. See casesession_test.go and the
// loopback test for validation.
package casesession

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// GenerateDestinationID computes the CASE destination identifier (Spec
// 4.14.2.5.1 / CHIP GenerateCaseDestinationId):
//
//	HMAC-SHA256(IPK, initiatorRandom || rootPublicKey || fabricID(8 LE) || nodeID(8 LE))
//
// fabricID and nodeID are written little-endian (CHIP uses a LittleEndian
// BufferWriter). It lets a responder recognize which fabric/node an incoming
// Sigma1 targets.
func GenerateDestinationID(ipk, initiatorRandom, rootPubKey []byte, fabricID, nodeID uint64) ([]byte, error) {
	if len(ipk) != 16 {
		return nil, fmt.Errorf("casesession: IPK must be 16 bytes, got %d", len(ipk))
	}
	if len(initiatorRandom) != 32 {
		return nil, fmt.Errorf("casesession: initiator random must be 32 bytes, got %d", len(initiatorRandom))
	}
	if len(rootPubKey) != 65 {
		return nil, fmt.Errorf("casesession: root public key must be 65 bytes, got %d", len(rootPubKey))
	}
	msg := make([]byte, 0, 32+65+8+8)
	msg = append(msg, initiatorRandom...)
	msg = append(msg, rootPubKey...)
	msg = binary.LittleEndian.AppendUint64(msg, fabricID)
	msg = binary.LittleEndian.AppendUint64(msg, nodeID)

	m := hmac.New(sha256.New, ipk)
	m.Write(msg)
	return m.Sum(nil), nil
}
