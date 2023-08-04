package cs_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/usnistgov/ndn-dpdk/container/cs"
	"github.com/usnistgov/ndn-dpdk/container/disk"
	"github.com/usnistgov/ndn-dpdk/container/fib"
	"github.com/usnistgov/ndn-dpdk/container/fib/fibdef"
	"github.com/usnistgov/ndn-dpdk/container/fib/fibreplica"
	"github.com/usnistgov/ndn-dpdk/container/fib/fibtestenv"
	"github.com/usnistgov/ndn-dpdk/container/pcct"
	"github.com/usnistgov/ndn-dpdk/container/pit"
	"github.com/usnistgov/ndn-dpdk/core/testenv"
	"github.com/usnistgov/ndn-dpdk/dpdk/bdev"
	"github.com/usnistgov/ndn-dpdk/dpdk/eal"
	"github.com/usnistgov/ndn-dpdk/dpdk/ealtestenv"
	"github.com/usnistgov/ndn-dpdk/dpdk/ealthread"
	"github.com/usnistgov/ndn-dpdk/dpdk/spdkenv"
	"github.com/usnistgov/ndn-dpdk/ndn"
	"github.com/usnistgov/ndn-dpdk/ndn/ndntestenv"
	"github.com/usnistgov/ndn-dpdk/ndni"
	"github.com/usnistgov/ndn-dpdk/ndni/ndnitestenv"
)

func TestMain(m *testing.M) {
	ealtestenv.Init()
	testenv.Exit(m.Run())
}

var (
	makeAR          = testenv.MakeAR
	nameEqual       = ndntestenv.NameEqual
	makeInterest    = ndnitestenv.MakeInterest
	makeData        = ndnitestenv.MakeData
	setActiveFwHint = ndnitestenv.SetActiveFwHint
	makeFibEntry    = fibtestenv.MakeEntry
)

type Fixture struct {
	t       testing.TB
	require *require.Assertions

	Pcct       *pcct.Pcct
	Cs         *cs.Cs
	Pit        *pit.Pit
	Fib        *fib.Fib
	FibReplica *fibreplica.Table
	FibEntry   *fibreplica.Entry

	Bdev      *bdev.Malloc
	SpdkTh    *spdkenv.Thread
	DiskStore *disk.Store
	DiskAlloc *disk.Alloc
}

func NewFixture(t testing.TB, cfg pcct.Config) (f *Fixture) {
	f = &Fixture{t: t, require: require.New(t)}

	cfg.PcctCapacity = 4095
	if cfg.CsMemoryCapacity == 0 {
		cfg.CsMemoryCapacity = 200
	}
	if cfg.CsIndirectCapacity == 0 {
		cfg.CsIndirectCapacity = 200
	}

	var e error
	f.Pcct, e = pcct.New(cfg, eal.NumaSocket{})
	f.require.NoError(e)
	f.Cs = cs.FromPcct(f.Pcct)
	f.Pit = pit.FromPcct(f.Pcct)

	f.Fib, e = fib.New(fibdef.Config{Capacity: 1023}, []fib.LookupThread{&fibtestenv.LookupThread{}})
	f.require.NoError(e)
	placeholderName := ndn.ParseName("/75f3c2eb-6147-4030-afbc-585b3ce876a9")
	e = f.Fib.Insert(makeFibEntry(placeholderName, nil, 9999))
	f.require.NoError(e)
	f.FibReplica = f.Fib.Replica(eal.NumaSocket{})
	f.FibEntry = f.FibReplica.Lpm(placeholderName)

	t.Cleanup(func() {
		f.require.NoError(f.Fib.Close())
		f.require.NoError(f.Pcct.Close())
		ealthread.AllocClear()
	})
	return f
}

// EnableDisk enables on-disk caching on a Malloc Bdev.
func (f *Fixture) EnableDisk(nSlots int64) {
	var e error
	f.Bdev, e = bdev.NewMalloc((1 + nSlots) * 16)
	f.require.NoError(e)
	f.t.Cleanup(func() { f.Bdev.Close() })

	f.SpdkTh, e = spdkenv.NewThread()
	f.require.NoError(e)
	f.t.Cleanup(func() { f.SpdkTh.Close() })
	f.require.NoError(ealthread.AllocLaunch(f.SpdkTh))

	f.DiskStore, e = disk.NewStore(f.Bdev, f.SpdkTh, 16, disk.StoreGetDataGo)
	f.require.NoError(e)

	min, max := f.DiskStore.SlotRange()
	f.DiskAlloc = disk.NewAlloc(min, max, eal.NumaSocket{})
	f.t.Cleanup(func() {
		f.DiskAlloc.Close()
		f.DiskStore.Close()
	})

	e = f.Cs.SetDisk(f.DiskStore, f.DiskAlloc)
	f.require.NoError(e)
}

// CountMpInUse returns number of in-use entries in PCCT's underlying mempool.
func (f *Fixture) CountMpInUse() int {
	return f.Pcct.AsMempool().CountInUse()
}

// Insert inserts a PIT entry then replaces it with a CS entry.
// Returns false if CS entry is found during PIT entry insertion.
// Returns true if CS entry is replacing PIT entry.
// This function takes ownership of interest and data.
func (f *Fixture) Insert(interest *ndni.Packet, data *ndni.Packet) (isReplacing bool) {
	pitEntry, csEntry := f.Pit.Insert(interest, f.FibEntry)
	if csEntry != nil {
		interest.Close()
		data.Close()
		return false
	}
	f.require.NotNil(pitEntry, "Pit.Insert failed")

	var pitFound pit.FindResult
	for {
		pitFound = f.Pit.FindByData(data, pitEntry.PitToken())
		if len(pitFound.ListEntries()) == 0 {
			f.require.Fail("Pit.FindByData returned empty result")
		}

		if pitFound.NeedDataDigest() {
			data.ComputeDataImplicitDigest()
		} else {
			break
		}
	}

	f.Cs.Insert(data, pitFound)
	return true
}

// InsertBulk inserts multiple CS entries following a template.
func (f *Fixture) InsertBulk(minIndex, maxIndex int, dataNameFmt, interestNameFmt string, makeInterestArgs ...any) (nInserted int) {
	for i := minIndex; i <= maxIndex; i++ {
		interest := makeInterest(fmt.Sprintf(interestNameFmt, i), makeInterestArgs...)
		data := makeData(fmt.Sprintf(dataNameFmt, i), time.Second)
		if f.Insert(interest, data) {
			nInserted++
		}
	}
	return nInserted
}

// Find finds a CS entry.
// If a PIT entry is created in Pit.Insert invocation, it is erased immediately.
// This function takes ownership of interest.
func (f *Fixture) Find(interest *ndni.Packet) *cs.Entry {
	pitEntry, csEntry := f.Pit.Insert(interest, f.FibEntry)
	if pitEntry != nil {
		f.Pit.Erase(pitEntry)
	} else {
		interest.Close()
	}
	return csEntry
}

// FindBulk finds multiple CS entries following a template.
func (f *Fixture) FindBulk(minIndex, maxIndex int, interestNameFmt string, makeInterestArgs ...any) (nFound int) {
	for i := minIndex; i <= maxIndex; i++ {
		interest := makeInterest(fmt.Sprintf(interestNameFmt, i), makeInterestArgs...)
		csEntry := f.Find(interest)
		if csEntry != nil {
			nFound++
		}
	}
	return nFound
}
