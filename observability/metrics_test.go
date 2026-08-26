package observability

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport"
)

func TestAtomicChannelRecorderAggregatesChannelMetrics(t *testing.T) {
	recorder := NewAtomicChannelRecorder()
	id := transport.ChannelID(7)

	recorder.RecordChannelRegistered(id)
	recorder.RecordChannelActive(id)
	recorder.RecordChannelRead(id, 5)
	recorder.RecordChannelRead(id, -1)
	recorder.RecordChannelReadComplete(id)
	recorder.RecordChannelWrite(id, 3)
	recorder.RecordChannelReadDuration(id, 2*time.Nanosecond)
	recorder.RecordChannelReadDuration(id, time.Nanosecond)
	recorder.RecordChannelWriteDuration(id, 3*time.Nanosecond)
	recorder.RecordChannelFlushDuration(id, 4*time.Nanosecond)
	recorder.RecordChannelCloseDuration(id, 5*time.Nanosecond)
	recorder.RecordChannelFlush(id)
	recorder.RecordException(id, errors.New("boom"))
	recorder.RecordChannelClose(id)
	recorder.RecordChannelInactive(id)
	recorder.RecordChannelUnregistered(id)

	snapshot := recorder.Snapshot()
	if snapshot.RegisteredChannels != 1 || snapshot.UnregisteredChannels != 1 {
		t.Fatalf("registered=%d unregistered=%d", snapshot.RegisteredChannels, snapshot.UnregisteredChannels)
	}
	if snapshot.ActiveTransitions != 1 || snapshot.InactiveTransitions != 1 || snapshot.ActiveChannels != 0 {
		t.Fatalf("active transitions=%d inactive transitions=%d active=%d", snapshot.ActiveTransitions, snapshot.InactiveTransitions, snapshot.ActiveChannels)
	}
	if snapshot.InboundMessages != 2 || snapshot.InboundBytes != 5 || snapshot.InboundCompletions != 1 {
		t.Fatalf("inbound messages=%d bytes=%d completions=%d", snapshot.InboundMessages, snapshot.InboundBytes, snapshot.InboundCompletions)
	}
	if snapshot.OutboundMessages != 1 || snapshot.OutboundBytes != 3 || snapshot.Flushes != 1 || snapshot.Closes != 1 || snapshot.Exceptions != 1 {
		t.Fatalf("outbound=%+v", snapshot)
	}
	if snapshot.InboundReadNanos != 3 || snapshot.MaxInboundReadNanos != 2 {
		t.Fatalf("read latency=%+v", snapshot)
	}
	if snapshot.OutboundWriteNanos != 3 || snapshot.FlushNanos != 4 || snapshot.CloseNanos != 5 {
		t.Fatalf("operation latency=%+v", snapshot)
	}
}

func TestReadableBytesSizerUsesByteBufContract(t *testing.T) {
	buf := buffer.NewHeapBuffer(8)
	defer buf.Release()
	if _, err := buf.WriteBytes([]byte("abcd")); err != nil {
		t.Fatal(err)
	}
	if got := ReadableBytesSizer.MessageSize(buf); got != 4 {
		t.Fatalf("size=%d, want 4", got)
	}
	if got := ReadableBytesSizer.MessageSize("not-sized"); got != 0 {
		t.Fatalf("size=%d, want 0", got)
	}
}

func TestNormalizeObservabilityDefaults(t *testing.T) {
	if NormalizeMessageSizer(nil) == nil {
		t.Fatal("nil sizer should normalize to default")
	}
	if NormalizeChannelRecorder(nil) == nil {
		t.Fatal("nil recorder should normalize to noop")
	}
}

func TestPrometheusExporterWritesTextFormat(t *testing.T) {
	recorder := NewAtomicChannelRecorder()
	recorder.RecordChannelActive(1)
	recorder.RecordChannelRead(1, 128)
	recorder.RecordChannelWriteDuration(1, 3*time.Nanosecond)

	var out bytes.Buffer
	exporter := NewPrometheusExporter(PrometheusConfig{Prefix: "gnalloy-test"})
	if err := ExportSnapshot(&out, recorder, exporter); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{
		"# TYPE gnalloy_test_active gauge",
		"gnalloy_test_active 1",
		"gnalloy_test_inbound_bytes_total 128",
		"gnalloy_test_outbound_write_duration_nanoseconds_total 3",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
}

func TestAtomicChannelRecorderConcurrentUpdates(t *testing.T) {
	recorder := NewAtomicChannelRecorder()
	const workers = 8
	const iterations = 1000

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(id transport.ChannelID) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				recorder.RecordChannelActive(id)
				recorder.RecordChannelRead(id, 64)
				recorder.RecordChannelWrite(id, 32)
				recorder.RecordException(id, nil)
				recorder.RecordChannelInactive(id)
			}
		}(transport.ChannelID(i + 1))
	}
	wg.Wait()

	snapshot := recorder.Snapshot()
	want := uint64(workers * iterations)
	if snapshot.ActiveTransitions != want || snapshot.InactiveTransitions != want || snapshot.ActiveChannels != 0 {
		t.Fatalf("lifecycle=%+v want transitions=%d active=0", snapshot, want)
	}
	if snapshot.InboundMessages != want || snapshot.OutboundMessages != want || snapshot.Exceptions != want {
		t.Fatalf("counters=%+v want=%d", snapshot, want)
	}
	if snapshot.InboundBytes != want*64 || snapshot.OutboundBytes != want*32 {
		t.Fatalf("bytes=%+v want inbound=%d outbound=%d", snapshot, want*64, want*32)
	}
}

func BenchmarkAtomicChannelRecorderRead(b *testing.B) {
	recorder := NewAtomicChannelRecorder()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		recorder.RecordChannelRead(1, 64)
	}
}
