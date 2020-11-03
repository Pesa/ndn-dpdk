package ethface

import (
	"fmt"
	"net"

	"github.com/usnistgov/ndn-dpdk/dpdk/eal"
	"github.com/usnistgov/ndn-dpdk/dpdk/ethdev"
	"github.com/usnistgov/ndn-dpdk/iface"
	"github.com/usnistgov/ndn-dpdk/ndn/memiftransport"
)

const schemeMemif = "memif"

// MemifLocator describes a memif face.
type MemifLocator struct {
	memiftransport.Locator
}

// Scheme returns "memif".
func (loc MemifLocator) Scheme() string {
	return schemeMemif
}

func (loc MemifLocator) local() net.HardwareAddr {
	return memiftransport.AddressDPDK
}

func (loc MemifLocator) remote() net.HardwareAddr {
	return memiftransport.AddressApp
}

func (loc MemifLocator) vlan() int {
	return 0
}

// CreateFace creates a memif face.
func (loc MemifLocator) CreateFace() (iface.Face, error) {
	name := "net_memif" + eal.AllocObjectID("ethface.Memif")
	args, e := loc.ToVDevArgs()
	if e != nil {
		return nil, fmt.Errorf("memif.Locator.ToVDevArgs %w", e)
	}

	vdev, e := eal.NewVDev(name, args, eal.NumaSocket{})
	if e != nil {
		return nil, fmt.Errorf("eal.NewVDev(%s,%s) %w", name, args, e)
	}

	var pc PortConfig
	pc.MTU = loc.Dataroom
	pc.NoSetMTU = true
	port, e := NewPort(ethdev.Find(vdev.Name()), loc.local(), pc)
	if e != nil {
		vdev.Close()
		return nil, fmt.Errorf("NewPort %w", e)
	}
	port.vdev = vdev

	return New(port, loc)
}

func init() {
	iface.RegisterLocatorType(MemifLocator{}, schemeMemif)
}