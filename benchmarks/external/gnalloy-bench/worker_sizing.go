package main

import "goark.dev/gnalloy/transport"

const (
	linuxNativePollerAutoWorkerLimit = 4
	windowsIOCPAutoWorkerLimit       = 8
)

type workerSizingInput struct {
	GOOS       string
	Backend    transport.BackendKind
	GOMAXPROCS int
}

func defaultWorkerCount(input workerSizingInput) int {
	workers := normalizeWorkerCPUCount(input.GOMAXPROCS)
	if input.GOOS == "linux" && isLinuxNativePoller(input.Backend) && workers > linuxNativePollerAutoWorkerLimit {
		return linuxNativePollerAutoWorkerLimit
	}
	if input.GOOS == "windows" && input.Backend == transport.BackendIOCP && workers > windowsIOCPAutoWorkerLimit {
		return windowsIOCPAutoWorkerLimit
	}
	return workers
}

func isLinuxNativePoller(backend transport.BackendKind) bool {
	return backend == transport.BackendEpoll || backend == transport.BackendIOUring
}

func normalizeWorkerCPUCount(cpus int) int {
	if cpus <= 0 {
		return 1
	}
	return cpus
}
