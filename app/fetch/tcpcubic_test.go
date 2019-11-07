package fetch_test

import (
	"testing"
	"time"

	"ndn-dpdk/app/fetch"
	"ndn-dpdk/dpdk"
)

func TestTcpCubic(t *testing.T) {
	assert, _ := makeAR(t)

	var ca fetch.TcpCubic
	ca.Init()

	assert.Equal(2, ca.GetCwnd())

	now := dpdk.TscNow()
	rtt := dpdk.ToTscDuration(100 * time.Millisecond)

	// slow start
	for i := 0; i < 98; i++ {
		ca.Increase(now, rtt)
		now = now.Add(5 * time.Millisecond)
	}
	assert.Equal(100, ca.GetCwnd())

	// enter congestion avoidance
	ca.Decrease(now, rtt)
	assert.Equal(70, ca.GetCwnd())
	now = now.Add(5 * time.Millisecond)

	// increase window
	firstCwnd := ca.GetCwnd()
	lastCwnd := firstCwnd
	for i := 0; i < 1000; i++ {
		ca.Increase(now, rtt)
		thisCwnd := ca.GetCwnd()
		assert.GreaterOrEqual(thisCwnd, lastCwnd)
		lastCwnd = thisCwnd
		now = now.Add(time.Millisecond)
	}
	assert.Greater(lastCwnd, firstCwnd)

	// decrease window
	ca.Decrease(now, rtt)
	thisCwnd := ca.GetCwnd()
	assert.Less(thisCwnd, lastCwnd)
	now = now.Add(5 * time.Millisecond)

	ca.Increase(now, rtt)
	lastCwnd = ca.GetCwnd()
	now = now.Add(5 * time.Millisecond)

	// decrease window again within RTT should have no effect
	ca.Decrease(now, rtt)
	thisCwnd = ca.GetCwnd()
	assert.Equal(lastCwnd, thisCwnd)
}
