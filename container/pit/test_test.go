package pit_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/usnistgov/ndn-dpdk/container/fib"
	"github.com/usnistgov/ndn-dpdk/container/fib/fibdef"
	"github.com/usnistgov/ndn-dpdk/container/fib/fibreplica"
	"github.com/usnistgov/ndn-dpdk/container/fib/fibtestenv"
	"github.com/usnistgov/ndn-dpdk/container/pcct"
	"github.com/usnistgov/ndn-dpdk/container/pit"
	"github.com/usnistgov/ndn-dpdk/core/testenv"
	"github.com/usnistgov/ndn-dpdk/dpdk/eal"
	"github.com/usnistgov/ndn-dpdk/dpdk/ealtestenv"
	"github.com/usnistgov/ndn-dpdk/dpdk/pktmbuf"
	"github.com/usnistgov/ndn-dpdk/iface"
	"github.com/usnistgov/ndn-dpdk/ndn"
	"github.com/usnistgov/ndn-dpdk/ndni"
	"github.com/usnistgov/ndn-dpdk/ndni/ndnitestenv"
	"go4.org/must"
)

func TestMain(m *testing.M) {
	ealtestenv.Init()
	pktmbuf.Direct.Update(pktmbuf.PoolConfig{Dataroom: 8000}) // needed for TestEntryLongName
	testenv.Exit(m.Run())
}

var (
	makeAR          = testenv.MakeAR
	makeInterest    = ndnitestenv.MakeInterest
	makeData        = ndnitestenv.MakeData
	makeNack        = ndnitestenv.MakeNack
	setActiveFwHint = ndnitestenv.SetActiveFwHint
	setPitToken     = ndnitestenv.SetPitToken
	setFace         = ndnitestenv.SetFace
)

type Fixture struct {
	require    *require.Assertions
	Pcct       *pcct.Pcct
	Pit        *pit.Pit
	Face       iface.ID
	Fib        *fib.Fib
	FibReplica *fibreplica.Table
	FibEntry   *fibreplica.Entry
}

func NewFixture(t testing.TB, pcctCapacity int) *Fixture {
	fixture := &Fixture{
		require: require.New(t),
	}
	var e error
	fixture.Pcct, e = pcct.New(pcct.Config{PcctCapacity: pcctCapacity}, eal.NumaSocket{})
	fixture.require.NoError(e)
	fixture.Pit = pit.FromPcct(fixture.Pcct)

	fixture.Face = iface.AllocID()
	fixture.Fib, e = fib.New(fibdef.Config{Capacity: 1023}, []fib.LookupThread{&fibtestenv.LookupThread{}})
	fixture.require.NoError(e)
	placeholderName := ndn.ParseName("/75f3c2eb-6147-4030-afbc-585b3ce876a9")
	e = fixture.Fib.Insert(fibtestenv.MakeEntry(placeholderName, nil, fixture.Face))
	fixture.require.NoError(e)
	fixture.FibReplica = fixture.Fib.Replica(eal.NumaSocket{})
	fixture.FibEntry = fixture.FibReplica.Lpm(placeholderName)

	t.Cleanup(func() {
		must.Close(fixture.Fib)
		must.Close(fixture.Pcct)
	})
	return fixture
}

// CountMpInUse returns number of in-use entries in PCCT's underlying mempool.
func (fixture *Fixture) CountMpInUse() int {
	return fixture.Pcct.AsMempool().CountInUse()
}

// Insert inserts a PIT entry and DN record.
// Returns the PIT entry.
// If CS entry is found, returns nil and frees interest.
func (fixture *Fixture) Insert(interest *ndni.Packet) *pit.Entry {
	if m := interest.Mbuf(); m.Port() == 0 {
		m.SetPort(uint16(fixture.Face))
	}
	pitEntry, csEntry := fixture.Pit.Insert(interest, fixture.FibEntry)
	if csEntry != nil {
		interest.Close()
		return nil
	}
	fixture.require.NotNil(pitEntry, "Pit.Insert failed")
	dn := pitEntry.InsertDnRecord(interest)
	fixture.require.NotNil(dn, "pitEntry.InsertDnRecord failed")
	return pitEntry
}

// FindByData finds PIT entries by Data packet.
// data is auto-released.
func (fixture *Fixture) FindByData(data *ndni.Packet, token uint64) pit.FindResult {
	defer data.Close()
	return fixture.Pit.FindByData(data, token)
}

func (fixture *Fixture) InsertFibEntry(name string, nexthop iface.ID) *fibreplica.Entry {
	n := ndn.ParseName(name)
	e := fixture.Fib.Insert(fibtestenv.MakeEntry(n, nil, nexthop))
	fixture.require.NoError(e)
	return fixture.FibReplica.Lpm(n)
}
