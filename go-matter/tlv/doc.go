// Package tlv implements the Matter TLV encoding (Matter Core Specification,
// Appendix A). Every Matter payload — Interaction Model messages, operational
// certificates, PASE/CASE parameters — is encoded in this format.
//
// The API is streaming, mirroring the reference implementation's TLVWriter /
// TLVReader: a Writer appends elements (containers are opened and closed
// explicitly) and a Reader iterates elements, entering containers on demand.
// All multi-byte integers and length fields are little-endian.
package tlv
