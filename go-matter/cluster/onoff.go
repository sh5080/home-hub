// Package cluster provides typed builders for the Matter application clusters
// the hub uses, layered over the Interaction Model. Command and attribute ids
// are from the Matter Application Cluster Specification.
package cluster

import (
	"fmt"

	"github.com/sh5080/go-matter/im"
	"github.com/sh5080/go-matter/tlv"
)

// On/Off cluster (Spec 1.5), id 0x0006.
const OnOffID = 0x0006

const (
	onOffCmdOff    = 0x00
	onOffCmdOn     = 0x01
	onOffCmdToggle = 0x02

	onOffAttrOnOff = 0x0000 // bool
)

// OnOffOff builds an On/Off Off command.
func OnOffOff(endpoint uint16) im.InvokeCommand {
	return im.InvokeCommand{Path: im.CommandPath{Endpoint: endpoint, Cluster: OnOffID, Command: onOffCmdOff}}
}

// OnOffOn builds an On/Off On command.
func OnOffOn(endpoint uint16) im.InvokeCommand {
	return im.InvokeCommand{Path: im.CommandPath{Endpoint: endpoint, Cluster: OnOffID, Command: onOffCmdOn}}
}

// OnOffToggle builds an On/Off Toggle command.
func OnOffToggle(endpoint uint16) im.InvokeCommand {
	return im.InvokeCommand{Path: im.CommandPath{Endpoint: endpoint, Cluster: OnOffID, Command: onOffCmdToggle}}
}

// OnOffAttribute is the path of the OnOff (bool) attribute.
func OnOffAttribute(endpoint uint16) im.AttributePath {
	return im.AttributePath{Endpoint: endpoint, Cluster: OnOffID, Attribute: onOffAttrOnOff}
}

// DecodeOnOff extracts the boolean OnOff value from a report's attribute data.
// The OnOff attribute is a TLV bool on the wire.
func DecodeOnOff(data []byte) (bool, error) {
	r := tlv.NewReader(data)
	if !r.Next() {
		return false, fmt.Errorf("cluster: empty OnOff attribute data")
	}
	return r.Bool()
}
