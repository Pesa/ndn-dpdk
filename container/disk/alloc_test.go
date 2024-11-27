package disk_test

import (
	"testing"

	"github.com/usnistgov/ndn-dpdk/container/disk"
	"github.com/usnistgov/ndn-dpdk/dpdk/bdev"
	"github.com/usnistgov/ndn-dpdk/dpdk/eal"
	"github.com/zyedidia/generic/mapset"
)

func TestAlloc(t *testing.T) {
	assert, _ := makeAR(t)

	min, max := uint64(512), uint64(1011)
	a := disk.NewAlloc(min, max, eal.NumaSocket{})
	defer a.Close()
	aMin, aMax := a.SlotRange()
	assert.EqualValues(min, aMin)
	assert.EqualValues(max, aMax)

	slots := mapset.New[uint64]()
	expectAlloc := func(msgAndArgs ...any) uint64 {
		slot, e := a.Alloc()
		if assert.NoError(e, msgAndArgs...) {
			assert.LessOrEqual(min, slot, msgAndArgs...)
			assert.GreaterOrEqual(max, slot, msgAndArgs...)
			assert.False(slots.Has(slot), msgAndArgs...)
			slots.Put(slot)
		}
		return slot
	}

	for i := range 500 {
		expectAlloc(i)
	}
	assert.Equal(500, slots.Size())

	_, e := a.Alloc()
	assert.Error(e)

	a.Free(515)
	slots.Remove(515)
	assert.EqualValues(515, expectAlloc(515))

	a.Free(516)
	slots.Remove(516)
	a.Free(517)
	slots.Remove(517)
	expectAlloc()
	expectAlloc()

	_, e = a.Alloc()
	assert.Error(e)
}

func TestSizeCalc(t *testing.T) {
	assert, _ := makeAR(t)

	calc := disk.SizeCalc{
		NThreads:   4,
		NPackets:   1000,
		PacketSize: 5000,
	}

	assert.Equal(10, calc.BlocksPerSlot())
	assert.Equal(int64(40010), calc.MinBlocks())

	f := NewStoreFixture(t)
	f.AddDevice(bdev.NewMalloc(calc.MinBlocks()))
	f.MakeStore(calc.BlocksPerSlot())

	a0 := disk.NewAllocIn(f.Store, 0, calc.NThreads, eal.NumaSocket{})
	defer a0.Close()
	min0, max0 := a0.SlotRange()
	assert.Equal(uint64(1), min0)
	assert.Equal(uint64(1000), max0)

	a3 := disk.NewAllocIn(f.Store, 3, calc.NThreads, eal.NumaSocket{})
	defer a3.Close()
	min3, max3 := a3.SlotRange()
	assert.Equal(uint64(3001), min3)
	assert.Equal(uint64(4000), max3)
}
