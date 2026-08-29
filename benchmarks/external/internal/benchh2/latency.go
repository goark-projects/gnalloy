package benchh2

import (
	"sort"
	"time"
)

func latencySamplingEnabled(rate int) bool {
	return rate > 0
}

func shouldRecordLatency(messageIndex int, rate int) bool {
	return rate > 0 && messageIndex%rate == 0
}

func elapsedLatencyNanos(started time.Time) int64 {
	elapsed := time.Since(started).Nanoseconds()
	if elapsed <= 0 {
		return 1
	}
	return elapsed
}

func newLatencySamples(messages int, rate int) []int64 {
	if !latencySamplingEnabled(rate) {
		return nil
	}
	capacity := messages / rate
	if messages%rate != 0 {
		capacity++
	}
	if capacity < 1 {
		capacity = 1
	}
	return make([]int64, 0, capacity)
}

func summarizeLatencySamples(samples []int64) LatencySummary {
	if len(samples) == 0 {
		return LatencySummary{}
	}
	sort.Slice(samples, func(i, j int) bool {
		return samples[i] < samples[j]
	})
	return LatencySummary{
		Samples: int64(len(samples)),
		P50:     percentileDuration(samples, 0.50),
		P95:     percentileDuration(samples, 0.95),
		P99:     percentileDuration(samples, 0.99),
		P999:    percentileDuration(samples, 0.999),
		Max:     time.Duration(samples[len(samples)-1]),
	}
}

func percentileDuration(sorted []int64, q float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if q <= 0 {
		return time.Duration(sorted[0])
	}
	if q >= 1 {
		return time.Duration(sorted[len(sorted)-1])
	}
	index := int(float64(len(sorted)-1) * q)
	return time.Duration(sorted[index])
}

func estimateLatencySampleCount(connections int, messages int, rate int) int {
	if connections <= 0 || messages <= 0 || rate <= 0 {
		return 0
	}
	perConnection := messages / rate
	if messages%rate != 0 {
		perConnection++
	}
	return connections * perConnection
}
