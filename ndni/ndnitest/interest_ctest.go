package ndnitest

/*
#include "../../csrc/ndni/interest.h"
#include "../../csrc/ndni/packet.h"

typedef struct PInterestUnpacked
{
	bool canBePrefix;
	bool mustBeFresh;
	uint8_t nFwHints;
	int8_t activeFwHint;
} PInterestUnpacked;

static void
PInterest_Unpack(const PInterest* p, PInterestUnpacked* u)
{
	u->canBePrefix = p->canBePrefix;
	u->mustBeFresh = p->mustBeFresh;
	u->nFwHints = p->nFwHints;
	u->activeFwHint = p->activeFwHint;
}
*/
import "C"
import (
	"math"
	"testing"
	"unsafe"

	"github.com/usnistgov/ndn-dpdk/ndni"
)

func ctestInterestParse(t *testing.T) {
	assert, require := makeAR(t)

	// missing Nonce
	p := makePacket("0505 0703080141")
	defer p.Close()
	assert.False(bool(C.Packet_Parse(p.npkt)))

	// empty name
	p = makePacket("0508 0700 0A04A0A1A2A3")
	defer p.Close()
	assert.False(bool(C.Packet_Parse(p.npkt)))

	// minimal
	p = makePacket("050B 0703050141 0A04A0A1A2A3")
	defer p.Close()
	require.True(bool(C.Packet_Parse(p.npkt)))
	require.EqualValues(ndni.PktInterest, C.Packet_GetType(p.npkt))
	interest := C.Packet_GetInterestHdr(p.npkt)
	var u C.PInterestUnpacked
	C.PInterest_Unpack(interest, &u)
	assert.EqualValues(1, interest.name.nComps)
	assert.Equal(bytesFromHex("050141"), C.GoBytes(unsafe.Pointer(interest.name.value), C.int(interest.name.length)))
	assert.False(bool(interest.name.hasDigestComp))
	assert.EqualValues(false, u.canBePrefix)
	assert.EqualValues(false, u.mustBeFresh)
	assert.EqualValues(0, u.nFwHints)
	assert.EqualValues(-1, u.activeFwHint)
	assert.EqualValues(0xA0A1A2A3, interest.nonce)
	assert.EqualValues(ndni.DefaultInterestLifetime, interest.lifetime)
	assert.EqualValues(math.MaxUint8, interest.hopLimit)

	// full
	p = makePacket(`
		055C
		072508`, `0141 01207F6A877C0CCD0AA5A7638F9749E9293CF81C32793670B481D5A6DB788C0831CE // name
		2100 // canbeprefix
		FD03BC00 // unknown-ignored
		1200 // mustbefresh
		1E1B // fwhint
			1F091E0121070408`, `024648
			1F0E (unknown FD03BC00) 1E012207050803484632
		0A04A0A1A2A3 // nonce
		0C0276A1 // lifetime
		2201DC // hoplimit
		2401C0 // appparameters
	`)
	require.True(bool(C.Packet_ParseL3(p.npkt)))
	require.EqualValues(ndni.PktInterest, C.Packet_GetType(p.npkt))
	interest = C.Packet_GetInterestHdr(p.npkt)
	C.PInterest_Unpack(interest, &u)
	assert.EqualValues(2, interest.name.nComps)
	assert.EqualValues(37, interest.name.length)
	assert.True(bool(interest.name.hasDigestComp))
	assert.EqualValues(true, u.canBePrefix)
	assert.EqualValues(true, u.mustBeFresh)
	assert.EqualValues(2, u.nFwHints)
	assert.EqualValues(-1, u.activeFwHint)
	assert.EqualValues(0xA0A1A2A3, interest.nonce)
	assert.EqualValues(30369, interest.lifetime)
	assert.EqualValues(220, interest.hopLimit)

	// SelectFwHint
	assert.True(bool(C.PInterest_SelectFwHint(interest, 0)))
	C.PInterest_Unpack(interest, &u)
	assert.EqualValues(0, u.activeFwHint)
	assert.EqualValues(1, interest.fwHint.nComps)
	assert.Equal(bytesFromHex("08024648"), C.GoBytes(unsafe.Pointer(interest.fwHint.value), C.int(interest.fwHint.length)))

	assert.True(bool(C.PInterest_SelectFwHint(interest, 1)))
	C.PInterest_Unpack(interest, &u)
	assert.EqualValues(1, u.activeFwHint)
	assert.EqualValues(1, interest.fwHint.nComps)
	assert.Equal(bytesFromHex("0803484632"), C.GoBytes(unsafe.Pointer(interest.fwHint.value), C.int(interest.fwHint.length)))
}

