package otel

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"

	"goark.dev/gnalloy/transport"
)

func TestRecorderMapsChannelEventsToInstruments(t *testing.T) {
	meter := newRecordingMeter()
	recorder, err := NewRecorder(Config{Meter: meter, Prefix: "gnalloy-test"})
	if err != nil {
		t.Fatal(err)
	}
	id := transport.ChannelID(9)

	recorder.RecordChannelRegistered(id)
	recorder.RecordChannelActive(id)
	recorder.RecordChannelRead(id, 128)
	recorder.RecordChannelRead(id, -1)
	recorder.RecordChannelReadComplete(id)
	recorder.RecordChannelWrite(id, 64)
	recorder.RecordChannelFlush(id)
	recorder.RecordException(id, errors.New("boom"))
	recorder.RecordChannelClose(id)
	recorder.RecordChannelReadDuration(id, 2*time.Nanosecond)
	recorder.RecordChannelWriteDuration(id, 3*time.Nanosecond)
	recorder.RecordChannelFlushDuration(id, 4*time.Nanosecond)
	recorder.RecordChannelCloseDuration(id, 5*time.Nanosecond)
	recorder.RecordChannelInactive(id)

	meter.assertCounter(t, "gnalloy_test.registered.total", 1)
	meter.assertCounter(t, "gnalloy_test.inbound.messages.total", 2)
	meter.assertCounter(t, "gnalloy_test.inbound.bytes.total", 128)
	meter.assertCounter(t, "gnalloy_test.outbound.bytes.total", 64)
	meter.assertCounter(t, "gnalloy_test.exceptions.total", 1)
	meter.assertUpDown(t, "gnalloy_test.active", 0)
	meter.assertHistogram(t, "gnalloy_test.inbound.read.duration", 2)
	meter.assertHistogram(t, "gnalloy_test.outbound.write.duration", 3)
	meter.assertHistogram(t, "gnalloy_test.flush.duration", 4)
	meter.assertHistogram(t, "gnalloy_test.close.duration", 5)
}

func TestRecorderUsesNoopMeterByDefault(t *testing.T) {
	recorder, err := NewRecorder(Config{})
	if err != nil {
		t.Fatal(err)
	}
	recorder.RecordChannelActive(1)
	recorder.RecordChannelInactive(1)
}

func TestRecorderWrapsInstrumentCreationError(t *testing.T) {
	want := errors.New("meter failed")
	_, err := NewRecorder(Config{Meter: failingMeter{err: want}})
	if !errors.Is(err, ErrInvalidMeter) || !errors.Is(err, want) {
		t.Fatalf("err=%v, want wrapped ErrInvalidMeter and meter error", err)
	}
}

type recordingMeter struct {
	noop.Meter
	mu         sync.Mutex
	counters   map[string]*recordingCounter
	updowns    map[string]*recordingUpDownCounter
	histograms map[string]*recordingHistogram
}

func newRecordingMeter() *recordingMeter {
	return &recordingMeter{
		counters:   make(map[string]*recordingCounter),
		updowns:    make(map[string]*recordingUpDownCounter),
		histograms: make(map[string]*recordingHistogram),
	}
}

func (m *recordingMeter) Int64Counter(name string, _ ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	counter := &recordingCounter{}
	m.counters[name] = counter
	return counter, nil
}

func (m *recordingMeter) Int64UpDownCounter(name string, _ ...metric.Int64UpDownCounterOption) (metric.Int64UpDownCounter, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	counter := &recordingUpDownCounter{}
	m.updowns[name] = counter
	return counter, nil
}

func (m *recordingMeter) Int64Histogram(name string, _ ...metric.Int64HistogramOption) (metric.Int64Histogram, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	histogram := &recordingHistogram{}
	m.histograms[name] = histogram
	return histogram, nil
}

func (m *recordingMeter) assertCounter(t *testing.T, name string, want int64) {
	t.Helper()
	m.mu.Lock()
	counter := m.counters[name]
	m.mu.Unlock()
	if counter == nil {
		t.Fatalf("counter %q was not created", name)
	}
	if got := counter.sum(); got != want {
		t.Fatalf("counter %q=%d, want %d", name, got, want)
	}
}

func (m *recordingMeter) assertUpDown(t *testing.T, name string, want int64) {
	t.Helper()
	m.mu.Lock()
	counter := m.updowns[name]
	m.mu.Unlock()
	if counter == nil {
		t.Fatalf("updown %q was not created", name)
	}
	if got := counter.sum(); got != want {
		t.Fatalf("updown %q=%d, want %d", name, got, want)
	}
}

func (m *recordingMeter) assertHistogram(t *testing.T, name string, want int64) {
	t.Helper()
	m.mu.Lock()
	histogram := m.histograms[name]
	m.mu.Unlock()
	if histogram == nil {
		t.Fatalf("histogram %q was not created", name)
	}
	if got := histogram.sum(); got != want {
		t.Fatalf("histogram %q=%d, want %d", name, got, want)
	}
}

type recordingCounter struct {
	noop.Int64Counter
	mu    sync.Mutex
	total int64
}

func (c *recordingCounter) Add(_ context.Context, value int64, _ ...metric.AddOption) {
	c.mu.Lock()
	c.total += value
	c.mu.Unlock()
}

func (c *recordingCounter) sum() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.total
}

type recordingUpDownCounter struct {
	noop.Int64UpDownCounter
	mu    sync.Mutex
	total int64
}

func (c *recordingUpDownCounter) Add(_ context.Context, value int64, _ ...metric.AddOption) {
	c.mu.Lock()
	c.total += value
	c.mu.Unlock()
}

func (c *recordingUpDownCounter) sum() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.total
}

type recordingHistogram struct {
	noop.Int64Histogram
	mu    sync.Mutex
	total int64
}

func (h *recordingHistogram) Record(_ context.Context, value int64, _ ...metric.RecordOption) {
	h.mu.Lock()
	h.total += value
	h.mu.Unlock()
}

func (h *recordingHistogram) sum() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.total
}

type failingMeter struct {
	noop.Meter
	err error
}

func (m failingMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return nil, m.err
}
