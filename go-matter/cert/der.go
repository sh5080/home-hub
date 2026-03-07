package cert

// Minimal DER (ASN.1 Distinguished Encoding Rules) primitives, sufficient to
// re-encode the X.509 TBSCertificate that a Matter certificate's signature is
// computed over. Correctness is validated byte-for-byte against real CHIP
// reference certificates (see vectors_test.go / verify_test.go).

// derLen encodes a DER definite length.
func derLen(n int) []byte {
	if n < 0x80 {
		return []byte{byte(n)}
	}
	var tmp []byte
	for n > 0 {
		tmp = append([]byte{byte(n)}, tmp...)
		n >>= 8
	}
	return append([]byte{0x80 | byte(len(tmp))}, tmp...)
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// derTLV builds a primitive/constructed element: tag || length || content.
func derTLV(tag byte, content []byte) []byte {
	out := append([]byte{tag}, derLen(len(content))...)
	return append(out, content...)
}

func derSeq(parts ...[]byte) []byte { return derTLV(0x30, concat(parts...)) }
func derSet(parts ...[]byte) []byte { return derTLV(0x31, concat(parts...)) }
func derOID(oid []byte) []byte      { return derTLV(0x06, oid) }
func derOctet(content []byte) []byte { return derTLV(0x04, content) }
func derUTF8(s string) []byte       { return derTLV(0x0c, []byte(s)) }
func derPrintable(s string) []byte  { return derTLV(0x13, []byte(s)) }

func derBool(b bool) []byte {
	if b {
		return derTLV(0x01, []byte{0xFF})
	}
	return derTLV(0x01, []byte{0x00})
}

// derBitString wraps content in a BIT STRING with the given unused-bit count.
func derBitString(unused byte, content []byte) []byte {
	return derTLV(0x03, append([]byte{unused}, content...))
}

// derInt encodes a big-endian unsigned magnitude as a DER INTEGER, stripping
// leading zeros and prepending 0x00 when the high bit is set (to stay positive).
func derInt(mag []byte) []byte {
	i := 0
	for i < len(mag)-1 && mag[i] == 0 {
		i++
	}
	m := mag[i:]
	if len(m) > 0 && m[0]&0x80 != 0 {
		m = append([]byte{0x00}, m...)
	}
	return derTLV(0x02, m)
}
