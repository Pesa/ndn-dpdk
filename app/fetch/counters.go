package fetch

/*
#include "../../csrc/fetch/tcpcubic.h"
*/
import "C"
import (
	"fmt"
	"time"

	"github.com/usnistgov/ndn-dpdk/dpdk/eal"
)

// Counters contains counters of Logic.
type Counters struct {
	Time      time.Time     `json:"time"`
	LastRtt   time.Duration `json:"lastRtt"`
	SRtt      time.Duration `json:"sRtt"`
	Rto       time.Duration `json:"rto"`
	Cwnd      int           `json:"cwnd"`
	NInFlight uint32        `json:"nInFlight"` // number of in-flight Interests
	NTxRetx   uint64        `json:"nTxRetx"`   // number of retransmitted Interests
	NRxData   uint64        `json:"nRxData"`   // number of Data satisfying pending Interests
}

// Counters retrieves counters.
func (fl *Logic) Counters() (cnt Counters) {
	cnt.Time = time.Now()
	cnt.LastRtt = eal.FromTscDuration(int64(fl.rtte.last))
	cnt.SRtt = eal.FromTscDuration(int64(fl.rtte.rttv.sRtt))
	cnt.Rto = eal.FromTscDuration(int64(fl.rtte.rto))
	cnt.Cwnd = int(C.TcpCubic_GetCwnd(&fl.ca))
	cnt.NInFlight = uint32(fl.nInFlight)
	cnt.NTxRetx = uint64(fl.nTxRetx)
	cnt.NRxData = uint64(fl.nRxData)
	return cnt
}

func (cnt Counters) String() string {
	return fmt.Sprintf("rtt=%dms srtt=%dms rto=%dms cwnd=%d %dP %dR %dD",
		cnt.LastRtt.Milliseconds(), cnt.SRtt.Milliseconds(), cnt.Rto.Milliseconds(),
		cnt.Cwnd, cnt.NInFlight, cnt.NTxRetx, cnt.NRxData)
}
