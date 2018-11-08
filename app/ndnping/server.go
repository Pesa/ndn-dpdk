package ndnping

/*
#include "server.h"
*/
import "C"
import (
	"fmt"
	"time"
	"unsafe"

	"ndn-dpdk/appinit"
	"ndn-dpdk/container/nameset"
	"ndn-dpdk/dpdk"
	"ndn-dpdk/iface"
	"ndn-dpdk/ndn"
)

// Server internal config.
const (
	Server_BurstSize       = 64
	Server_FreshnessPeriod = 60000
)

type Server struct {
	c *C.NdnpingServer
}

func newServer(face iface.IFace, cfg ServerConfig) *Server {
	var server Server

	socket := face.GetNumaSocket()
	server.c = (*C.NdnpingServer)(dpdk.Zmalloc("NdnpingServer", C.sizeof_NdnpingServer, socket))
	server.c.face = (C.FaceId)(face.GetFaceId())
	server.c.freshnessPeriod = C.uint32_t(Server_FreshnessPeriod)

	server.c.dataMp = (*C.struct_rte_mempool)(appinit.MakePktmbufPool(
		appinit.MP_DATA, socket).GetPtr())
	server.c.dataMbufHeadroom = C.uint16_t(appinit.SizeofEthLpHeaders() + ndn.EncodeData_GetHeadroom())

	for _, patternCfg := range cfg.Patterns {
		server.addPattern(patternCfg)
	}

	server.c.wantNackNoRoute = C.bool(cfg.Nack)

	return &server
}

func (server Server) Close() error {
	server.getPatterns().Close()
	dpdk.Free(server.c)
	return nil
}

func (server Server) SetFreshnessPeriod(freshness time.Duration) {
	server.c.freshnessPeriod = C.uint32_t(freshness / time.Millisecond)
}

func (server Server) getPatterns() nameset.NameSet {
	return nameset.FromPtr(unsafe.Pointer(&server.c.patterns))
}

func (server Server) addPattern(cfg ServerPattern) {
	suffixL := 0
	if cfg.Suffix != nil {
		suffixL = cfg.Suffix.Size()
	}
	sizeofUsr := int(C.sizeof_NdnpingServerPattern) + suffixL

	_, usr := server.getPatterns().InsertWithZeroUsr(cfg.Prefix, sizeofUsr)
	patternC := (*C.NdnpingServerPattern)(usr)
	patternC.payloadL = C.uint16_t(cfg.PayloadLen)
	if suffixL > 0 {
		suffixV := unsafe.Pointer(uintptr(usr) + uintptr(C.sizeof_NdnpingServerPattern))
		oldSuffixV := cfg.Suffix.GetValue()
		C.memcpy(suffixV, unsafe.Pointer(&oldSuffixV[0]), C.size_t(suffixL))
		patternC.nameSuffix.value = (*C.uint8_t)(suffixV)
		patternC.nameSuffix.length = (C.uint16_t)(suffixL)
	}
}

func (server Server) Run() int {
	C.NdnpingServer_Run(server.c)
	return 0
}

type ServerPatternCounters struct {
	NInterests uint64
}

func (cnt ServerPatternCounters) String() string {
	return fmt.Sprintf("%dI", cnt.NInterests)
}

type ServerCounters struct {
	PerPattern  []ServerPatternCounters
	NInterests  uint64
	NNoMatch    uint64
	NAllocError uint64
}

func (cnt ServerCounters) String() string {
	s := fmt.Sprintf("%dI %dno-match %dalloc-error", cnt.NInterests, cnt.NNoMatch, cnt.NAllocError)
	for i, pcnt := range cnt.PerPattern {
		s += fmt.Sprintf(", pattern(%d) %s", i, pcnt)
	}
	return s
}

func (server Server) ReadCounters() (cnt ServerCounters) {
	patterns := server.getPatterns()
	cnt.PerPattern = make([]ServerPatternCounters, patterns.Len())
	for i := 0; i < len(cnt.PerPattern); i++ {
		pattern := (*C.NdnpingServerPattern)(patterns.GetUsr(i))
		cnt.PerPattern[i].NInterests = uint64(pattern.nInterests)
		cnt.NInterests += uint64(pattern.nInterests)
	}

	cnt.NNoMatch = uint64(server.c.nNoMatch)
	cnt.NAllocError = uint64(server.c.nAllocError)
	return cnt
}
