package ethnetif

import (
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/safchain/ethtool"
	"github.com/usnistgov/ndn-dpdk/core/pciaddr"
	"github.com/usnistgov/ndn-dpdk/dpdk/eal"
	"github.com/usnistgov/ndn-dpdk/dpdk/ethdev"
	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
)

// etht is an ethtool instance.
// This is assigned when netIntfByName() is invoked for the first time.
var etht *ethtool.Ethtool

type netIntf struct {
	*netlink.LinkAttrs
	Link   netlink.Link
	logger *zap.Logger
}

func (n *netIntf) save(link netlink.Link) {
	n.Link = link
	n.LinkAttrs = link.Attrs()
	n.logger = logger.With(
		zap.Int("ifindex", n.Index),
		zap.String("ifname", n.Name),
	)
}

// Refresh refreshes netlink information stores in this struct.
func (n *netIntf) Refresh() {
	link, e := netlink.LinkByIndex(n.Index)
	if e != nil {
		n.logger.Warn("refresh error", zap.Error(e))
		return
	}
	n.save(link)
}

// VDevName constructs virtual device name for a particular driver.
func (n netIntf) VDevName(drv string) string {
	return fmt.Sprintf("%s_%d", drv, n.Index)
}

// EnsureLinkUp brings up the link.
// If skipBringUp is true but the interface is down, returns an error.
func (n *netIntf) EnsureLinkUp(skipBringUp bool) error {
	if n.Flags&net.FlagUp != 0 {
		return nil
	}
	if skipBringUp {
		return fmt.Errorf("interface %s is not UP", n.Name)
	}
	if e := netlink.LinkSetUp(n.Link); e != nil {
		n.logger.Error("netlink.LinkSetUp error", zap.Error(e))
		return fmt.Errorf("netlink.LinkSetUp(%s): %w", n.Name, e)
	}
	n.logger.Info("brought up the interface")
	n.Refresh()
	return nil
}

// PCIAddr determines the PCI address of a physical network interface.
func (n netIntf) PCIAddr() (a pciaddr.PCIAddress, e error) {
	busInfo, e := etht.BusInfo(n.Name)
	if e != nil {
		return pciaddr.PCIAddress{}, e
	}

	return pciaddr.Parse(filepath.Base(busInfo))
}

// NumaSocket determines the NUMA socket of a physical network interface.
func (n netIntf) NumaSocket() (socket eal.NumaSocket) {
	body, e := os.ReadFile(filepath.Join("/dev/class/net", n.Name, "device/numa_node"))
	if e != nil {
		return eal.NumaSocket{}
	}

	i, e := strconv.ParseInt(string(body), 10, 8)
	if e != nil {
		return eal.NumaSocket{}
	}
	return eal.NumaSocketFromID(int(i))
}

// FindDev locates an existing EthDev for the network interface.
func (n netIntf) FindDev() (dev ethdev.EthDev) {
	if pciAddr, e := n.PCIAddr(); e == nil {
		if dev = ethdev.FromPCI(pciAddr); dev != nil {
			return dev
		}
	}
	for _, drv := range []string{ethdev.DriverXDP, ethdev.DriverAfPacket} {
		if dev = ethdev.FromName(n.VDevName(drv)); dev != nil {
			return dev
		}
	}
	return nil
}

// SetOneChannel modifies the Ethernet device to have only one RX channel.
// This helps ensure all traffic goes into the XDP program.
func (n *netIntf) SetOneChannel() {
	channels, e := etht.GetChannels(n.Name)
	if e != nil {
		n.logger.Error("ethtool.GetChannels error", zap.Error(e))
		return
	}

	channelsUpdate := channels
	channelsUpdate.RxCount = min(channels.MaxRx, 1)
	channelsUpdate.CombinedCount = min(channels.MaxCombined, 1)

	logEntry := n.logger.With(
		zap.Uint32("old-rx", channels.RxCount),
		zap.Uint32("old-combined", channels.CombinedCount),
		zap.Uint32("new-rx", channelsUpdate.RxCount),
		zap.Uint32("new-combined", channelsUpdate.CombinedCount),
	)

	if channelsUpdate == channels {
		logEntry.Debug("no change in channels")
		return
	}

	_, e = etht.SetChannels(n.Name, channelsUpdate)
	if e != nil {
		logEntry.Error("ethtool.SetChannels error", zap.Error(e))
		return
	}

	logEntry.Debug("changed to 1 channel")
	n.Refresh()
}

// DisableVLANOffload modifies the Ethernet device to disable VLAN offload.
// This helps ensure all traffic goes into the XDP program.
func (n *netIntf) DisableVLANOffload() {
	logEntry := n.logger

	features, e := etht.Features(n.Name)
	if e != nil {
		logEntry.Error("ethtool.Features error", zap.Error(e))
		return
	}

	const rxvlanKey = "rx-vlan-hw-parse"
	rxvlan, ok := features[rxvlanKey]
	if !ok {
		logEntry.Debug("rxvlan offload not supported")
		return
	}
	if !rxvlan {
		logEntry.Debug("rxvlan offload already disabled")
		return
	}

	e = etht.Change(n.Name, map[string]bool{
		rxvlanKey: false,
	})
	if e != nil {
		logEntry.Error("ethtool.Change(rxvlan=false) error", zap.Error(e))
		return
	}

	logEntry.Debug("disabled rxvlan offload")
	n.Refresh()
}

// UnloadXDP unloads any existing XDP program on a network interface.
// This allows libxdp to load a new XDP program.
func (n *netIntf) UnloadXDP() {
	if n.Xdp == nil || !n.Xdp.Attached {
		n.logger.Debug("netlink has no attached XDP program")
		return
	}

	logEntry := n.logger.With(zap.Uint32("old-xdp-prog", n.Xdp.ProgId))
	if e := netlink.LinkSetXdpFd(n.Link, math.MaxUint32); e != nil {
		logEntry.Error("netlink.LinkSetXdpFd error", zap.Error(e))
		return
	}

	logEntry.Debug("unloaded previous XDP program")
	n.Refresh()
}

// netIntfByName creates netIntf by network interface name.
// If the network interface does not exist, returns an error.
func netIntfByName(ifname string) (n netIntf, e error) {
	if etht == nil {
		if etht, e = ethtool.NewEthtool(); e != nil {
			return netIntf{}, fmt.Errorf("ethtool.NewEthtool: %w", e)
		}
	}

	link, e := netlink.LinkByName(ifname)
	if e != nil {
		return netIntf{}, fmt.Errorf("netlink.LinkByName(%s): %w", ifname, e)
	}

	n.save(link)
	return n, nil
}
