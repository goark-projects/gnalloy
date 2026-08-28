package main

import (
	"testing"
	"time"
)

func TestSummarizeLatencySamples(t *testing.T) {
	summary := summarizeLatencySamples([]int64{
		int64(10 * time.Millisecond),
		int64(2 * time.Millisecond),
		int64(5 * time.Millisecond),
		int64(time.Millisecond),
	})
	if summary.Samples != 4 {
		t.Fatalf("samples=%d, want 4", summary.Samples)
	}
	if summary.P50 != 2*time.Millisecond || summary.P95 != 5*time.Millisecond || summary.Max != 10*time.Millisecond {
		t.Fatalf("summary=%+v", summary)
	}
}

func TestLatencySamplingPredicate(t *testing.T) {
	if shouldRecordLatency(1, 0) {
		t.Fatal("disabled sampler recorded latency")
	}
	if !shouldRecordLatency(4, 2) || shouldRecordLatency(5, 2) {
		t.Fatal("unexpected latency sampling predicate")
	}
}

func TestElapsedLatencyNanosIsPositive(t *testing.T) {
	if elapsedLatencyNanos(time.Now()) <= 0 {
		t.Fatal("elapsed latency must be positive")
	}
}
