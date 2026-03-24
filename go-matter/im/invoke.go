// Package im implements the Matter Interaction Model command path: encoding
// InvokeRequest and decoding InvokeResponse (Spec 8.8, 8.9). Tag numbers are
// taken from CHIP (src/app/MessageDef): CommandPathIB {0:endpoint,1:cluster,
// 2:command}, CommandDataIB {0:path,1:fields}, InvokeRequestMessage
// {0:suppressResponse,1:timedRequest,2:invokeRequests,255:imRevision},
// InvokeResponseMessage {0:suppressResponse,1:invokeResponses,255:imRevision},
// InvokeResponseIB {0:command,1:status}, CommandStatusIB {0:path,1:errorStatus},
// StatusIB {0:status,1:clusterStatus}.
package im

import (
	"fmt"

	"github.com/sh5080/go-matter/tlv"
)

// InteractionModelRevision written into request/response messages (tag 255).
const InteractionModelRevision = 11

// CommandPath identifies a cluster command on an endpoint.
type CommandPath struct {
	Endpoint uint16
	Cluster  uint32
	Command  uint32
}

// InvokeCommand is one command in an invoke request. Fields is the pre-encoded
// *content* of the command-fields structure (its inner tagged elements), or nil
// for a command that takes no fields.
type InvokeCommand struct {
	Path   CommandPath
	Fields []byte
}

func encodePath(w *tlv.Writer, tag uint8, p CommandPath) {
	w.StartList(tlv.Context(tag))
	w.PutUint(tlv.Context(0), uint64(p.Endpoint))
	w.PutUint(tlv.Context(1), uint64(p.Cluster))
	w.PutUint(tlv.Context(2), uint64(p.Command))
	w.EndContainer()
}

func encodeCommandData(w *tlv.Writer, tag tlv.Tag, c InvokeCommand) {
	w.StartStructure(tag)
	encodePath(w, 0, c.Path)
	if c.Fields != nil {
		w.StartStructure(tlv.Context(1))
		w.Raw(c.Fields)
		w.EndContainer()
	}
	w.EndContainer()
}

// EncodeInvokeRequest builds an InvokeRequestMessage.
func EncodeInvokeRequest(commands []InvokeCommand, suppressResponse, timedRequest bool) ([]byte, error) {
	w := tlv.NewWriter()
	w.StartStructure(tlv.Anonymous())
	w.PutBool(tlv.Context(0), suppressResponse)
	w.PutBool(tlv.Context(1), timedRequest)
	w.StartArray(tlv.Context(2))
	for _, c := range commands {
		encodeCommandData(w, tlv.Anonymous(), c) // array members are anonymous
	}
	w.EndContainer()
	w.PutUint(tlv.Context(255), InteractionModelRevision)
	w.EndContainer()
	return w.Bytes()
}

func decodePath(r *tlv.Reader) (CommandPath, error) {
	var p CommandPath
	if r.Type() != tlv.TypeList {
		return p, fmt.Errorf("im: command path is not a list")
	}
	if err := r.Enter(); err != nil {
		return p, err
	}
	for r.Next() {
		v, err := r.Uint()
		if err != nil {
			return p, err
		}
		switch r.Tag().Num {
		case 0:
			p.Endpoint = uint16(v)
		case 1:
			p.Cluster = uint32(v)
		case 2:
			p.Command = uint32(v)
		}
	}
	return p, nil
}

// transcodeFlatStruct re-encodes the (flat) structure the reader is positioned
// on, returning its content bytes (inner elements). Nested containers inside
// command fields are not supported by our target clusters and are rejected.
func transcodeFlatStruct(r *tlv.Reader) ([]byte, error) {
	if err := r.Enter(); err != nil {
		return nil, err
	}
	w := tlv.NewWriter()
	for r.Next() {
		tag := r.Tag()
		switch r.Type() {
		case tlv.TypeUint:
			v, _ := r.Uint()
			w.PutUint(tag, v)
		case tlv.TypeInt:
			v, _ := r.Int()
			w.PutInt(tag, v)
		case tlv.TypeBool:
			v, _ := r.Bool()
			w.PutBool(tag, v)
		case tlv.TypeBytes:
			v, _ := r.Bytes()
			w.PutBytes(tag, v)
		case tlv.TypeString:
			v, _ := r.String()
			w.PutString(tag, v)
		default:
			return nil, fmt.Errorf("im: unsupported command-field type %v", r.Type())
		}
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return w.Bytes()
}

func decodeCommandData(r *tlv.Reader) (InvokeCommand, error) {
	var c InvokeCommand
	if err := r.Enter(); err != nil {
		return c, err
	}
	for r.Next() {
		switch r.Tag().Num {
		case 0:
			p, err := decodePath(r)
			if err != nil {
				return c, err
			}
			c.Path = p
		case 1:
			f, err := transcodeFlatStruct(r)
			if err != nil {
				return c, err
			}
			c.Fields = f
		}
	}
	return c, nil
}

// DecodeInvokeRequest parses an InvokeRequestMessage.
func DecodeInvokeRequest(b []byte) (commands []InvokeCommand, suppressResponse, timedRequest bool, err error) {
	r := tlv.NewReader(b)
	if !r.Next() || r.Type() != tlv.TypeStructure {
		return nil, false, false, fmt.Errorf("im: expected top-level structure")
	}
	if err = r.Enter(); err != nil {
		return
	}
	for r.Next() {
		switch r.Tag().Num {
		case 0:
			suppressResponse, err = r.Bool()
		case 1:
			timedRequest, err = r.Bool()
		case 2:
			if err = r.Enter(); err != nil { // invokeRequests array
				return
			}
			for r.Next() {
				var c InvokeCommand
				if c, err = decodeCommandData(r); err != nil {
					return
				}
				commands = append(commands, c)
			}
		}
		if err != nil {
			return
		}
	}
	return commands, suppressResponse, timedRequest, r.Err()
}
