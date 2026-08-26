package observability

import (
	"io"
	"strconv"
	"strings"
	"sync"
)

const (
	defaultMetricPrefix       = "gnalloy_channel"
	initialPrometheusBufSize  = 4096
	maxPrometheusBufCacheSize = 64 << 10
)

var prometheusBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, initialPrometheusBufSize)
		return &buf
	},
}

// PrometheusConfig 描述 Prometheus 文本格式导出策略。
type PrometheusConfig struct {
	// Prefix 是指标名前缀，空值表示 gnalloy_channel。
	Prefix string
}

// PrometheusExporter 以 Prometheus text exposition format 导出 ChannelMetrics。
type PrometheusExporter struct {
	prefix string
}

// NewPrometheusExporter 创建 Prometheus 文本导出器。
func NewPrometheusExporter(cfg PrometheusConfig) *PrometheusExporter {
	prefix := strings.TrimSpace(cfg.Prefix)
	if prefix == "" {
		prefix = defaultMetricPrefix
	}
	return &PrometheusExporter{prefix: sanitizeMetricName(prefix)}
}

// Export 写出 Prometheus 文本指标。
func (e *PrometheusExporter) Export(w io.Writer, metrics ChannelMetrics) error {
	if e == nil || w == nil {
		return ErrInvalidExporter
	}

	bufPtr := prometheusBufferPool.Get().(*[]byte)
	writer := prometheusWriter{
		prefix: e.prefix,
		buf:    (*bufPtr)[:0],
	}
	writer.writeUint64("registered_total", "counter", "Total registered channel events.", metrics.RegisteredChannels)
	writer.writeUint64("unregistered_total", "counter", "Total unregistered channel events.", metrics.UnregisteredChannels)
	writer.writeUint64("active_transitions_total", "counter", "Total channel active transitions.", metrics.ActiveTransitions)
	writer.writeUint64("inactive_transitions_total", "counter", "Total channel inactive transitions.", metrics.InactiveTransitions)
	writer.writeInt64("active", "gauge", "Currently active channels.", metrics.ActiveChannels)
	writer.writeUint64("inbound_messages_total", "counter", "Total inbound messages.", metrics.InboundMessages)
	writer.writeUint64("inbound_bytes_total", "counter", "Total inbound bytes.", metrics.InboundBytes)
	writer.writeUint64("inbound_completions_total", "counter", "Total inbound read-complete events.", metrics.InboundCompletions)
	writer.writeUint64("outbound_messages_total", "counter", "Total outbound messages.", metrics.OutboundMessages)
	writer.writeUint64("outbound_bytes_total", "counter", "Total outbound bytes.", metrics.OutboundBytes)
	writer.writeUint64("flushes_total", "counter", "Total flush events.", metrics.Flushes)
	writer.writeUint64("closes_total", "counter", "Total close events.", metrics.Closes)
	writer.writeUint64("exceptions_total", "counter", "Total exception events.", metrics.Exceptions)
	writer.writeUint64("inbound_read_duration_nanoseconds_total", "counter", "Total inbound read handler duration in nanoseconds.", metrics.InboundReadNanos)
	writer.writeUint64("inbound_read_duration_nanoseconds_max", "gauge", "Max inbound read handler duration in nanoseconds.", metrics.MaxInboundReadNanos)
	writer.writeUint64("outbound_write_duration_nanoseconds_total", "counter", "Total outbound write handler duration in nanoseconds.", metrics.OutboundWriteNanos)
	writer.writeUint64("outbound_write_duration_nanoseconds_max", "gauge", "Max outbound write handler duration in nanoseconds.", metrics.MaxOutboundWriteNanos)
	writer.writeUint64("flush_duration_nanoseconds_total", "counter", "Total flush handler duration in nanoseconds.", metrics.FlushNanos)
	writer.writeUint64("flush_duration_nanoseconds_max", "gauge", "Max flush handler duration in nanoseconds.", metrics.MaxFlushNanos)
	writer.writeUint64("close_duration_nanoseconds_total", "counter", "Total close handler duration in nanoseconds.", metrics.CloseNanos)
	writer.writeUint64("close_duration_nanoseconds_max", "gauge", "Max close handler duration in nanoseconds.", metrics.MaxCloseNanos)

	_, err := w.Write(writer.buf)
	// 限制回池容量，避免异常大导出结果长期占用堆内存。
	if cap(writer.buf) <= maxPrometheusBufCacheSize {
		*bufPtr = writer.buf[:0]
		prometheusBufferPool.Put(bufPtr)
	}
	return err
}

type prometheusWriter struct {
	prefix string
	buf    []byte
}

func (w *prometheusWriter) writeUint64(suffix string, kind string, help string, value uint64) {
	w.writeHeader(suffix, kind, help)
	w.buf = strconv.AppendUint(w.buf, value, 10)
	w.buf = append(w.buf, '\n')
}

func (w *prometheusWriter) writeInt64(suffix string, kind string, help string, value int64) {
	w.writeHeader(suffix, kind, help)
	w.buf = strconv.AppendInt(w.buf, value, 10)
	w.buf = append(w.buf, '\n')
}

func (w *prometheusWriter) writeHeader(suffix string, kind string, help string) {
	w.writeString("# HELP ")
	w.writeName(suffix)
	w.writeString(" ")
	w.writeString(help)
	w.writeString("\n# TYPE ")
	w.writeName(suffix)
	w.writeString(" ")
	w.writeString(kind)
	w.writeString("\n")
	w.writeName(suffix)
	w.writeString(" ")
}

func (w *prometheusWriter) writeName(suffix string) {
	w.writeString(w.prefix)
	w.buf = append(w.buf, '_')
	w.writeString(suffix)
}

func (w *prometheusWriter) writeString(value string) {
	w.buf = append(w.buf, value...)
}

func sanitizeMetricName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for i, r := range name {
		valid := r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9'
		if valid {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	if b.Len() == 0 {
		return defaultMetricPrefix
	}
	return b.String()
}
