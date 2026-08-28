package benchdiff

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteMarkdownIncludesComparisonRows(t *testing.T) {
	report := Report{
		BaseLabel:      "HEAD",
		CandidateLabel: "working tree",
		Command:        []string{"go", "test", "-bench", "BenchmarkAllocator", "./buffer"},
		Comparisons: []Comparison{{
			Package:              "goark.dev/gnalloy/buffer",
			Benchmark:            "BenchmarkAllocator-16",
			Base:                 Summary{Samples: 2, NsPerOp: 110, BytesPerOp: 8, AllocsPerOp: 1},
			Candidate:            Summary{Samples: 2, NsPerOp: 85, BytesPerOp: 0, AllocsPerOp: 0},
			NsPerOpChangePercent: -22.7272,
		}, {
			Package:                  "goark.dev/gnalloy/buffer",
			Benchmark:                "BenchmarkAllocationRegression-16",
			Base:                     Summary{Samples: 1, NsPerOp: 10, BytesPerOp: 0, AllocsPerOp: 0},
			Candidate:                Summary{Samples: 1, NsPerOp: 11, BytesPerOp: 8, AllocsPerOp: 1},
			NsPerOpChangePercent:     10,
			BytesPerOpChangePercent:  0,
			AllocsPerOpChangePercent: 0,
		}},
	}
	var out bytes.Buffer
	if err := WriteMarkdown(&out, report); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"# gnalloy benchmark diff", "HEAD", "working tree", "BenchmarkAllocator-16", "-22.73%", "+inf"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
}
