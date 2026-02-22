package crypto

import (
	"crypto/hkdf"
	"crypto/pbkdf2"
	"crypto/sha256"
)

// HKDF derives keyLen bytes from secret using HKDF-SHA256 (RFC 5869). This is
// the key-derivation function used throughout Matter's PASE/CASE key schedule.
func HKDF(secret, salt, info []byte, keyLen int) ([]byte, error) {
	return hkdf.Key(sha256.New, secret, salt, string(info), keyLen)
}

// PBKDF2 derives keyLen bytes using PBKDF2-HMAC-SHA256. Matter uses it in
// SPAKE2+ registration to expand the setup passcode into the w0/w1 scalars.
func PBKDF2(password, salt []byte, iterations, keyLen int) ([]byte, error) {
	return pbkdf2.Key(sha256.New, string(password), salt, iterations, keyLen)
}
