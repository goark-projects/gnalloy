package parity

import (
	"math"
	"sort"
)

// StatsSummary 汇总同一场景的多次正式采样。
type StatsSummary struct {
	Scenario            string  `json:"scenario"`
	Framework           string  `json:"framework,omitempty"`
	Protocol            string  `json:"protocol,omitempty"`
	Backend             string  `json:"backend,omitempty"`
	Samples             int     `json:"samples"`
	MinThroughputOps    float64 `json:"minThroughputOps"`
	MedianThroughputOps float64 `json:"medianThroughputOps"`
	MaxThroughputOps    float64 `json:"maxThroughputOps"`
	MeanThroughputOps   float64 `json:"meanThroughputOps"`
	MedianNsPerOp       float64 `json:"medianNsPerOp"`
	TotalErrors         int64   `json:"totalErrors"`
}

func statsSummaries(report Report) []StatsSummary {
	summaries := make([]StatsSummary, 0, len(report.Scenarios))
	for _, result := range report.Scenarios {
		summary, ok := summarizeScenarioStats(result)
		if ok {
			summaries = append(summaries, summary)
		}
	}
	return summaries
}

func summarizeScenarioStats(result ScenarioResult) (StatsSummary, bool) {
	if len(result.Stats) < 2 {
		return StatsSummary{}, false
	}
	throughputs := make([]float64, 0, len(result.Stats))
	nsPerOp := make([]float64, 0, len(result.Stats))
	summary := StatsSummary{
		Scenario: result.Scenario.Name,
		Samples:  len(result.Stats),
	}
	for i, stat := range result.Stats {
		if i == 0 {
			summary.Framework = stat.Framework
			summary.Protocol = stat.Protocol
			summary.Backend = stat.Backend
		}
		if stat.ThroughputOpsPerSec > 0 {
			throughputs = append(throughputs, stat.ThroughputOpsPerSec)
		}
		if stat.Elapsed > 0 && stat.TotalRequests > 0 {
			nsPerOp = append(nsPerOp, float64(stat.Elapsed.Nanoseconds())/float64(stat.TotalRequests))
		}
		summary.TotalErrors += stat.Errors
	}
	if len(throughputs) == 0 {
		return StatsSummary{}, false
	}
	sort.Float64s(throughputs)
	summary.MinThroughputOps = throughputs[0]
	summary.MedianThroughputOps = medianSorted(throughputs)
	summary.MaxThroughputOps = throughputs[len(throughputs)-1]
	summary.MeanThroughputOps = mean(throughputs)
	sort.Float64s(nsPerOp)
	summary.MedianNsPerOp = medianSorted(nsPerOp)
	return summary, true
}

func medianSorted(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	mid := len(values) / 2
	if len(values)%2 == 1 {
		return values[mid]
	}
	return (values[mid-1] + values[mid]) / 2
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var total float64
	for _, value := range values {
		total += value
	}
	return math.Round(total/float64(len(values))*100) / 100
}
