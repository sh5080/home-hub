// Package crypto implements the Matter cryptographic primitives (Spec §3):
// AES-128-CCM, HKDF / PBKDF2, SPAKE2+, and raw-format ECDSA / ECDH over P-256.
//
// Only primitives absent from the Go standard library are implemented from
// scratch (AES-CCM and SPAKE2+); the remainder wrap crypto/* and
// golang.org/x/crypto. Every primitive is checked against published test
// vectors (NIST, RFC 3610, RFC 9383).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
)

// Matter AES-CCM parameters (Spec 3.6): 128-bit key, 13-byte nonce, 16-byte tag.
const (
	CCMNonceSize = 13
	CCMTagSize   = 16
)

// ccm is a parameterized AES-CCM (NIST SP 800-38C). The public constructor
// fixes Matter's parameters; the internal constructor keeps nonce/tag lengths
// configurable so the implementation can be validated against RFC 3610 vectors.
type ccm struct {
	block    cipher.Block
	nonceLen int
	tagLen   int
}

// NewCCM returns an AES-128-CCM AEAD using Matter's parameters. It implements
// cipher.AEAD; Seal/Open panic on misuse (wrong nonce length) per that contract.
func NewCCM(key []byte) (cipher.AEAD, error) {
	return newCCM(key, CCMNonceSize, CCMTagSize)
}

