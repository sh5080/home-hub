package cluster

import (
	"fmt"

	"github.com/sh5080/go-matter/im"
	"github.com/sh5080/go-matter/tlv"
)

// Level Control cluster (Spec 1.6), id 0x0008. Controls a device's level
// (e.g. a dimmable light's brightness) on a 0..254 scale, where 254 is maximum.
const LevelControlID = 0x0008

const (
	lcCmdMoveToLevel = 0x00

	lcAttrCurrentLevel = 0x0000 // uint8, nullable
)

// MaxLevel is the highest valid CurrentLevel/MoveToLevel value (255 is reserved).
const MaxLevel = 254

// MoveToLevel builds a MoveToLevel command. level is 0..254; transitionTime is
// in tenths of a second (0 = as fast as the device supports).
//
// Fields per spec 1.6.6.1:
//
//	0: Level           (uint8)
//	1: TransitionTime  (uint16, nullable)
//	2: OptionsMask     (map8)
//	3: OptionsOverride (map8)
func MoveToLevel(endpoint uint16, level uint8, transitionTime uint16) (im.InvokeCommand, error) {
	if level > MaxLevel {
		return im.InvokeCommand{}, fmt.Errorf("cluster: level %d out of range [0,%d]", level, MaxLevel)
	}
	w := tlv.NewWriter()
	w.PutUint(tlv.Context(0), uint64(level))
	w.PutUint(tlv.Context(1), uint64(transitionTime))
	w.PutUint(tlv.Context(2), 0) // OptionsMask: no option bits selected
	w.PutUint(tlv.Context(3), 0) // OptionsOverride
	fields, err := w.Bytes()
	if err != nil {
		return im.InvokeCommand{}, err
	}
	return im.InvokeCommand{
		Path:   im.CommandPath{Endpoint: endpoint, Cluster: LevelControlID, Command: lcCmdMoveToLevel},
		Fields: fields,
	}, nil
}

// CurrentLevelAttribute is the path of the CurrentLevel attribute.
func CurrentLevelAttribute(endpoint uint16) im.AttributePath {
	return im.AttributePath{Endpoint: endpoint, Cluster: LevelControlID, Attribute: lcAttrCurrentLevel}
}

// DecodeLevel converts a CurrentLevel report value (0..254) to an int. A null
// value (device level undefined) is reported as -1.
func DecodeLevel(data []byte) (int, error) {
	r := tlv.NewReader(data)
	if !r.Next() {
		if err := r.Err(); err != nil {
			return 0, err
		}
		return 0, fmt.Errorf("cluster: empty CurrentLevel report")
	}
	if r.Type() == tlv.TypeNull {
		return -1, nil
	}
	v, err := r.Uint()
	if err != nil {
		return 0, err
	}
	return int(v), nil
}
