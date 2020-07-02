package fwdp

/*
#include "../../csrc/fwdp/crypto.h"
*/
import "C"
import (
	"fmt"
	"unsafe"

	"github.com/usnistgov/ndn-dpdk/app/inputdemux"
	"github.com/usnistgov/ndn-dpdk/container/ndt"
	"github.com/usnistgov/ndn-dpdk/dpdk/cryptodev"
	"github.com/usnistgov/ndn-dpdk/dpdk/eal"
	"github.com/usnistgov/ndn-dpdk/dpdk/ealthread"
	"github.com/usnistgov/ndn-dpdk/dpdk/mempool"
	"github.com/usnistgov/ndn-dpdk/dpdk/ringbuffer"
)

type CryptoConfig struct {
	InputCapacity  int
	OpPoolCapacity int
}

type Crypto struct {
	ealthread.Thread
	id     int
	c      *C.FwCrypto
	demuxD *inputdemux.Demux
	devS   *cryptodev.CryptoDev
	devM   *cryptodev.CryptoDev
}

func newCrypto(id int, lc eal.LCore, cfg CryptoConfig, ndt *ndt.Ndt, fwds []*Fwd) (*Crypto, error) {
	socket := lc.NumaSocket()
	fwc := &Crypto{
		id: id,
		c:  (*C.FwCrypto)(eal.ZmallocAligned("FwCrypto", C.sizeof_FwCrypto, 1, socket)),
	}
	fwc.Thread = ealthread.New(
		func() int { C.FwCrypto_Run(fwc.c); return 0 },
		ealthread.InitStopFlag(unsafe.Pointer(&fwc.c.stop)),
	)
	fwc.SetLCore(lc)

	input, e := ringbuffer.New(fwc.String()+"_input", cfg.InputCapacity, socket,
		ringbuffer.ProducerMulti, ringbuffer.ConsumerSingle)
	if e != nil {
		return nil, fmt.Errorf("ringbuffer.New: %v", e)
	}
	fwc.c.input = (*C.struct_rte_ring)(input.Ptr())

	opPool, e := cryptodev.NewOpPool(fwc.String()+"_pool", cryptodev.OpPoolConfig{Capacity: cfg.OpPoolCapacity}, socket)
	if e != nil {
		return nil, fmt.Errorf("cryptodev.NewOpPool: %v", e)
	}
	fwc.c.opPool = (*C.struct_rte_mempool)(opPool.Ptr())

	fwc.devS, e = cryptodev.SingleSegDrv.Create(fmt.Sprintf("fwc%ds", fwc.id), cryptodev.Config{}, socket)
	if e != nil {
		return nil, fmt.Errorf("cryptodev.SingleSegDrv.Create: %v", e)
	}
	fwc.devS.QueuePair(0).CopyToC(unsafe.Pointer(&fwc.c.singleSeg))

	fwc.devM, e = cryptodev.MultiSegDrv.Create(fmt.Sprintf("fwc%dm", fwc.id), cryptodev.Config{}, socket)
	if e != nil {
		return nil, fmt.Errorf("cryptodev.MultiSegDrv.Create: %v", e)
	}
	fwc.devM.QueuePair(0).CopyToC(unsafe.Pointer(&fwc.c.multiSeg))

	fwc.demuxD = inputdemux.DemuxFromPtr(unsafe.Pointer(&fwc.c.output))
	fwc.demuxD.InitNdt(ndt, id)
	for i, fwd := range fwds {
		fwc.demuxD.SetDest(i, fwd.dataQueue)
		fwd.c.crypto = fwc.c.input
	}

	return fwc, nil
}

func (fwc *Crypto) String() string {
	return fmt.Sprintf("crypto%d", fwc.id)
}

func (fwc *Crypto) Close() error {
	fwc.Stop()

	fwc.devM.Close()
	fwc.devS.Close()
	mempool.FromPtr(unsafe.Pointer(fwc.c.opPool)).Close()
	ringbuffer.FromPtr(unsafe.Pointer(fwc.c.input)).Close()
	eal.Free(unsafe.Pointer(fwc.c))
	return nil
}
