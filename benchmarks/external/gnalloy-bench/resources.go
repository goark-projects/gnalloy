package main

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

type resourceSnapshot struct {
	RSSBytes       uint64
	HeapAllocBytes uint64
	HeapSysBytes   uint64
	HeapObjects    uint64
	GCCount        uint32
	GCPauseNanos   uint64
	Goroutines     int
}

type resourceDelta struct {
	RSSBytes       uint64
	HeapAllocBytes uint64
	HeapSysBytes   uint64
	HeapObjects    uint64
	GCCount        uint32
	GCPauseNanos   uint64
	Goroutines     int
}

func captureResourceSnapshot() resourceSnapshot {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	return resourceSnapshot{
		RSSBytes:       currentRSSBytes(),
		HeapAllocBytes: mem.HeapAlloc,
		HeapSysBytes:   mem.HeapSys,
		HeapObjects:    mem.HeapObjects,
		GCCount:        mem.NumGC,
		GCPauseNanos:   mem.PauseTotalNs,
		Goroutines:     runtime.NumGoroutine(),
	}
}

func resourceDeltaSince(start resourceSnapshot, end resourceSnapshot) resourceDelta {
	return resourceDelta{
		RSSBytes:       end.RSSBytes,
		HeapAllocBytes: end.HeapAllocBytes,
		HeapSysBytes:   end.HeapSysBytes,
		HeapObjects:    end.HeapObjects,
		GCCount:        end.GCCount - start.GCCount,
		GCPauseNanos:   end.GCPauseNanos - start.GCPauseNanos,
		Goroutines:     end.Goroutines,
	}
}

func currentRSSBytes() uint64 {
	if runtime.GOOS != "linux" {
		return 0
	}
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return pages * uint64(os.Getpagesize())
}
