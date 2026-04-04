package cluster

import (
	"fmt"
	"math"

	"github.com/sh5080/go-matter/im"
	"github.com/sh5080/go-matter/tlv"
)

// Window Covering cluster (Spec 5.3), id 0x0102. Lift position is expressed in
// hundredths of a percent, where 0 = fully open and 10000 = fully closed.
const WindowCoveringID = 0x0102

const (
	wcCmdUpOrOpen           = 0x00
	wcCmdDownOrClose        = 0x01
	wcCmdStopMotion         = 0x02
	wcCmdGoToLiftPercentage = 0x05

	wcAttrCurrentPositionLiftPercent100ths = 0x000E // uint16
)

// UpOrOpen builds a Window Covering UpOrOpen command (moves toward open).
func UpOrOpen(endpoint uint16) im.InvokeCommand {
	return im.InvokeCommand{Path: im.CommandPath{Endpoint: endpoint, Cluster: WindowCoveringID, Command: wcCmdUpOrOpen}}
}

// DownOrClose builds a Window Covering DownOrClose command (moves toward closed).
func DownOrClose(endpoint uint16) im.InvokeCommand {
	return im.InvokeCommand{Path: im.CommandPath{Endpoint: endpoint, Cluster: WindowCoveringID, Command: wcCmdDownOrClose}}
}

// StopMotion builds a Window Covering StopMotion command.
func StopMotion(endpoint uint16) im.InvokeCommand {
	return im.InvokeCommand{Path: im.CommandPath{Endpoint: endpoint, Cluster: WindowCoveringID, Command: wcCmdStopMotion}}
}

// GoToLiftPercentage builds a command moving the covering to percent (0..100,
// where 0 is fully open). Fields = { 0: LiftPercent100thsValue (uint16) }.
func GoToLiftPercentage(endpoint uint16, percent float64) (im.InvokeCommand, error) {
	if percent < 0 || percent > 100 {
		return im.InvokeCommand{}, fmt.Errorf("cluster: lift percent %g out of range [0,100]", percent)
	}
	w := tlv.NewWriter()
	w.PutUint(tlv.Context(0), uint64(math.Round(percent*100)))
	fields, err := w.Bytes()
	if err != nil {
		return im.InvokeCommand{}, err
	}
	return im.InvokeCommand{
		Path:   im.CommandPath{Endpoint: endpoint, Cluster: WindowCoveringID, Command: wcCmdGoToLiftPercentage},
		Fields: fields,
	}, nil
}

// LiftPositionAttribute is the path of CurrentPositionLiftPercent100ths.
func LiftPositionAttribute(endpoint uint16) im.AttributePath {
	return im.AttributePath{Endpoint: endpoint, Cluster: WindowCoveringID, Attribute: wcAttrCurrentPositionLiftPercent100ths}
}

// DecodeLiftPercent converts a CurrentPositionLiftPercent100ths report value to
// a percentage (0..100).
func DecodeLiftPercent(data []byte) (float64, error) {
	v, err := im.DecodeUint(data)
	if err != nil {
		return 0, err
	}
	return float64(v) / 100, nil
}
