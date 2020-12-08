package ealthread

/*
#include "../../csrc/dpdk/thread.h"

#ifdef NDNDPDK_THREADSLEEP
#define ENABLE_THREADSLEEP 1
#else
#define ENABLE_THREADSLEEP 0
#endif
*/
import "C"
import (
	"time"
	"unsafe"
)

// Stopper abstracts how to tell a thread top stop.
type Stopper interface {
	// BeforeWait is invoked before lcore.Wait().
	BeforeWait()

	// AfterWait is invoked after lcore.Wait().
	AfterWait()
}

// StopFlag stops a thread by setting a boolean flag.
type StopFlag struct {
	c *C.ThreadStopFlag
}

// NewStopFlag constructs a StopFlag from initialized C pointer.
func NewStopFlag(c unsafe.Pointer) (stop StopFlag) {
	stop.c = (*C.ThreadStopFlag)(c)
	return stop
}

// InitStopFlag constructs and initializes a StopFlag.
func InitStopFlag(c unsafe.Pointer) (stop StopFlag) {
	stop = NewStopFlag(c)
	C.ThreadStopFlag_Init(stop.c)
	return stop
}

// BeforeWait requests a stop.
func (stop StopFlag) BeforeWait() {
	C.ThreadStopFlag_RequestStop(stop.c)
}

// AfterWait completes a stop request.
func (stop StopFlag) AfterWait() {
	C.ThreadStopFlag_FinishStop(stop.c)
}

// StopChan stops a thread by sending to a channel.
type StopChan chan bool

// NewStopChan constructs a StopChan.
func NewStopChan() (stop StopChan) {
	return make(StopChan)
}

// Continue returns true if the thread should continue.
// This should be invoked within the running thread.
func (stop StopChan) Continue() bool {
	if C.ENABLE_THREADSLEEP > 0 {
		time.Sleep(time.Nanosecond)
	}

	select {
	case <-stop:
		return false
	default:
		return true
	}
}

// BeforeWait requests a stop.
func (stop StopChan) BeforeWait() {
	stop <- true
}

// AfterWait completes a stop request.
func (stop StopChan) AfterWait() {
}

// RequestStop requests a stop.
// This may be used independent from Thread.
func (stop StopChan) RequestStop() {
	stop <- true
}
