package observability

import "io"

// Exporter 将聚合指标快照导出到外部系统。
type Exporter interface {
	Export(w io.Writer, metrics ChannelMetrics) error
}

// ExportSnapshot 从 Snapshotter 读取快照并导出。
func ExportSnapshot(w io.Writer, snapshotter Snapshotter, exporter Exporter) error {
	if snapshotter == nil || exporter == nil {
		return ErrInvalidExporter
	}
	return exporter.Export(w, snapshotter.Snapshot())
}
