package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"goark.dev/gnalloy/examples/internal/stresscheck"
	"goark.dev/gnalloy/examples/internal/stressclient"
)

func main() {
	var (
		addr                   string
		backend                string
		protocol               string
		scenario               string
		boss                   int
		workers                int
		connections            int
		messages               int
		payloadSize            int
		timeout                time.Duration
		delay                  time.Duration
		drain                  time.Duration
		reusePort              bool
		mmap                   bool
		mmapFallback           bool
		mmapBlockSize          int
		mmapBlocks             int
		iouringEntries         uint
		iouringSQPoll          bool
		iouringSQPollAffinity  bool
		iouringSQPollCPU       int
		iouringSQPollIdle      uint
		iouringMultishotAccept bool
		iouringFixedBuffers    bool
	)
	flag.StringVar(&addr, "addr", "127.0.0.1:0", "listen address")
	flag.StringVar(&backend, "backend", "default", "poller backend")
	flag.StringVar(&protocol, "protocol", string(stresscheck.ProtocolBoth), "protocol: raw, length-field or both")
	flag.StringVar(&scenario, "scenario", string(stressclient.ScenarioMixed), "scenario: long, short, half-frame, slow or mixed")
	flag.IntVar(&boss, "boss", 1, "boss event loop count")
	flag.IntVar(&workers, "workers", 2, "worker event loop count")
	flag.IntVar(&connections, "connections", 32, "concurrent connections")
	flag.IntVar(&messages, "messages", 32, "messages per connection")
	flag.IntVar(&payloadSize, "payload-size", 64, "payload bytes")
	flag.DurationVar(&timeout, "timeout", 30*time.Second, "overall timeout")
	flag.DurationVar(&delay, "delay", time.Millisecond, "delay for half-frame and slow scenarios")
	flag.DurationVar(&drain, "drain-timeout", 5*time.Second, "timeout waiting for active connections and allocator use to drain")
	flag.BoolVar(&reusePort, "reuseport", false, "enable SO_REUSEPORT when supported")
	flag.BoolVar(&mmap, "mmap", false, "use per-worker mmap allocator")
	flag.BoolVar(&mmapFallback, "mmap-fallback", true, "fallback to heap allocator when mmap is unsupported")
	flag.IntVar(&mmapBlockSize, "mmap-block-size", 4096, "mmap allocator block size")
	flag.IntVar(&mmapBlocks, "mmap-blocks", 4096, "mmap allocator block count per worker")
	flag.UintVar(&iouringEntries, "iouring-entries", 0, "io_uring queue depth")
	flag.BoolVar(&iouringSQPoll, "iouring-sqpoll", false, "enable io_uring SQPOLL")
	flag.BoolVar(&iouringSQPollAffinity, "iouring-sqpoll-affinity", false, "pin io_uring SQPOLL kernel thread")
	flag.IntVar(&iouringSQPollCPU, "iouring-sqpoll-cpu", 0, "io_uring SQPOLL CPU id")
	flag.UintVar(&iouringSQPollIdle, "iouring-sqpoll-idle-ms", 0, "io_uring SQPOLL idle timeout in milliseconds")
	flag.BoolVar(&iouringMultishotAccept, "iouring-multishot-accept", false, "enable io_uring multishot accept")
	flag.BoolVar(&iouringFixedBuffers, "iouring-fixed-buffers", false, "register mmap allocator blocks as io_uring fixed buffers")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result, err := stresscheck.Run(ctx, stresscheck.Config{
		Addr:                    addr,
		BackendName:             backend,
		Protocol:                stressclient.Protocol(protocol),
		Scenario:                stressclient.Scenario(scenario),
		Boss:                    boss,
		Workers:                 workers,
		Connections:             connections,
		MessagesPerConn:         messages,
		PayloadSize:             payloadSize,
		Timeout:                 timeout,
		Delay:                   delay,
		DrainTimeout:            drain,
		ReusePort:               reusePort,
		Mmap:                    mmap,
		MmapFallback:            mmapFallback,
		MmapBlockSize:           mmapBlockSize,
		MmapBlocks:              mmapBlocks,
		IOUringEntries:          iouringEntries,
		IOUringSQPoll:           iouringSQPoll,
		IOUringSQPollAffinity:   iouringSQPollAffinity,
		IOUringSQPollCPU:        iouringSQPollCPU,
		IOUringSQPollIdleMillis: iouringSQPollIdle,
		IOUringMultishotAccept:  iouringMultishotAccept,
		IOUringFixedBuffers:     iouringFixedBuffers,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("stress-check backend=%s protocol=%s scenario=%s requests=%d errors=%d elapsed=%s leaks=0\n",
		backend,
		protocol,
		scenario,
		result.Requests,
		result.Errors,
		result.Elapsed,
	)
	if result.Errors != 0 {
		os.Exit(2)
	}
}
