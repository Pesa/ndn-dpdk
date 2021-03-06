// Package fibreplica controls a FIB replica in C.Fib struct.
package fibreplica

/*
#include "../../../csrc/fib/fib.h"
*/
import "C"
import (
	"errors"
	"math/rand"
	"unsafe"

	"github.com/usnistgov/ndn-dpdk/container/fib/fibdef"
	"github.com/usnistgov/ndn-dpdk/dpdk/eal"
	"github.com/usnistgov/ndn-dpdk/dpdk/mempool"
	"github.com/usnistgov/ndn-dpdk/ndn"
	"github.com/usnistgov/ndn-dpdk/ndni"
)

// Table represents a FIB replica.
type Table struct {
	mp    *mempool.Mempool
	c     *C.Fib
	nDyns int
}

// Ptr returns *C.Fib pointer.
func (t *Table) Ptr() unsafe.Pointer {
	return unsafe.Pointer(t.c)
}

// Close frees C memory.
func (t *Table) Close() error {
	C.Fib_Clear(t.c)
	C.cds_lfht_destroy(t.c.lfht, nil)
	return t.mp.Close()
}

// Get retrieves an entry.
func (t *Table) Get(name ndn.Name) *Entry {
	pname := ndni.NewPName(name)
	defer pname.Free()
	lname := *(*C.LName)(pname.Ptr())
	return entryFromPtr(C.Fib_Get(t.c, lname, C.uint64_t(pname.ComputeHash())))
}

// Lpm performs longest prefix match.
func (t *Table) Lpm(name ndn.Name) *Entry {
	pname := ndni.NewPName(name)
	defer pname.Free()
	return entryFromPtr(C.Fib_Lpm(t.c, (*C.PName)(pname.Ptr())))
}

func (t *Table) allocBulk(entries []*Entry) error {
	if len(entries) == 0 {
		return nil
	}
	ok := C.Fib_AllocBulk(t.c, (**C.FibEntry)(unsafe.Pointer(&entries[0])), C.uint(len(entries)))
	if !bool(ok) {
		return errors.New("allocation failure")
	}
	return nil
}

func (t *Table) write(entry *Entry) {
	C.Fib_Write(t.c, entry.ptr())
}

func (t *Table) erase(entry *Entry) {
	C.Fib_Erase(t.c, entry.ptr())
}

func (t *Table) deferredFree(entry *Entry) {
	C.Fib_DeferredFree(t.c, entry.ptr())
}

// New creates a Table.
func New(cfg fibdef.Config, nDyns int, socket eal.NumaSocket) (*Table, error) {
	cfg.ApplyDefaults()
	mp, e := mempool.New(mempool.Config{
		Capacity:       cfg.Capacity,
		ElementSize:    int(C.sizeof_FibEntry) + nDyns*int(C.sizeof_FibEntryDyn),
		PrivSize:       int(C.sizeof_Fib),
		Socket:         socket,
		NoCache:        true,
		SingleProducer: true,
		SingleConsumer: true,
	})
	if e != nil {
		return nil, e
	}
	t := &Table{
		mp:    mp,
		nDyns: nDyns,
	}

	mpC := (*C.struct_rte_mempool)(t.mp.Ptr())
	t.c = (*C.Fib)(C.rte_mempool_get_priv(mpC))
	*t.c = C.Fib{
		mp: mpC,
	}

	t.c.lfht = C.cds_lfht_new(C.ulong(cfg.NBuckets), C.ulong(cfg.NBuckets), C.ulong(cfg.NBuckets), 0, nil)
	if t.c.lfht == nil {
		t.mp.Close()
		return nil, errors.New("cds_lfht_new error")
	}

	t.c.startDepth = C.int(cfg.StartDepth)
	t.c.insertSeqNum = C.uint32_t(rand.Uint32())
	return t, nil
}
