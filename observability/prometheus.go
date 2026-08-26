package observability

import (
	"fmt"
	"io"
	"strings"
)

const defaultMetricPrefix = "gnalloy_channel"

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
	prefix := e.prefix
	write := func(suffix string, kind string, help string, value any) error {
		name := prefix + "_" + suffix
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n%s %v\n", name, help, name, kind, name, value); err != nil {
			return err
		}
		return nil
	}
	for _, metric := range []struct {
		suffix string
		kind   string
		help   string
		value  any
	}{
		{"registered_total", "counter", "Total registered channel events.", metrics.RegisteredChannels},
		{"unregistered_total", "counter", "Total unregistered channel events.", metrics.UnregisteredChannels},
		{"active_transitions_total", "counter", "Total channel active transitions.", metrics.ActiveTransitions},
		{"inactive_transitions_total", "counter", "Total channel inactive transitions.", metrics.InactiveTransitions},
		{"active", "gauge", "Currently active channels.", metrics.ActiveChannels},
		{"inbound_messages_total", "counter", "Total inbound messages.", metrics.InboundMessages},
		{"inbound_bytes_total", "counter", "Total inbound bytes.", metrics.InboundBytes},
		{"inbound_completions_total", "counter", "Total inbound read-complete events.", metrics.InboundCompletions},
		{"outbound_messages_total", "counter", "Total outbound messages.", metrics.OutboundMessages},
		{"outbound_bytes_total", "counter", "Total outbound bytes.", metrics.OutboundBytes},
		{"flushes_total", "counter", "Total flush events.", metrics.Flushes},
		{"closes_total", "counter", "Total close events.", metrics.Closes},
		{"exceptions_total", "counter", "Total exception events.", metrics.Exceptions},
		{"inbound_read_duration_nanoseconds_total", "counter", "Total inbound read handler duration in nanoseconds.", metrics.InboundReadNanos},
		{"inbound_read_duration_nanoseconds_max", "gauge", "Max inbound read handler duration in nanoseconds.", metrics.MaxInboundReadNanos},
		{"outbound_write_duration_nanoseconds_total", "counter", "Total outbound write handler duration in nanoseconds.", metrics.OutboundWriteNanos},
		{"outbound_write_duration_nanoseconds_max", "gauge", "Max outbound write handler duration in nanoseconds.", metrics.MaxOutboundWriteNanos},
		{"flush_duration_nanoseconds_total", "counter", "Total flush handler duration in nanoseconds.", metrics.FlushNanos},
		{"flush_duration_nanoseconds_max", "gauge", "Max flush handler duration in nanoseconds.", metrics.MaxFlushNanos},
		{"close_duration_nanoseconds_total", "counter", "Total close handler duration in nanoseconds.", metrics.CloseNanos},
		{"close_duration_nanoseconds_max", "gauge", "Max close handler duration in nanoseconds.", metrics.MaxCloseNanos},
	} {
		if err := write(metric.suffix, metric.kind, metric.help, metric.value); err != nil {
			return err
		}
	}
	return nil
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
