package cluster

import (
	"fmt"

	"github.com/sh5080/go-matter/im"
	"github.com/sh5080/go-matter/tlv"
)

// Commissioning clusters (Spec 11.10 General Commissioning, 11.18 Node
// Operational Credentials). Both live on the root endpoint (0).
const (
	GeneralCommissioningID   = 0x0030
	OperationalCredentialsID = 0x003E
	commissioningEndpoint    = 0
)

// General Commissioning commands.
const (
	gcCmdArmFailSafe                   = 0x00
	gcCmdArmFailSafeResponse           = 0x01
	gcCmdCommissioningComplete         = 0x04
	gcCmdCommissioningCompleteResponse = 0x05
)

// Operational Credentials commands.
const (
	ocCmdCSRRequest         = 0x04
	ocCmdCSRResponse        = 0x05
	ocCmdAddNOC             = 0x06
	ocCmdNOCResponse        = 0x08
	ocCmdAddTrustedRootCert = 0x0B
)

// ArmFailSafe builds a GeneralCommissioning ArmFailSafe command: the device
// arms a timer that reverts commissioning changes unless CommissioningComplete
// arrives in time. Fields: { 0: expiryLengthSeconds, 1: breadcrumb }.
func ArmFailSafe(expirySeconds uint16) (im.InvokeCommand, error) {
	w := tlv.NewWriter()
	w.PutUint(tlv.Context(0), uint64(expirySeconds))
	w.PutUint(tlv.Context(1), 0) // breadcrumb
	fields, err := w.Bytes()
	if err != nil {
		return im.InvokeCommand{}, err
	}
	return im.InvokeCommand{
		Path:   im.CommandPath{Endpoint: commissioningEndpoint, Cluster: GeneralCommissioningID, Command: gcCmdArmFailSafe},
		Fields: fields,
	}, nil
}

// CommissioningComplete builds the flow-final command. It must be sent over a
// CASE session on the new fabric (Spec 5.5): success disarms the fail-safe.
func CommissioningComplete() im.InvokeCommand {
	return im.InvokeCommand{
		Path: im.CommandPath{Endpoint: commissioningEndpoint, Cluster: GeneralCommissioningID, Command: gcCmdCommissioningComplete},
	}
}

// DecodeCommissioningError extracts { 0: errorCode, 1: debugText } from a
// General Commissioning *Response command. 0 = OK.
func DecodeCommissioningError(fields []byte) (uint8, error) {
	r := tlv.NewReader(fields)
	for r.Next() {
		if r.Tag().Num == 0 {
			v, err := r.Uint()
			if err != nil {
				return 0, err
			}
			return uint8(v), nil
		}
	}
	if err := r.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("cluster: response carries no errorCode")
}

// CSRRequest builds an OperationalCredentials CSRRequest: the device generates
// its operational keypair and returns a CSR. Fields: { 0: CSRNonce(32) }.
func CSRRequest(nonce []byte) (im.InvokeCommand, error) {
	if len(nonce) != 32 {
		return im.InvokeCommand{}, fmt.Errorf("cluster: CSR nonce must be 32 bytes, got %d", len(nonce))
	}
	w := tlv.NewWriter()
	w.PutBytes(tlv.Context(0), nonce)
	fields, err := w.Bytes()
	if err != nil {
		return im.InvokeCommand{}, err
	}
	return im.InvokeCommand{
		Path:   im.CommandPath{Endpoint: commissioningEndpoint, Cluster: OperationalCredentialsID, Command: ocCmdCSRRequest},
		Fields: fields,
	}, nil
}

// CSRResponseFields is the decoded CSRResponse.
type CSRResponseFields struct {
	// NOCSRElements is a TLV structure { 1: csr(DER), 2: CSRNonce, ... } signed
	// by the device attestation key.
	NOCSRElements        []byte
	AttestationSignature []byte
}

// DecodeCSRResponse parses a CSRResponse command's fields.
func DecodeCSRResponse(fields []byte) (CSRResponseFields, error) {
	var out CSRResponseFields
	r := tlv.NewReader(fields)
	for r.Next() {
		var err error
		switch r.Tag().Num {
		case 0:
			out.NOCSRElements, err = r.Bytes()
		case 1:
			out.AttestationSignature, err = r.Bytes()
		}
		if err != nil {
			return out, err
		}
	}
	if err := r.Err(); err != nil {
		return out, err
	}
	if out.NOCSRElements == nil {
		return out, fmt.Errorf("cluster: CSRResponse missing NOCSRElements")
	}
	return out, nil
}

