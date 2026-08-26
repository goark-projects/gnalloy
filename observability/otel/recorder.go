package otel

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"

	"goark.dev/gnalloy/transport"
)

const (
	defaultScopeName = "goark.dev/gnalloy"
	defaultPrefix    = "gnalloy.channel"
)

// ContextProvider 为热路径记录提供上下文。
type ContextProvider func() context.Context

// Config 描述 OpenTelemetry ChannelRecorder 适配策略。
type Config struct {
	Meter      metric.Meter
	ScopeName  string
	Prefix     string
	Attributes []attribute.KeyValue
	Context    ContextProvider
}

// Recorder 把 ChannelRecorder 事件写入 OpenTelemetry metric instruments。
type Recorder struct {
	ctx  ContextProvider
	opts metric.MeasurementOption

	registered          metric.Int64Counter
	unregistered        metric.Int64Counter
	activeTransitions   metric.Int64Counter
	inactiveTransitions metric.Int64Counter
	activeChannels      metric.Int64UpDownCounter
	inboundMessages     metric.Int64Counter
	inboundBytes        metric.Int64Counter
	inboundCompletions  metric.Int64Counter
	outboundMessages    metric.Int64Counter
	outboundBytes       metric.Int64Counter
	flushes             metric.Int64Counter
	closes              metric.Int64Counter
	exceptions          metric.Int64Counter

	inboundReadDuration   metric.Int64Histogram
	outboundWriteDuration metric.Int64Histogram
	flushDuration         metric.Int64Histogram
	closeDuration         metric.Int64Histogram
}

// NewRecorder 创建 OTel ChannelRecorder。
func NewRecorder(cfg Config) (*Recorder, error) {
	meter := cfg.Meter
	if meter == nil {
		scope := strings.TrimSpace(cfg.ScopeName)
		if scope == "" {
			scope = defaultScopeName
		}
		meter = noop.NewMeterProvider().Meter(scope)
	}
	prefix := strings.TrimSpace(cfg.Prefix)
	if prefix == "" {
		prefix = defaultPrefix
	}
	prefix = sanitizeInstrumentPrefix(prefix)
	opt := metric.WithAttributeSet(attribute.NewSet(cfg.Attributes...))
	recorder := &Recorder{
		ctx:  normalizeContextProvider(cfg.Context),
		opts: opt,
	}
	var err error
	if recorder.registered, err = newCounter(meter, prefix, "registered.total", "Channel registered events."); err != nil {
		return nil, err
	}
	if recorder.unregistered, err = newCounter(meter, prefix, "unregistered.total", "Channel unregistered events."); err != nil {
		return nil, err
	}
	if recorder.activeTransitions, err = newCounter(meter, prefix, "active.transitions.total", "Channel active transitions."); err != nil {
		return nil, err
	}
	if recorder.inactiveTransitions, err = newCounter(meter, prefix, "inactive.transitions.total", "Channel inactive transitions."); err != nil {
		return nil, err
	}
	if recorder.activeChannels, err = meter.Int64UpDownCounter(prefix+".active", metric.WithDescription("Currently active channels."), metric.WithUnit("1")); err != nil {
		return nil, instrumentError(prefix+".active", err)
	}
	if recorder.inboundMessages, err = newCounter(meter, prefix, "inbound.messages.total", "Inbound messages."); err != nil {
		return nil, err
	}
	if recorder.inboundBytes, err = newByteCounter(meter, prefix, "inbound.bytes.total", "Inbound bytes."); err != nil {
		return nil, err
	}
	if recorder.inboundCompletions, err = newCounter(meter, prefix, "inbound.completions.total", "Inbound read-complete events."); err != nil {
		return nil, err
	}
	if recorder.outboundMessages, err = newCounter(meter, prefix, "outbound.messages.total", "Outbound messages."); err != nil {
		return nil, err
	}
	if recorder.outboundBytes, err = newByteCounter(meter, prefix, "outbound.bytes.total", "Outbound bytes."); err != nil {
		return nil, err
	}
	if recorder.flushes, err = newCounter(meter, prefix, "flushes.total", "Flush events."); err != nil {
		return nil, err
	}
	if recorder.closes, err = newCounter(meter, prefix, "closes.total", "Close events."); err != nil {
		return nil, err
	}
	if recorder.exceptions, err = newCounter(meter, prefix, "exceptions.total", "Exception events."); err != nil {
		return nil, err
	}
	if recorder.inboundReadDuration, err = newDurationHistogram(meter, prefix, "inbound.read.duration", "Inbound read handler duration."); err != nil {
		return nil, err
	}
	if recorder.outboundWriteDuration, err = newDurationHistogram(meter, prefix, "outbound.write.duration", "Outbound write handler duration."); err != nil {
		return nil, err
	}
	if recorder.flushDuration, err = newDurationHistogram(meter, prefix, "flush.duration", "Flush handler duration."); err != nil {
		return nil, err
	}
	if recorder.closeDuration, err = newDurationHistogram(meter, prefix, "close.duration", "Close handler duration."); err != nil {
		return nil, err
	}
	return recorder, nil
}

