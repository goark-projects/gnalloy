package benchh3

import (
	"testing"
	"time"
)

func TestAverageLatencyNanosUsesWindowMean(t *testing.T) {
	got := averageLatencyNanos(640*time.Microsecond, 64)
	if got != 10000 {
		t.Fatalf("latency=%d, want 10000", got)
	}
}

func TestAverageLatencyNanosKeepsPositiveFloor(t *testing.T) {
	if got := averageLatencyNanos(0, 64); got != 1 {
		t.Fatalf("latency=%d, want positive lower bound", got)
	}
	if got := averageLatencyNanos(time.Nanosecond, 64); got != 1 {
		t.Fatalf("latency=%d, want positive lower bound", got)
	}
}
