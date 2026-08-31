package quic

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"testing"
	"time"

	nativequic "github.com/quic-go/quic-go"
)

func TestStatsFromNativeCopiesConnectionStats(t *testing.T) {
	native := nativequic.ConnectionStats{
		MinRTT:          time.Millisecond,
		LatestRTT:       2 * time.Millisecond,
		SmoothedRTT:     3 * time.Millisecond,
		MeanDeviation:   4 * time.Millisecond,
		BytesSent:       10,
		PacketsSent:     11,
		BytesReceived:   12,
		PacketsReceived: 13,
		BytesLost:       14,
		PacketsLost:     15,
	}

	stats := statsFromNative(native)
	if stats.MinRTT != native.MinRTT ||
		stats.LatestRTT != native.LatestRTT ||
		stats.SmoothedRTT != native.SmoothedRTT ||
		stats.MeanDeviation != native.MeanDeviation ||
		stats.BytesSent != native.BytesSent ||
		stats.PacketsSent != native.PacketsSent ||
		stats.BytesReceived != native.BytesReceived ||
		stats.PacketsReceived != native.PacketsReceived ||
		stats.BytesLost != native.BytesLost ||
		stats.PacketsLost != native.PacketsLost {
		t.Fatalf("stats=%+v native=%+v", stats, native)
	}
}

func TestNormalizeConfigInstallsQLogTracer(t *testing.T) {
	var captured QLogTraceInfo
	var called bool
	cfg := Config{
		TLS: &tls.Config{},
		QLog: QLogConfig{
			WriterFactory: QLogWriterFactoryFunc(func(ctx context.Context, info QLogTraceInfo) (io.WriteCloser, error) {
				if ctx == nil {
					t.Fatal("qlog context must be normalized")
				}
				captured = info
				called = true
				return nopWriteCloser{Writer: &bytes.Buffer{}}, nil
			}),
			EventSchemas: []string{"urn:gnalloy:test"},
		},
	}
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.quic.Tracer == nil {
		t.Fatal("missing native qlog tracer")
	}

	trace := normalized.quic.Tracer(nil, true, nativequic.ConnectionIDFromBytes([]byte{1, 2, 3}))
	if trace == nil {
		t.Fatal("missing qlog trace")
	}
	producer := trace.AddProducer()
	if producer == nil {
		t.Fatal("missing qlog producer")
	}
	if err := producer.Close(); err != nil {
		t.Fatal(err)
	}
	if !called || !captured.Client || !bytes.Equal(captured.OriginalDestinationConnectionID, []byte{1, 2, 3}) {
		t.Fatalf("captured=%+v called=%v", captured, called)
	}
}

func TestQLogTracerSkipsFactoryErrors(t *testing.T) {
	cfg := Config{
		TLS: &tls.Config{},
		QLog: QLogConfig{
			WriterFactory: QLogWriterFactoryFunc(func(context.Context, QLogTraceInfo) (io.WriteCloser, error) {
				return nil, errors.New("open qlog")
			}),
		},
	}
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	trace := normalized.quic.Tracer(context.Background(), false, nativequic.ConnectionIDFromBytes([]byte{9}))
	if trace != nil {
		t.Fatal("factory error must skip qlog trace")
	}
}

func TestQLogFactoryFuncNilSkipsTrace(t *testing.T) {
	cfg := Config{
		TLS:  &tls.Config{},
		QLog: QLogConfig{WriterFactory: QLogWriterFactoryFunc(nil)},
	}
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.quic.Tracer != nil {
		t.Fatal("nil function factory must not install qlog tracer")
	}
}

func TestDetectNativeSupportReportsRFC9000Boundary(t *testing.T) {
	support := DetectNativeSupport()
	if support.Engine != NativeEngineQUICGo {
		t.Fatalf("engine=%s", support.Engine)
	}
	if !support.RFC9000 || !support.TLS13Only || !support.ConnectionStats || !support.QLog {
		t.Fatalf("support=%+v", support)
	}
	if !support.Datagrams || !support.StreamResetPartialDelivery || !support.ZeroRTT {
		t.Fatalf("extension support=%+v", support)
	}
}

type nopWriteCloser struct {
	io.Writer
}

func (w nopWriteCloser) Close() error {
	return nil
}
