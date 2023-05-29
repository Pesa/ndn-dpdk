// Package urcu is a thin wrapper of Userspace RCU library.
package urcu

/*
#include "../../csrc/core/urcu.h"
*/
import "C"
import (
	"runtime"

	"github.com/usnistgov/ndn-dpdk/core/logging"
)

var logger = logging.New("urcu")

// ReadSide represents an RCU read-side thread.
// Fields are exported so that they can be updated to reflect what C code did.
type ReadSide struct {
	IsOnline bool
	NLocks   int
}

// Close unregisters current thread as an RCU read-side thread.
// If the thread is unregistered in C code, do not call this function.
func (*ReadSide) Close() error {
	C.rcu_unregister_thread()
	runtime.UnlockOSThread()
	return nil
}

// Offline marks current thread offline.
func (rs *ReadSide) Offline() {
	if rs.NLocks > 0 {
		logger.Panic("cannot go offline when locked")
	}
	rs.IsOnline = false
	C.rcu_thread_offline()
}

// Online marks current thread online.
func (rs *ReadSide) Online() {
	C.rcu_thread_online()
	rs.IsOnline = true
}

// Quiescent indicates current thread is quiescent.
func (rs *ReadSide) Quiescent() {
	if rs.NLocks > 0 {
		logger.Panic("cannot go quiescent when locked")
	}
	C.rcu_quiescent_state()
}

// Lock obtains read-side lock.
func (rs *ReadSide) Lock() {
	if !rs.IsOnline {
		logger.Panic("cannot lock when offline")
	}
	rs.NLocks++
	C.rcu_read_lock()
}

// Unlock releases read-side lock.
func (rs *ReadSide) Unlock() {
	if rs.NLocks <= 0 {
		return
	}
	C.rcu_read_unlock()
	rs.NLocks--
}

// NewReadSide registers current thread as an RCU read-side thread.
// If the thread is registered in C code, do not call this function, use a zero ReadSide instead.
func NewReadSide() *ReadSide {
	runtime.LockOSThread()
	C.rcu_register_thread()
	return &ReadSide{true, 0}
}
