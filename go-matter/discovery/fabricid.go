// Package discovery resolves Matter devices on the local network and derives
// the identifiers used to address them in operational (mDNS/DNS-SD) discovery.
package discovery

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/sh5080/go-matter/crypto"
)

// info label for the compressed fabric id KDF (CHIP kCompressedFabricInfo).
var compressedFabricInfo = []byte("CompressedFabric")

// CompressedFabricID derives the 8-byte compressed fabric identifier (Spec
// 4.3.2.4 / CHIP GenerateCompressedFabricId):
//
//	HKDF-SHA256(IKM = rootPublicKey[1:] (uncompressed point minus the 0x04
//	prefix), salt = fabricID big-endian, info = "CompressedFabric") -> 8 bytes.
func CompressedFabricID(rootPubKey []byte, fabricID uint64) ([]byte, error) {
	if len(rootPubKey) != 65 || rootPubKey[0] != 0x04 {
		return nil, fmt.Errorf("discovery: root public key must be a 65-byte uncompressed point")
	}
	var salt [8]byte
	binary.BigEndian.PutUint64(salt[:], fabricID)
	return crypto.HKDF(rootPubKey[1:], salt[:], compressedFabricInfo, 8)
}

// OperationalInstanceName returns the DNS-SD instance name for a node, i.e.
// "<CompressedFabricID>-<NodeID>", each rendered as 16 uppercase hex digits
// (Spec 4.3.1). This is the instance published under _matter._tcp.local.
func OperationalInstanceName(compressedFabricID []byte, nodeID uint64) string {
	return fmt.Sprintf("%s-%016X", strings.ToUpper(hex.EncodeToString(compressedFabricID)), nodeID)
}
