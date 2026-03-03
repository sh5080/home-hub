// Package session encrypts and decrypts Matter messages over an established
// PASE/CASE secure session (Spec 4.6–4.8). Keys come from the handshake; this
// package owns nonce construction, AEAD sealing, and (with the replay window)
// counter management.
package session

import (
	"crypto/cipher"
	"encoding/binary"
	"fmt"

	"github.com/sh5080/go-matter/crypto"
	"github.com/sh5080/go-matter/message"
)

// nonce builds the 13-byte AES-CCM nonce (Spec 4.7.2):
// securityFlags(1) || messageCounter(4 LE) || sourceNodeID(8 LE).
func nonce(securityFlags byte, counter uint32, sourceNodeID uint64) []byte {
	n := make([]byte, crypto.CCMNonceSize)
	n[0] = securityFlags
	binary.LittleEndian.PutUint32(n[1:], counter)
	binary.LittleEndian.PutUint64(n[5:], sourceNodeID)
	return n
}

// Secure is an established, keyed unicast session. It is not safe for concurrent
// use; callers (or the Table) serialize access.
type Secure struct {
	LocalSessionID uint16 // the id peers put in messages addressed to us
	PeerSessionID  uint16 // the id we put in messages addressed to the peer
	LocalNodeID    uint64 // our operational node id (0 for PASE)
	PeerNodeID     uint64 // the peer's node id (0 for PASE)

	send cipher.AEAD // our transmit key (I2R for a controller)
	recv cipher.AEAD // our receive key (R2I for a controller)

	txCounter uint32
	rx        replayWindow
}

// NewSecure builds a session from its identifiers and directional keys. A
// controller passes sendKey = I2R and recvKey = R2I; a device passes them
// swapped. txStart is the initial (randomized) transmit counter.
func NewSecure(localID, peerID uint16, localNode, peerNode uint64, sendKey, recvKey []byte, txStart uint32) (*Secure, error) {
	send, err := crypto.NewCCM(sendKey)
	if err != nil {
		return nil, fmt.Errorf("session: send key: %w", err)
	}
	recv, err := crypto.NewCCM(recvKey)
	if err != nil {
		return nil, fmt.Errorf("session: recv key: %w", err)
	}
	return &Secure{
		LocalSessionID: localID, PeerSessionID: peerID,
		LocalNodeID: localNode, PeerNodeID: peerNode,
		send: send, recv: recv, txCounter: txStart,
	}, nil
}

// Encrypt seals a protocol payload into a complete wire message, stamping and
// advancing the transmit counter.
func (s *Secure) Encrypt(payload []byte) ([]byte, error) {
	counter := s.txCounter
	s.txCounter++

	hdr := message.Header{
		SessionID:   s.PeerSessionID,
		SessionType: message.Unicast,
		Counter:     counter,
	}
	if s.LocalNodeID != 0 {
		hdr.SourceNodeID = s.LocalNodeID
		hdr.SourcePresent = true
	}
	aad, err := hdr.Encode()
	if err != nil {
		return nil, err
	}
	ct := s.send.Seal(nil, nonce(aad[3], counter, s.LocalNodeID), payload, aad)
	return append(aad, ct...), nil
}

// Decrypt authenticates and decrypts a wire message addressed to this session.
func (s *Secure) Decrypt(frame []byte) ([]byte, error) {
	hdr, rest, err := message.Decode(frame)
	if err != nil {
		return nil, err
	}
	if hdr.SessionID != s.LocalSessionID {
		return nil, fmt.Errorf("session: message for session %d, expected %d", hdr.SessionID, s.LocalSessionID)
	}
	aad := frame[:len(frame)-len(rest)]
	pt, err := s.recv.Open(nil, nonce(aad[3], hdr.Counter, s.PeerNodeID), rest, aad)
	if err != nil {
		return nil, err
	}
	if err := s.rx.accept(hdr.Counter); err != nil {
		return nil, err
	}
	return pt, nil
}