func newCCM(key []byte, nonceLen, tagLen int) (*ccm, error) {
	if len(key) != 16 {
		return nil, fmt.Errorf("crypto: AES-CCM key must be 16 bytes, got %d", len(key))
	}
	if nonceLen < 7 || nonceLen > 13 {
		return nil, fmt.Errorf("crypto: AES-CCM nonce length %d out of range [7,13]", nonceLen)
	}
	if tagLen < 4 || tagLen > 16 || tagLen%2 != 0 {
		return nil, fmt.Errorf("crypto: AES-CCM tag length %d invalid", tagLen)
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return &ccm{block: b, nonceLen: nonceLen, tagLen: tagLen}, nil
}

func (c *ccm) NonceSize() int { return c.nonceLen }
func (c *ccm) Overhead() int  { return c.tagLen }

// lenField is the size in bytes of the CCM length field, L = 15 - nonceLen.
func (c *ccm) lenField() int { return 15 - c.nonceLen }

func (c *ccm) maxPayload() uint64 {
	if l := c.lenField(); l < 8 {
		return (uint64(1) << (8 * l)) - 1
	}
	return ^uint64(0)
}

// mac computes the CBC-MAC T over aad and plaintext (before CTR masking).
func (c *ccm) mac(nonce, aad, plaintext []byte) []byte {
	l := c.lenField()
	var x [16]byte // X_0 = 0

	// B_0 = flags || nonce || Q, where Q is the plaintext length, l bytes BE.
	var b0 [16]byte
	flags := byte(l - 1)
	flags |= byte((c.tagLen-2)/2) << 3
	if len(aad) > 0 {
		flags |= 1 << 6 // Adata
	}
	b0[0] = flags
	copy(b0[1:], nonce)
	putBE(b0[1+c.nonceLen:], uint64(len(plaintext)), l)
	c.macBlock(&x, b0[:])

	// AAD: a 2-byte big-endian length prefix (valid for len < 2^16 - 2^8),
	// followed by the AAD, zero-padded to a block boundary.
	if len(aad) > 0 {
		var prefix [2]byte
		binary.BigEndian.PutUint16(prefix[:], uint16(len(aad)))
		c.macRegion(&x, prefix[:], aad)
	}

	// Payload, zero-padded to a block boundary (a separate region from AAD).
	c.macRegion(&x, plaintext)

	out := make([]byte, c.tagLen)
	copy(out, x[:])
	return out
}

func (c *ccm) macBlock(x *[16]byte, blk []byte) {
	for i := 0; i < 16; i++ {
		x[i] ^= blk[i]
	}
	c.block.Encrypt(x[:], x[:])
}

// macRegion feeds the concatenation of parts into the CBC-MAC, zero-padding the
// final block of the region.
func (c *ccm) macRegion(x *[16]byte, parts ...[]byte) {
	var buf []byte
	for _, p := range parts {
		buf = append(buf, p...)
	}
	for i := 0; i < len(buf); i += 16 {
		var blk [16]byte
		copy(blk[:], buf[i:])
		c.macBlock(x, blk[:])
	}
}

// ctr applies AES-CTR keystream (counter blocks A_start, A_start+1, ...) to src.
func (c *ccm) ctr(nonce []byte, start uint64, dst, src []byte) {
	l := c.lenField()
	var a, s [16]byte
	a[0] = byte(l - 1)
	copy(a[1:], nonce)
	counter := start
	for i := 0; i < len(src); i += 16 {
		putBE(a[1+c.nonceLen:], counter, l)
		c.block.Encrypt(s[:], a[:])
		n := len(src) - i
		if n > 16 {
			n = 16
		}
		for j := 0; j < n; j++ {
			dst[i+j] = src[i+j] ^ s[j]
		}
		counter++
	}
}

// keystream0 is S_0 = E(K, A_0), used to mask the authentication tag.
func (c *ccm) keystream0(nonce []byte) [16]byte {
	l := c.lenField()
	var a, s [16]byte
	a[0] = byte(l - 1)
	copy(a[1:], nonce)
	c.block.Encrypt(s[:], a[:]) // counter field is already zero
	return s
}

// Seal encrypts and authenticates plaintext, appending the result to dst.
func (c *ccm) Seal(dst, nonce, plaintext, aad []byte) []byte {
	if len(nonce) != c.nonceLen {
		panic("crypto: incorrect CCM nonce length")
	}
	if len(aad) >= 0xFF00 {
		panic("crypto: CCM additional data too large")
	}
	if uint64(len(plaintext)) > c.maxPayload() {
		panic("crypto: CCM plaintext too large")
	}
	tag := c.mac(nonce, aad, plaintext)
	s0 := c.keystream0(nonce)
	for i := 0; i < c.tagLen; i++ {
		tag[i] ^= s0[i]
	}
	ret, out := sliceForAppend(dst, len(plaintext)+c.tagLen)
	c.ctr(nonce, 1, out[:len(plaintext)], plaintext)
	copy(out[len(plaintext):], tag)
	return ret
}

// Open authenticates and decrypts ciphertext, appending the plaintext to dst.
func (c *ccm) Open(dst, nonce, ciphertext, aad []byte) ([]byte, error) {
	if len(nonce) != c.nonceLen {
		return nil, errors.New("crypto: incorrect CCM nonce length")
	}
	if len(ciphertext) < c.tagLen {
		return nil, errors.New("crypto: CCM ciphertext too short")
	}
	if len(aad) >= 0xFF00 {
		return nil, errors.New("crypto: CCM additional data too large")
	}
	ctLen := len(ciphertext) - c.tagLen
	ct, recvTag := ciphertext[:ctLen], ciphertext[ctLen:]

	plaintext := make([]byte, ctLen)
	c.ctr(nonce, 1, plaintext, ct)

	tag := c.mac(nonce, aad, plaintext)
	s0 := c.keystream0(nonce)
	for i := 0; i < c.tagLen; i++ {
		tag[i] ^= s0[i]
	}
	if subtle.ConstantTimeCompare(tag, recvTag) != 1 {
		return nil, errors.New("crypto: CCM authentication failed")
	}
	ret, out := sliceForAppend(dst, ctLen)
	copy(out, plaintext)
	return ret, nil
}

// putBE writes v into the last n bytes of dst in big-endian order.
func putBE(dst []byte, v uint64, n int) {
	for i := 0; i < n; i++ {
		dst[n-1-i] = byte(v >> (8 * i))
	}
}

func sliceForAppend(in []byte, n int) (head, tail []byte) {
	if total := len(in) + n; cap(in) >= total {
		head = in[:total]
	} else {
		head = make([]byte, total)
		copy(head, in)
	}
	return head, head[len(in):]
}
