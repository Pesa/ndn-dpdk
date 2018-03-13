package fwdp

/*
#include "input.h"
*/
import "C"
import (
	"unsafe"

	"ndn-dpdk/container/pcct"
	"ndn-dpdk/core/running_stat"
	"ndn-dpdk/dpdk"
	"ndn-dpdk/iface"
)

// Count number of input and forwarding processes.
func (dp *DataPlane) CountLCores() (nInputs int, nFwds int) {
	return len(dp.inputLCores), len(dp.fwdLCores)
}

// Information and counters about an input process.
type InputInfo struct {
	LCore dpdk.LCore     // LCore executing this input process
	Faces []iface.FaceId // faces served by this input process

	NNameDisp  uint64 // packets dispatched by name
	NTokenDisp uint64 // packets dispatched by token
	NBadToken  uint64 // dropped packets due to missing or bad token
}

// Read information about i-th input.
func (dp *DataPlane) ReadInputInfo(i int) (info *InputInfo) {
	if i < 0 || i >= len(dp.inputLCores) {
		return nil
	}
	input := dp.inputs[i]

	info = new(InputInfo)
	info.LCore = dp.inputLCores[i]
	info.Faces = dp.inputRxLoopers[i].ListFacesInRxLoop()

	info.NNameDisp = uint64(input.nNameDisp)
	info.NTokenDisp = uint64(input.nTokenDisp)
	info.NBadToken = uint64(input.nBadToken)

	return info
}

// Information and counters about a fwd process.
type FwdInfo struct {
	LCore dpdk.LCore // LCore executing this fwd process

	QueueCapacity int                   // input queue capacity
	NQueueDrops   uint64                // packets dropped because input queue is full
	TimeSinceRx   running_stat.Snapshot // input latency in nanos

	HeaderMpUsage   int // how many entries are used in header mempool
	IndirectMpUsage int // how many entries are used in indirect mempool
}

// Read information about i-th fwd.
func (dp *DataPlane) ReadFwdInfo(i int) (info *FwdInfo) {
	if i < 0 || i >= len(dp.fwdLCores) {
		return nil
	}

	info = new(FwdInfo)
	fwd := dp.fwds[i]
	info.LCore = dp.fwdLCores[i]

	fwdQ := dpdk.RingFromPtr(unsafe.Pointer(fwd.queue))
	info.QueueCapacity = fwdQ.GetCapacity()

	timeSinceRxStat := running_stat.FromPtr(unsafe.Pointer(&fwd.timeSinceRxStat))
	info.TimeSinceRx = running_stat.TakeSnapshot(timeSinceRxStat).Multiply(dpdk.GetNanosInTscUnit())

	info.HeaderMpUsage = dpdk.MempoolFromPtr(unsafe.Pointer(fwd.headerMp)).CountInUse()
	info.IndirectMpUsage = dpdk.MempoolFromPtr(unsafe.Pointer(fwd.indirectMp)).CountInUse()

	for _, input := range dp.inputs {
		inputConn := C.FwInput_GetConn(input, C.uint8_t(i))
		info.NQueueDrops += uint64(inputConn.nDrops)
	}

	return info
}

// Access i-th fwd's PCCT.
func (dp *DataPlane) GetFwdPcct(i int) *pcct.Pcct {
	if i < 0 || i >= len(dp.fwds) {
		return nil
	}
	pcct := pcct.PcctFromPtr(unsafe.Pointer(dp.fwds[i].pit))
	return &pcct
}
