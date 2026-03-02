package spake2

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"

	"github.com/sh5080/go-matter/crypto"
)

// KeySchedule holds the outputs of the SPAKE2+ confirmation phase (RFC 9383 §4).
type KeySchedule struct {
	KMain     []byte // Hash(TT)
	ConfirmP  []byte // HMAC(K_confirmP, shareV): the prover's confirmation MAC
	ConfirmV  []byte // HMAC(K_confirmV, shareP): the expected verifier MAC
	SharedKey []byte // Ke, the shared secret handed to the session layer
}

// transcript builds TT (RFC 9383 §3.3): a sequence of elements, each prefixed
// with its 8-byte little-endian length. M and N are included in uncompressed
// form. Order: Context, idProver, idVerifier, M, N, shareP, shareV, Z, V, w0.
func transcript(context, idProver, idVerifier, shareP, shareV, z, v, w0 []byte) []byte {
	var tt []byte
	add := func(b []byte) {
		var l [8]byte
		binary.LittleEndian.PutUint64(l[:], uint64(len(b)))
		tt = append(tt, l[:]...)
		tt = append(tt, b...)
	}
	add(context)
	add(idProver)
	add(idVerifier)
	add(pointM.Bytes())
	add(pointN.Bytes())
	add(shareP)
	add(shareV)
	add(z)
	add(v)
	add(w0)
	return tt
}

func hmacSHA256(key, msg []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(msg)
	return m.Sum(nil)
}

// deriveKeys computes the confirmation MACs and shared key from a completed
// transcript (RFC 9383 §4).
func deriveKeys(context, idProver, idVerifier, shareP, shareV, z, v, w0 []byte) (*KeySchedule, error) {
	tt := transcript(context, idProver, idVerifier, shareP, shareV, z, v, w0)
	kMain := sha256.Sum256(tt)

	ck, err := crypto.HKDF(kMain[:], nil, []byte("ConfirmationKeys"), 64)
	if err != nil {
		return nil, err
	}
	kConfirmP, kConfirmV := ck[:32], ck[32:]

	sharedKey, err := crypto.HKDF(kMain[:], nil, []byte("SharedKey"), 32)
	if err != nil {
		return nil, err
	}

	return &KeySchedule{
		KMain:     kMain[:],
		ConfirmP:  hmacSHA256(kConfirmP, shareV),
		ConfirmV:  hmacSHA256(kConfirmV, shareP),
		SharedKey: sharedKey,
	}, nil
}

// Confirm completes the SPAKE2+ exchange: it derives Z and V from the verifier's
// share pB and produces the confirmation keys under the given application
// context and identities. For Matter PASE the context is defined by Spec 3.10;
// for RFC 9383 interoperability it is the ciphersuite string.
func (p *Prover) Confirm(pB, context, idProver, idVerifier []byte) (*KeySchedule, error) {
	z, v, err := p.Finish(pB)
	if err != nil {
		return nil, err
	}
	return deriveKeys(context, idProver, idVerifier, p.Share(), pB, z, v, scalarBytes(p.w0))
}

// VerifyPeer checks the confirmation MAC received from the verifier in constant
// time.
func (k *KeySchedule) VerifyPeer(confirmV []byte) bool {
	return hmac.Equal(k.ConfirmV, confirmV)
}
