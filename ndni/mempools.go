package ndni

/*
#include "../csrc/ndni/packet.h"
*/
import "C"
import (
	"unsafe"

	"github.com/usnistgov/ndn-dpdk/dpdk/eal"
	"github.com/usnistgov/ndn-dpdk/dpdk/pktmbuf"
)

// Predefined mempool templates.
var (
	// PacketMempool is a mempool template for receiving packets.
	// This is an alias of pktmbuf.Direct.
	PacketMempool pktmbuf.Template

	// IndirectMempool is a mempool template for referencing buffers.
	// This is an alias of pktmbuf.Indirect.
	IndirectMempool pktmbuf.Template

	// HeaderMempool is a mempool template for packet headers.
	// This includes T-L portion of an L3 packet, NDNLP header, and Ethernet header.
	// It is also used for Interest guiders.
	HeaderMempool pktmbuf.Template

	// InterestMempool is a mempool template for encoding Interests.
	InterestMempool pktmbuf.Template

	// DataMempool is a mempool template for encoding Data headers.
	DataMempool pktmbuf.Template

	// PayloadMempool is a mempool template for encoding Data payload.
	PayloadMempool pktmbuf.Template
)

func init() {
	PacketMempool = pktmbuf.Direct
	PacketMempool.Update(pktmbuf.PoolConfig{
		PrivSize: int(C.sizeof_PacketPriv),
	})

	IndirectMempool = pktmbuf.Indirect

	headerDataroom := pktmbuf.DefaultHeadroom + LpHeaderHeadroom
	HeaderMempool = pktmbuf.RegisterTemplate("HEADER", pktmbuf.PoolConfig{
		Capacity: 65535,
		PrivSize: int(C.sizeof_PacketPriv),
		Dataroom: headerDataroom,
	})

	InterestMempool = pktmbuf.RegisterTemplate("INTEREST", pktmbuf.PoolConfig{
		Capacity: 65535,
		PrivSize: int(C.sizeof_PacketPriv),
		Dataroom: headerDataroom + InterestTemplateDataroom,
	})

	DataMempool = pktmbuf.RegisterTemplate("DATA", pktmbuf.PoolConfig{
		Capacity: 65535,
		PrivSize: int(C.sizeof_PacketPriv),
		Dataroom: headerDataroom + DataGenDataroom,
	})

	PayloadMempool = pktmbuf.RegisterTemplate("PAYLOAD", pktmbuf.PoolConfig{
		Capacity: 1023,
		PrivSize: int(C.sizeof_PacketPriv),
		Dataroom: headerDataroom + DataGenBufLen + 9000,
	})
}

// MakePacketMempools creates mempools and assigns them to *C.PacketMempools.
func MakePacketMempools(ptr unsafe.Pointer, socket eal.NumaSocket) {
	c := (*C.PacketMempools)(ptr)
	c.packet = (*C.struct_rte_mempool)(PacketMempool.MakePool(socket).Ptr())
	c.indirect = (*C.struct_rte_mempool)(IndirectMempool.MakePool(socket).Ptr())
	c.header = (*C.struct_rte_mempool)(HeaderMempool.MakePool(socket).Ptr())
}
