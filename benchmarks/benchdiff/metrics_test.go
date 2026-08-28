package benchdiff

import (
	"math"
	"testing"
)

func TestParseGoBenchOutputTracksPackage(t *testing.T) {
	output := `goos: windows
goarch: amd64
pkg: goark.dev/gnalloy/buffer
BenchmarkPooledAllocator-16 100 10.00 ns/op 0 B/op 0 allocs/op
pkg: goark.dev/gnalloy/channel
BenchmarkPipeline-16 200 20.00 ns/op 8 B/op 1 allocs/op
`
	samples := ParseGoBenchOutput(output)
	if len(samples) != 2 {
		t.Fatalf("samples=%d, want 2", len(samples))
	}
	if samples[0].Package != "goark.dev/gnalloy/buffer" || samples[0].Metric.Name != "BenchmarkPooledAllocator-16" {
		t.Fatalf("sample=%+v", samples[0])
	}
	if samples[1].Package != "goark.dev/gnalloy/channel" || samples[1].Metric.BytesPerOp != 8 {
		t.Fatalf("sample=%+v", samples[1])
	}
}

func TestCompareSamplesUsesMedianAndReportsMissing(t *testing.T) {
	base := ParseGoBenchOutput(`pkg: goark.dev/gnalloy/buffer
BenchmarkAllocator-16 100 100.00 ns/op 8 B/op 1 allocs/op
BenchmarkAllocator-16 100 120.00 ns/op 8 B/op 1 allocs/op
BenchmarkBaseOnly-16 100 50.00 ns/op 0 B/op 0 allocs/op
`)
	candidate := ParseGoBenchOutput(`pkg: goark.dev/gnalloy/buffer
BenchmarkAllocator-16 100 80.00 ns/op 0 B/op 0 allocs/op
BenchmarkAllocator-16 100 90.00 ns/op 0 B/op 0 allocs/op
BenchmarkCandidateOnly-16 100 70.00 ns/op 0 B/op 0 allocs/op
`)
	comparisons, missing := CompareSamples(base, candidate)
	if len(comparisons) != 1 {
		t.Fatalf("comparisons=%+v", comparisons)
	}
	comparison := comparisons[0]
	if comparison.Base.NsPerOp != 110 || comparison.Candidate.NsPerOp != 85 {
		t.Fatalf("comparison=%+v", comparison)
	}
	if math.Abs(comparison.NsPerOpChangePercent-(-22.7272727)) > 0.0001 {
		t.Fatalf("ns delta=%f", comparison.NsPerOpChangePercent)
	}
	if len(missing) != 2 {
		t.Fatalf("missing=%+v", missing)
	}
}
