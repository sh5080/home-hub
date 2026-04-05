package casesession

import (
	"testing"

	"github.com/sh5080/go-matter/cluster"
	"github.com/sh5080/go-matter/im"
	"github.com/sh5080/go-matter/message"
)

// TestM4InvokeOverSession exercises the entire stack end to end: a CASE
// handshake, the secure sessions it yields, and an encrypted Interaction Model
// Invoke (WindowCovering GoToLiftPercentage) plus its response — proving TLV,
// message framing, crypto, session, certificates, CASE, IM, and clusters all
// interoperate, without hardware.
func TestM4InvokeOverSession(t *testing.T) {
	fabric, initID, respID := testFabric(t)
	initiator, _ := NewInitiator(fabric, initID, respNodeID, 0x1001)
	responder, _ := NewResponder(fabric, respID, 0x2002)

	// CASE handshake.
	s1, _ := initiator.Sigma1()
	s2, _ := responder.HandleSigma1(s1)
	s3, err := initiator.HandleSigma2(s2)
	if err != nil {
		t.Fatal(err)
	}
	if err := responder.HandleSigma3(s3); err != nil {
		t.Fatal(err)
	}

	initSess, err := initiator.SecureSession()
	if err != nil {
		t.Fatal(err)
	}
	respSess, err := responder.SecureSession()
	if err != nil {
		t.Fatal(err)
	}

	// Controller -> device: Invoke GoToLiftPercentage(37%) on endpoint 1.
	cmd, err := cluster.GoToLiftPercentage(1, 37)
	if err != nil {
		t.Fatal(err)
	}
	invokeBytes, _ := im.EncodeInvokeRequest([]im.InvokeCommand{cmd}, false, false)
	reqProto := message.ProtoHeader{
		Initiator: true, Reliable: true, Opcode: message.IMInvokeRequest,
		ExchangeID: 1, ProtocolID: message.ProtocolInteractionModel,
	}
	reqFrame, err := initSess.Encrypt(append(reqProto.Encode(), invokeBytes...))
	if err != nil {
		t.Fatal(err)
	}

	// Device decrypts and decodes the command.
	reqPayload, err := respSess.Decrypt(reqFrame)
	if err != nil {
		t.Fatalf("device decrypt: %v", err)
	}
	ph, imBytes, err := message.DecodeProto(reqPayload)
	if err != nil {
		t.Fatal(err)
	}
	if ph.ProtocolID != message.ProtocolInteractionModel || ph.Opcode != message.IMInvokeRequest {
		t.Fatalf("unexpected proto header: %+v", ph)
	}
	cmds, _, _, err := im.DecodeInvokeRequest(imBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 || cmds[0].Path.Cluster != cluster.WindowCoveringID || cmds[0].Path.Command != 0x05 {
		t.Fatalf("device got command %+v", cmds[0].Path)
	}
	if lift, _ := im.DecodeUint(cmds[0].Fields); lift != 3700 {
		t.Fatalf("liftPercent100ths = %d", lift)
	}

	// Device -> controller: success response.
	results := []im.InvokeResult{{Status: &im.CommandStatus{Path: cmds[0].Path, Status: im.StatusSuccess}}}
	respBytes, _ := im.EncodeInvokeResponse(results, false)
	respProto := message.ProtoHeader{
		Opcode: message.IMInvokeResponse, ExchangeID: 1, ProtocolID: message.ProtocolInteractionModel,
	}
	respFrame, err := respSess.Encrypt(append(respProto.Encode(), respBytes...))
	if err != nil {
		t.Fatal(err)
	}

	// Controller decrypts the response.
	respPayload, err := initSess.Decrypt(respFrame)
	if err != nil {
		t.Fatalf("controller decrypt: %v", err)
	}
	rph, respIM, err := message.DecodeProto(respPayload)
	if err != nil {
		t.Fatal(err)
	}
	if rph.Opcode != message.IMInvokeResponse {
		t.Fatalf("response opcode %#x", rph.Opcode)
	}
	got, err := im.DecodeInvokeResponse(respIM)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Status == nil || got[0].Status.Status != im.StatusSuccess {
		t.Fatalf("controller got response %+v", got)
	}
}
