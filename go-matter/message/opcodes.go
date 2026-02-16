package message

// Protocol IDs (Spec 4.4.3).
const (
	ProtocolSecureChannel    uint16 = 0x0000
	ProtocolInteractionModel uint16 = 0x0001
	ProtocolBDX              uint16 = 0x0002
	ProtocolUserDirected     uint16 = 0x0003
)

// Secure Channel protocol opcodes (Spec 4.9.2).
const (
	SCMsgCounterSyncReq  byte = 0x00
	SCMsgCounterSyncRsp  byte = 0x01
	SCStandaloneAck      byte = 0x10
	SCPBKDFParamRequest  byte = 0x20
	SCPBKDFParamResponse byte = 0x21
	SCPASEPake1          byte = 0x22
	SCPASEPake2          byte = 0x23
	SCPASEPake3          byte = 0x24
	SCCASESigma1         byte = 0x30
	SCCASESigma2         byte = 0x31
	SCCASESigma3         byte = 0x32
	SCCASESigma2Resume   byte = 0x33
	SCStatusReport       byte = 0x40
)

// Interaction Model protocol opcodes (Spec 8.2.3).
const (
	IMStatusResponse    byte = 0x01
	IMReadRequest       byte = 0x02
	IMSubscribeRequest  byte = 0x03
	IMSubscribeResponse byte = 0x04
	IMReportData        byte = 0x05
	IMWriteRequest      byte = 0x06
	IMWriteResponse     byte = 0x07
	IMInvokeRequest     byte = 0x08
	IMInvokeResponse    byte = 0x09
	IMTimedRequest      byte = 0x0A
)