// ParseNOCSRElements extracts the DER CSR and the echoed nonce from
// NOCSRElements: TLV structure { 1: csr, 2: CSRNonce }.
func ParseNOCSRElements(elements []byte) (csrDER, nonce []byte, err error) {
	r := tlv.NewReader(elements)
	if !r.Next() || r.Type() != tlv.TypeStructure {
		return nil, nil, fmt.Errorf("cluster: NOCSRElements: expected structure")
	}
	if err := r.Enter(); err != nil {
		return nil, nil, err
	}
	for r.Next() {
		var e error
		switch r.Tag().Num {
		case 1:
			csrDER, e = r.Bytes()
		case 2:
			nonce, e = r.Bytes()
		}
		if e != nil {
			return nil, nil, e
		}
	}
	if err := r.Err(); err != nil {
		return nil, nil, err
	}
	if csrDER == nil {
		return nil, nil, fmt.Errorf("cluster: NOCSRElements missing CSR")
	}
	return csrDER, nonce, nil
}

// EncodeNOCSRElements builds NOCSRElements for a device-side responder.
func EncodeNOCSRElements(csrDER, nonce []byte) ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	w.PutBytes(tlv.Context(1), csrDER)
	w.PutBytes(tlv.Context(2), nonce)
	w.EndContainer()
	return w.Bytes()
}

// AddTrustedRootCert builds AddTrustedRootCertificate carrying the fabric's
// RCAC in Matter TLV form. Fields: { 0: rootCACertificate }.
func AddTrustedRootCert(rcacTLV []byte) (im.InvokeCommand, error) {
	w := tlv.NewWriter()
	w.PutBytes(tlv.Context(0), rcacTLV)
	fields, err := w.Bytes()
	if err != nil {
		return im.InvokeCommand{}, err
	}
	return im.InvokeCommand{
		Path:   im.CommandPath{Endpoint: commissioningEndpoint, Cluster: OperationalCredentialsID, Command: ocCmdAddTrustedRootCert},
		Fields: fields,
	}, nil
}

// AddNOC installs the device's operational certificate and fabric secrets.
// Fields: { 0: NOCValue, 2: IPKValue, 3: caseAdminSubject, 4: adminVendorId }
// (1: ICACValue omitted — this controller issues directly from the root).
func AddNOC(nocTLV, ipk []byte, caseAdminSubject uint64, adminVendorID uint16) (im.InvokeCommand, error) {
	w := tlv.NewWriter()
	w.PutBytes(tlv.Context(0), nocTLV)
	w.PutBytes(tlv.Context(2), ipk)
	w.PutUint(tlv.Context(3), caseAdminSubject)
	w.PutUint(tlv.Context(4), uint64(adminVendorID))
	fields, err := w.Bytes()
	if err != nil {
		return im.InvokeCommand{}, err
	}
	return im.InvokeCommand{
		Path:   im.CommandPath{Endpoint: commissioningEndpoint, Cluster: OperationalCredentialsID, Command: ocCmdAddNOC},
		Fields: fields,
	}, nil
}

// NOCResponseFields is the decoded NOCResponse { 0: statusCode, 1: fabricIndex }.
type NOCResponseFields struct {
	Status      uint8 // 0 = OK (Spec 11.18.4.10 NodeOperationalCertStatusEnum)
	FabricIndex uint8
}

// DecodeNOCResponse parses a NOCResponse command's fields.
func DecodeNOCResponse(fields []byte) (NOCResponseFields, error) {
	var out NOCResponseFields
	r := tlv.NewReader(fields)
	for r.Next() {
		v, err := r.Uint()
		if err != nil {
			return out, err
		}
		switch r.Tag().Num {
		case 0:
			out.Status = uint8(v)
		case 1:
			out.FabricIndex = uint8(v)
		}
	}
	return out, r.Err()
}

// Response command ids, exported for responder/test implementations.
const (
	CmdArmFailSafeResponse           = gcCmdArmFailSafeResponse
	CmdCommissioningCompleteResponse = gcCmdCommissioningCompleteResponse
	CmdCSRResponse                   = ocCmdCSRResponse
	CmdNOCResponse                   = ocCmdNOCResponse
)
