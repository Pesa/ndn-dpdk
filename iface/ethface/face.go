// Package ethface implements Ethernet faces using DPDK Ethernet devices.
package ethface

/*
#include "../../csrc/ethface/face.h"
*/
import "C"
import (
	"unsafe"

	"github.com/usnistgov/ndn-dpdk/dpdk/pktmbuf"
	"github.com/usnistgov/ndn-dpdk/iface"
	"github.com/usnistgov/ndn-dpdk/ndni"
)

type ethFace struct {
	iface.Face
	port *Port
	loc  ethLocator
	cloc cLocator
	priv *C.EthFacePriv
	flow *C.struct_rte_flow
	rxf  []*rxFlow
}

func (face *ethFace) ptr() *C.Face {
	return (*C.Face)(face.Ptr())
}

// New creates a face on the given port.
func New(port *Port, loc ethLocator) (iface.Face, error) {
	face := &ethFace{
		port: port,
		loc:  loc,
		cloc: loc.cLoc(),
	}
	return iface.New(iface.NewParams{
		Config:     loc.faceConfig().Config.WithMaxMTU(port.cfg.MTU + C.RTE_ETHER_HDR_LEN - face.cloc.sizeofHeader()),
		Socket:     port.dev.NumaSocket(),
		SizeofPriv: uintptr(C.sizeof_EthFacePriv),
		Init: func(f iface.Face) (l2TxBurstFunc unsafe.Pointer, e error) {
			for _, other := range port.faces {
				if !face.cloc.canCoexist(other.cloc) {
					return nil, LocatorConflictError{a: loc, b: other.loc}
				}
			}

			face.Face = f

			priv := (*C.EthFacePriv)(C.Face_GetPriv(face.ptr()))
			*priv = C.EthFacePriv{
				port:   C.uint16_t(port.dev.ID()),
				faceID: C.FaceID(f.ID()),
			}

			cfg, devInfo := loc.faceConfig(), port.dev.DevInfo()

			C.EthRxMatch_Prepare(&priv.rxMatch, face.cloc.ptr())
			useTxMultiSegOffload := !cfg.DisableTxMultiSegOffload && devInfo.HasTxMultiSegOffload()
			useTxChecksumOffload := !cfg.DisableTxChecksumOffload && devInfo.HasTxChecksumOffload()
			C.EthTxHdr_Prepare(&priv.txHdr, face.cloc.ptr(), C.bool(useTxChecksumOffload))
			if !useTxMultiSegOffload {
				needDataroom := pktmbuf.DefaultHeadroom + port.dev.MTU()
				haveDataroom := ndni.HeaderMempool.Config().Dataroom
				if haveDataroom >= needDataroom {
					priv.txLinearize = true
				} else {
					face.port.logger.WithFields(makeLogFields("need", needDataroom, "have", haveDataroom)).Warn(
						"TxMultiSegOffload unavailable, but cannot use txLinearize due to insufficient HEADER mempool dataroom")
				}
			}

			face.priv = priv
			return C.EthFace_TxBurst, nil
		},
		Start: func(iface.Face) (iface.Face, error) {
			return face, port.startFace(face, false)
		},
		Locator: func(iface.Face) iface.Locator {
			return face.loc
		},
		Stop: func(iface.Face) error {
			return face.port.stopFace(face)
		},
		Close: func(iface.Face) error {
			if len(face.port.faces) == 0 {
				face.port.Close()
			}
			return nil
		},
	})
}
