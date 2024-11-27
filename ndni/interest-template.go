package ndni

/*
#include "../csrc/ndni/interest.h"
*/
import "C"
import (
	"unsafe"

	"github.com/usnistgov/ndn-dpdk/core/cptr"
	"github.com/usnistgov/ndn-dpdk/core/nnduration"
	"github.com/usnistgov/ndn-dpdk/dpdk/pktmbuf"
	"github.com/usnistgov/ndn-dpdk/ndn"
	"github.com/usnistgov/ndn-dpdk/ndn/an"
	"github.com/usnistgov/ndn-dpdk/ndn/tlv"
	"go.uber.org/zap"
)

// InterestTemplate is a template for Interest encoding.
// A zero InterestTemplate is invalid. It must be initialized before use.
type InterestTemplate C.InterestTemplate

// InterestTemplateFromPtr converts *C.InterestTemplate to InterestTemplate.
func InterestTemplateFromPtr(ptr unsafe.Pointer) *InterestTemplate {
	return (*InterestTemplate)(ptr)
}

func (tpl *InterestTemplate) ptr() *C.InterestTemplate {
	return (*C.InterestTemplate)(tpl)
}

// Init initializes InterestTemplate.
// Arguments should be acceptable to ndn.MakeInterest.
// Name is used as name prefix.
// Panics on error.
func (tpl *InterestTemplate) Init(args ...any) {
	interest := ndn.MakeInterest(args...)
	wire, e := tlv.EncodeValueOnly(interest)
	if e != nil {
		logger.Panic("encode Interest error", zap.Error(e))
	}

	*tpl = InterestTemplate{}

	d := tlv.DecodingBuffer(wire)
	for de := range d.IterElements() {
		switch de.Type {
		case an.TtName:
			tpl.prefixL = C.uint16_t(copy(cptr.AsByteSlice(tpl.prefixV[:]), de.Value))
			tpl.midLen = C.uint16_t(copy(cptr.AsByteSlice(tpl.midBuf[:]), de.After))
		case an.TtNonce:
			tpl.nonceVOffset = tpl.midLen - C.uint16_t(len(de.After)+len(de.Value))
		}
	}
}

// Encode encodes an Interest via template.
func (tpl *InterestTemplate) Encode(m *pktmbuf.Packet, suffix ndn.Name, nonce uint32) *Packet {
	suffixP := NewPName(suffix)
	defer suffixP.Free()
	pktC := C.InterestTemplate_Encode(tpl.ptr(), (*C.struct_rte_mbuf)(m.Ptr()), suffixP.lname(), C.uint32_t(nonce))
	return PacketFromPtr(unsafe.Pointer(pktC))
}

// InterestTemplateConfig is a JSON serializable object that can construct InterestTemplate.
type InterestTemplateConfig struct {
	Prefix           ndn.Name                `json:"prefix"`
	CanBePrefix      bool                    `json:"canBePrefix,omitempty"`
	MustBeFresh      bool                    `json:"mustBeFresh,omitempty"`
	InterestLifetime nnduration.Milliseconds `json:"interestLifetime,omitempty"`
	HopLimit         ndn.HopLimit            `json:"hopLimit,omitempty"`
}

// Apply initializes InterestTemplate.
func (cfg InterestTemplateConfig) Apply(tpl *InterestTemplate) {
	a := []any{cfg.Prefix}
	if cfg.CanBePrefix {
		a = append(a, ndn.CanBePrefixFlag)
	}
	if cfg.MustBeFresh {
		a = append(a, ndn.MustBeFreshFlag)
	}
	if lifetime := cfg.InterestLifetime.Duration(); lifetime != 0 {
		a = append(a, lifetime)
	}
	if cfg.HopLimit != 0 {
		a = append(a, cfg.HopLimit)
	}
	tpl.Init(a...)
}