func caseInterestModify(t *testing.T, linearize bool, nSegs int, input string, check func(interest *C.PInterest, u C.PInterestUnpacked)) {
	assert, require := makeAR(t)

	p := makePacket(input)
	defer p.Close()
	require.True(bool(C.Packet_Parse(p.npkt)))
	require.EqualValues(ndni.PktInterest, C.Packet_GetType(p.npkt))

	guiders := C.InterestGuiders{
		nonce:    0xAFAEADAC,
		lifetime: 8160,
		hopLimit: 15,
	}
	align := C.PacketTxAlign{
		linearize: C.bool(linearize),
	}
	modify := toPacket(unsafe.Pointer(C.Interest_ModifyGuiders(p.npkt, guiders, makeMempoolsC(), align)))
	defer modify.Close()
	assert.EqualValues(ndni.PktSInterest, C.Packet_GetType(modify.npkt))
	assert.EqualValues(nSegs, modify.mbuf.nb_segs)

	copy := makePacket(modify.Bytes())
	require.True(bool(C.Packet_ParseL3(copy.npkt)))
	require.EqualValues(ndni.PktInterest, C.Packet_GetType(copy.npkt))
	interest := C.Packet_GetInterestHdr(copy.npkt)
	var u C.PInterestUnpacked
	C.PInterest_Unpack(interest, &u)
	check(interest, u)
	assert.EqualValues(0, u.nFwHints)
	assert.EqualValues(-1, u.activeFwHint)
	assert.EqualValues(0xAFAEADAC, interest.nonce)
	assert.EqualValues(8160, interest.lifetime)
	assert.EqualValues(15, interest.hopLimit)
}

func ctestInterestModify(t *testing.T) {
	assert, _ := makeAR(t)

	const inputShort = "050B 0703080141 0A04A0A1A2A3"
	checkShort := func(interest *C.PInterest, u C.PInterestUnpacked) {
		assert.EqualValues(1, interest.name.nComps)
		assert.Equal(bytesFromHex("080141"), C.GoBytes(unsafe.Pointer(interest.name.value), C.int(interest.name.length)))
		assert.False(bool(interest.name.hasDigestComp))
		assert.EqualValues(false, u.canBePrefix)
		assert.EqualValues(false, u.mustBeFresh)
	}
	caseInterestModify(t, false, 3, inputShort, checkShort)
	caseInterestModify(t, true, 1, inputShort, checkShort)

	const inputLong = "051A 0703080142 2100 1200 0A04A0A1A2A3 2400 2C031B0101 2E02E0E1"
	checkLong := func(interest *C.PInterest, u C.PInterestUnpacked) {
		assert.EqualValues(1, interest.name.nComps)
		assert.Equal(bytesFromHex("080142"), C.GoBytes(unsafe.Pointer(interest.name.value), C.int(interest.name.length)))
		assert.False(bool(interest.name.hasDigestComp))
		assert.EqualValues(true, u.canBePrefix)
		assert.EqualValues(true, u.mustBeFresh)
	}
	caseInterestModify(t, false, 4, inputLong, checkLong)
	caseInterestModify(t, true, 1, inputLong, checkLong)
}
