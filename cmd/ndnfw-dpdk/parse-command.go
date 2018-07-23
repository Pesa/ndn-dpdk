package main

import (
	"flag"
	"os"

	"ndn-dpdk/appinit"
	"ndn-dpdk/container/fib"
	"ndn-dpdk/container/ndt"
)

type initConfig struct {
	Mempool           appinit.MempoolsCapacityConfig
	FaceQueueCapacity appinit.FaceQueueCapacityConfig
	Ndt               ndt.Config
	Fib               fib.Config
	Fwdp              fwdpInitConfig
}

type fwdpInitConfig struct {
	FwdQueueCapacity  int
	LatencySampleFreq int
	PcctCapacity      int
	CsCapacity        int
}

func parseCommand(args []string) (initCfg initConfig, e error) {
	initCfg.FaceQueueCapacity = appinit.TheFaceQueueCapacityConfig
	initCfg.Ndt.PrefixLen = 2
	initCfg.Ndt.IndexBits = 16
	initCfg.Ndt.SampleFreq = 8
	initCfg.Fib.MaxEntries = 65535
	initCfg.Fib.NBuckets = 256
	initCfg.Fib.StartDepth = 8
	initCfg.Fwdp.FwdQueueCapacity = 128
	initCfg.Fwdp.LatencySampleFreq = 16
	initCfg.Fwdp.PcctCapacity = 131071
	initCfg.Fwdp.CsCapacity = 32768

	flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	appinit.DeclareInitConfigFlag(flags, &initCfg)

	e = flags.Parse(args)
	if e != nil {
		return initConfig{}, e
	}

	return initCfg, nil
}