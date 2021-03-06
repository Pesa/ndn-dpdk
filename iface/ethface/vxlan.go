package ethface

import (
	"errors"

	"github.com/usnistgov/ndn-dpdk/core/macaddr"
	"github.com/usnistgov/ndn-dpdk/iface"
	"github.com/usnistgov/ndn-dpdk/ndn/packettransport"
)

const (
	// MinVXLAN is the minimum VXLAN Network Identifier.
	MinVXLAN = 0x000000

	// MaxVXLAN is the maximum VXLAN Network Identifier.
	MaxVXLAN = 0xFFFFFF
)

// Error conditions.
var (
	ErrVXLAN = errors.New("invalid VXLAN Network Identifier")
)

const schemeVxlan = "vxlan"

// VxlanLocator describes an Ethernet VXLAN face.
type VxlanLocator struct {
	// UDPLocator contains MAC addresses, EthDev specification, and UDP endpoints.
	// loc.LocalUDP and loc.RemoteUDP are interpreted as destination port numbers.
	UDPLocator

	// VXLAN is the VXLAN virtual network identifier.
	// This must be between MinVXLAN and MaxVXLAN.
	VXLAN int `json:"vxlan"`

	// InnerLocal is the inner local MAC address.
	// This must be a 48-bit unicast address.
	InnerLocal macaddr.Flag `json:"innerLocal"`

	// InnerRemote is the inner remote MAC address.
	// This must be a 48-bit unicast address.
	InnerRemote macaddr.Flag `json:"innerRemote"`
}

// Scheme returns "vxlan".
func (VxlanLocator) Scheme() string {
	return schemeVxlan
}

// Validate checks Locator fields.
func (loc VxlanLocator) Validate() error {
	if e := loc.UDPLocator.Validate(); e != nil {
		return e
	}

	if loc.VXLAN < MinVXLAN || loc.VXLAN > MaxVXLAN {
		return ErrVXLAN
	}

	if !macaddr.IsUnicast(loc.InnerLocal.HardwareAddr) || !macaddr.IsUnicast(loc.InnerRemote.HardwareAddr) {
		return packettransport.ErrUnicastMacAddr
	}

	return nil
}

func (loc VxlanLocator) cLoc() (c cLocator) {
	c = loc.UDPLocator.cLoc()
	c.Vxlan = uint32(loc.VXLAN)
	copy(c.InnerLocal.Bytes[:], ([]byte)(loc.InnerLocal.HardwareAddr))
	copy(c.InnerRemote.Bytes[:], ([]byte)(loc.InnerRemote.HardwareAddr))
	return
}

// CreateFace creates a VXLAN face.
func (loc VxlanLocator) CreateFace() (face iface.Face, e error) {
	port, e := loc.makePort()
	if e != nil {
		return nil, e
	}
	return New(port, loc)
}

func init() {
	iface.RegisterLocatorType(VxlanLocator{}, schemeVxlan)
}
