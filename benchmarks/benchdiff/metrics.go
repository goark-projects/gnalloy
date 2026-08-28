package benchdiff

import (
	"sort"
	"strings"

	"goark.dev/gnalloy/benchmarks/parity"
)

// Sample 是携带包名上下文的 Go benchmark 采样。
type Sample struct {
	Package string
	Metric  parity.BenchmarkMetric
}

// Summary 是同一 benchmark 多次采样后的中位数视图。
type Summary struct {
	Samples     int
	NsPerOp     float64
	BytesPerOp  float64
	AllocsPerOp float64
}

// Comparison 描述候选版本相对基线版本的同机变化。
type Comparison struct {
	Package                  string
	Benchmark                string
	Base                     Summary
	Candidate                Summary
	NsPerOpChangePercent     float64
	BytesPerOpChangePercent  float64
	AllocsPerOpChangePercent float64
}

// Missing 描述只出现在其中一个版本里的 benchmark。
type Missing struct {
	Package   string
	Benchmark string
	Side      string
}

// ParseGoBenchOutput 解析 go test -bench 输出，并保留 pkg 行提供的包名。
func ParseGoBenchOutput(output string) []Sample {
	var samples []Sample
	currentPackage := ""
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "pkg:") {
			currentPackage = strings.TrimSpace(strings.TrimPrefix(line, "pkg:"))
			continue
		}
		metrics := parity.ParseBenchmarkMetrics(line)
		if len(metrics) == 0 {
			continue
		}
		for _, metric := range metrics {
			samples = append(samples, Sample{Package: currentPackage, Metric: metric})
		}
	}
	return samples
}

// CompareSamples 对比基线与候选采样，只对共同存在的 benchmark 计算变化率。
func CompareSamples(baseSamples []Sample, candidateSamples []Sample) ([]Comparison, []Missing) {
	base := groupSamples(baseSamples)
	candidate := groupSamples(candidateSamples)
	keys := make([]metricKey, 0, len(base)+len(candidate))
	seen := make(map[metricKey]struct{}, len(base)+len(candidate))
	for key := range base {
		keys = appendMetricKey(keys, seen, key)
	}
	for key := range candidate {
		keys = appendMetricKey(keys, seen, key)
	}
	sort.Slice(keys, func(i int, j int) bool {
		if keys[i].Package == keys[j].Package {
			return keys[i].Benchmark < keys[j].Benchmark
		}
		return keys[i].Package < keys[j].Package
	})

	comparisons := make([]Comparison, 0, len(keys))
	missing := make([]Missing, 0)
	for _, key := range keys {
		baseMetrics, hasBase := base[key]
		candidateMetrics, hasCandidate := candidate[key]
		switch {
		case hasBase && hasCandidate:
			baseSummary := summarize(baseMetrics)
			candidateSummary := summarize(candidateMetrics)
			comparisons = append(comparisons, Comparison{
				Package:                  key.Package,
				Benchmark:                key.Benchmark,
				Base:                     baseSummary,
				Candidate:                candidateSummary,
				NsPerOpChangePercent:     changePercent(baseSummary.NsPerOp, candidateSummary.NsPerOp),
				BytesPerOpChangePercent:  changePercent(baseSummary.BytesPerOp, candidateSummary.BytesPerOp),
				AllocsPerOpChangePercent: changePercent(baseSummary.AllocsPerOp, candidateSummary.AllocsPerOp),
			})
		case hasBase:
			missing = append(missing, Missing{Package: key.Package, Benchmark: key.Benchmark, Side: "candidate"})
		default:
			missing = append(missing, Missing{Package: key.Package, Benchmark: key.Benchmark, Side: "base"})
		}
	}
	return comparisons, missing
}

type metricKey struct {
	Package   string
	Benchmark string
}

func appendMetricKey(keys []metricKey, seen map[metricKey]struct{}, key metricKey) []metricKey {
	if _, ok := seen[key]; ok {
		return keys
	}
	seen[key] = struct{}{}
	return append(keys, key)
}

func groupSamples(samples []Sample) map[metricKey][]parity.BenchmarkMetric {
	grouped := make(map[metricKey][]parity.BenchmarkMetric)
	for _, sample := range samples {
		key := metricKey{Package: sample.Package, Benchmark: sample.Metric.Name}
		grouped[key] = append(grouped[key], sample.Metric)
	}
	return grouped
}

func summarize(metrics []parity.BenchmarkMetric) Summary {
	nsPerOp := make([]float64, 0, len(metrics))
	bytesPerOp := make([]float64, 0, len(metrics))
	allocsPerOp := make([]float64, 0, len(metrics))
	for _, metric := range metrics {
		nsPerOp = append(nsPerOp, metric.NsPerOp)
		bytesPerOp = append(bytesPerOp, float64(metric.BytesPerOp))
		allocsPerOp = append(allocsPerOp, float64(metric.AllocsPerOp))
	}
	return Summary{
		Samples:     len(metrics),
		NsPerOp:     median(nsPerOp),
		BytesPerOp:  median(bytesPerOp),
		AllocsPerOp: median(allocsPerOp),
	}
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	mid := len(values) / 2
	if len(values)%2 == 1 {
		return values[mid]
	}
	return (values[mid-1] + values[mid]) / 2
}

func changePercent(base float64, candidate float64) float64 {
	if base == 0 {
		return 0
	}
	return (candidate - base) / base * 100
}
