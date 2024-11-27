package ethdev_test

import (
	"log"
	"testing"
	"time"

	"github.com/usnistgov/ndn-dpdk/core/cptr"
	"github.com/usnistgov/ndn-dpdk/dpdk/eal"
	"github.com/usnistgov/ndn-dpdk/dpdk/ethdev/ethringdev"
	"github.com/usnistgov/ndn-dpdk/dpdk/pktmbuf"
	"go4.org/must"
)

func TestEthDev(t *testing.T) {
	assert, require := makeAR(t)

	pair, e := ethringdev.NewPair(ethringdev.PairConfig{RxPool: directMp})
	require.NoError(e)
	defer pair.Close()
	pair.PortA.Start(pair.EthDevConfig())
	pair.PortB.Start(pair.EthDevConfig())
	assert.False(pair.PortA.IsDown())
	assert.False(pair.PortB.IsDown())

	rxq := pair.PortA.RxQueues()[0]
	txq := pair.PortB.TxQueues()[0]

	const rxBurstSize = 6
	const txLoops = 100000
	const txBurstSize = 10
	const maxTxRetry = 20
	const txRetryInterval = 1 * time.Millisecond
	const rxFinishWait = 10 * time.Millisecond

	nReceived := 0
	rxBurstSizeFreq := map[int]int{}
	rxQuit := make(chan bool)
	eal.Workers[0].RemoteLaunch(cptr.Func0.Void(func() {
		for {
			vec := make(pktmbuf.Vector, rxBurstSize)
			burstSize := rxq.RxBurst(vec)
			rxBurstSizeFreq[burstSize]++
			for _, pkt := range vec[:burstSize] {
				if assert.NotNil(pkt) {
					nReceived++
					assert.Equal(1, pkt.Len(), "bad RX length at %d", nReceived)
				}
			}
			must.Close(vec)

			select {
			case <-rxQuit:
				return
			default:
			}
		}
	}))

	txRetryFreq := map[int]int{}
	eal.Workers[1].RemoteLaunch(cptr.Func0.Void(func() {
		for i := range txLoops {
			vec := directMp.MustAlloc(txBurstSize)
			for j := range txBurstSize {
				vec[j].Append([]byte{byte(j)})
			}

			nSent := 0
			for nRetries := range maxTxRetry {
				res := txq.TxBurst(vec[nSent:])
				nSent = nSent + int(res)
				if nSent == txBurstSize {
					txRetryFreq[nRetries]++
					break
				}
				time.Sleep(txRetryInterval)
			}
			assert.Equal(txBurstSize, nSent, "TxBurst incomplete at loop %d", i)
		}
	}))
	eal.Workers[1].Wait()
	time.Sleep(rxFinishWait)
	rxQuit <- true
	eal.Workers[0].Wait()

	log.Println("portA.stats=", pair.PortA.Stats())
	log.Println("portB.stats=", pair.PortB.Stats())
	log.Println("txRetryFreq=", txRetryFreq)
	log.Println("rxBurstSizeFreq=", rxBurstSizeFreq)
	assert.True(nReceived <= txLoops*txBurstSize)
	assert.InEpsilon(txLoops*txBurstSize, nReceived, 0.05)
}
