package main

import "goark.dev/gnalloy/transport"

const windowsIOCPAutoWorkerLimit = 8

type workerSizingInput struct {
	GOOS       string
	Backend    transport.BackendKind
	GOMAXPROCS int
}

func defaultWorkerCount(input workerSizingInput) int {
	workers := normalizeWorkerCPUCount(input.GOMAXPROCS)
	if input.GOOS == "windows" && input.Backend == transport.BackendIOCP && workers > windowsIOCPAutoWorkerLimit {
		return windowsIOCPAutoWorkerLimit
	}
	return workers
}

func normalizeWorkerCPUCount(cpus int) int {
	if cpus <= 0 {
		return 1
	}
	return cpus
}