func (r *Recorder) RecordChannelRegistered(transport.ChannelID) {
	r.registered.Add(r.context(), 1, r.opts)
}

func (r *Recorder) RecordChannelUnregistered(transport.ChannelID) {
	r.unregistered.Add(r.context(), 1, r.opts)
}

func (r *Recorder) RecordChannelActive(transport.ChannelID) {
	ctx := r.context()
	r.activeTransitions.Add(ctx, 1, r.opts)
	r.activeChannels.Add(ctx, 1, r.opts)
}

func (r *Recorder) RecordChannelInactive(transport.ChannelID) {
	ctx := r.context()
	r.inactiveTransitions.Add(ctx, 1, r.opts)
	r.activeChannels.Add(ctx, -1, r.opts)
}

func (r *Recorder) RecordChannelRead(_ transport.ChannelID, bytes int64) {
	ctx := r.context()
	r.inboundMessages.Add(ctx, 1, r.opts)
	if bytes > 0 {
		r.inboundBytes.Add(ctx, bytes, r.opts)
	}
}

func (r *Recorder) RecordChannelReadComplete(transport.ChannelID) {
	r.inboundCompletions.Add(r.context(), 1, r.opts)
}

func (r *Recorder) RecordChannelWrite(_ transport.ChannelID, bytes int64) {
	ctx := r.context()
	r.outboundMessages.Add(ctx, 1, r.opts)
	if bytes > 0 {
		r.outboundBytes.Add(ctx, bytes, r.opts)
	}
}

func (r *Recorder) RecordChannelFlush(transport.ChannelID) {
	r.flushes.Add(r.context(), 1, r.opts)
}

func (r *Recorder) RecordChannelClose(transport.ChannelID) {
	r.closes.Add(r.context(), 1, r.opts)
}

func (r *Recorder) RecordException(transport.ChannelID, error) {
	r.exceptions.Add(r.context(), 1, r.opts)
}

func (r *Recorder) RecordChannelReadDuration(_ transport.ChannelID, duration time.Duration) {
	r.recordDuration(r.inboundReadDuration, duration)
}

func (r *Recorder) RecordChannelWriteDuration(_ transport.ChannelID, duration time.Duration) {
	r.recordDuration(r.outboundWriteDuration, duration)
}

func (r *Recorder) RecordChannelFlushDuration(_ transport.ChannelID, duration time.Duration) {
	r.recordDuration(r.flushDuration, duration)
}

func (r *Recorder) RecordChannelCloseDuration(_ transport.ChannelID, duration time.Duration) {
	r.recordDuration(r.closeDuration, duration)
}

func (r *Recorder) recordDuration(histogram metric.Int64Histogram, duration time.Duration) {
	if duration <= 0 {
		return
	}
	histogram.Record(r.context(), int64(duration), r.opts)
}

func (r *Recorder) context() context.Context {
	ctx := r.ctx()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func newCounter(meter metric.Meter, prefix string, suffix string, description string) (metric.Int64Counter, error) {
	name := prefix + "." + suffix
	counter, err := meter.Int64Counter(name, metric.WithDescription(description), metric.WithUnit("1"))
	if err != nil {
		return nil, instrumentError(name, err)
	}
	return counter, nil
}

func newByteCounter(meter metric.Meter, prefix string, suffix string, description string) (metric.Int64Counter, error) {
	name := prefix + "." + suffix
	counter, err := meter.Int64Counter(name, metric.WithDescription(description), metric.WithUnit("By"))
	if err != nil {
		return nil, instrumentError(name, err)
	}
	return counter, nil
}

func newDurationHistogram(meter metric.Meter, prefix string, suffix string, description string) (metric.Int64Histogram, error) {
	name := prefix + "." + suffix
	histogram, err := meter.Int64Histogram(name, metric.WithDescription(description), metric.WithUnit("ns"))
	if err != nil {
		return nil, instrumentError(name, err)
	}
	return histogram, nil
}

func normalizeContextProvider(provider ContextProvider) ContextProvider {
	if provider != nil {
		return provider
	}
	return context.Background
}

func sanitizeInstrumentPrefix(prefix string) string {
	var b strings.Builder
	b.Grow(len(prefix))
	prevDot := false
	for _, r := range prefix {
		valid := r == '_' || r == '.' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
		if !valid {
			r = '_'
		}
		if r == '.' {
			if prevDot || b.Len() == 0 {
				continue
			}
			prevDot = true
		} else {
			prevDot = false
		}
		b.WriteRune(r)
	}
	out := strings.Trim(b.String(), "._")
	if out == "" {
		return defaultPrefix
	}
	return out
}

func instrumentError(name string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: create instrument %q: %w", ErrInvalidMeter, name, err)
}
