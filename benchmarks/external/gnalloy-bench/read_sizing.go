package main

const minimumBenchmarkReadBufferSize = 4096

func defaultReadBufferSize(payload int) int {
	if payload > minimumBenchmarkReadBufferSize {
		return payload
	}
	return minimumBenchmarkReadBufferSize
}
