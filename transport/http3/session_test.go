package http3

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"goark.dev/gnalloy/channel"
	codechttp3 "goark.dev/gnalloy/codec/http3"
	"goark.dev/gnalloy/transport/quic/rfc9000"
)

func TestSessionRejectsNilConnection(t *testing.T) {
	if _, err := NewSession(nil, Config{}); !errors.Is(err, ErrInvalidConnection) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidConnection)
	}
}

func TestSessionRejectsNonHTTP3ALPN(t *testing.T) {
	conn := newFakeConnection()
	conn.alpn = "gnalloy-quic"
	if _, err := NewSession(conn, Config{}); !errors.Is(err, ErrInvalidALPN) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidALPN)
	}
}

func TestSessionRejectsNonTLS13(t *testing.T) {
	conn := newFakeConnection()
	conn.tlsVersion = tls.VersionTLS12
	if _, err := NewSession(conn, Config{}); !errors.Is(err, ErrInvalidTLSState) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidTLSState)
	}
}

func TestSessionAcceptsConfiguredALPN(t *testing.T) {
	conn := newFakeConnection()
	conn.alpn = "h3-29"
	if _, err := NewSession(conn, Config{AllowedALPN: []string{"h3-29"}}); err != nil {
		t.Fatal(err)
	}
}

func TestSessionOpenRequestStreamInstallsPipelineAndWritesFrames(t *testing.T) {
	conn := newFakeConnection()
	session, err := NewSession(conn, Config{})
	if err != nil {
		t.Fatal(err)
	}
	streamCh, err := session.OpenRequestStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	wantNames := []string{
		codechttp3.HandlerNameHTTP3FrameDecoder,
		codechttp3.HandlerNameHTTP3HeaderDecoder,
		codechttp3.HandlerNameHTTP3FrameEncoder,
		codechttp3.HandlerNameHTTP3HeaderEncoder,
	}
	if got := streamCh.Channel().Pipeline().Names(); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("pipeline names=%v, want %v", got, wantNames)
	}

	if err := streamCh.Channel().Write(codechttp3.HeadersBlock{Fields: []codechttp3.HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":path", Value: "/items"},
	}}); err != nil {
		t.Fatal(err)
	}
	raw := append([]byte(nil), conn.openedBidi.written.Bytes()...)
	if len(raw) == 0 {
		t.Fatal("missing encoded HTTP/3 request bytes")
	}

	capture := &captureInbound{}
	if err := streamCh.Channel().Pipeline().AddLast("capture", capture); err != nil {
		t.Fatal(err)
	}
	conn.openedBidi.feed(raw)
	if _, err := streamCh.ReadOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(capture.messages) != 1 {
		t.Fatalf("messages=%d, want 1", len(capture.messages))
	}
	headers, ok := capture.messages[0].(codechttp3.HeadersBlock)
	if !ok || len(headers.Fields) != 2 || headers.Fields[1].Value != "/items" {
		t.Fatalf("headers=%+v", capture.messages[0])
	}
}

func TestSessionOpensLocalControlStreamAndWritesSettings(t *testing.T) {
	conn := newFakeConnection()
	session, err := NewSession(conn, Config{
		Pipeline: codechttp3.PipelineConfig{
			Settings: []codechttp3.Setting{{ID: 1, Value: 10}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.OpenLocalControlStream(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := conn.openedUni.written.Bytes()
	want := []byte{byte(codechttp3.StreamTypeControl), byte(codechttp3.FrameSettings), 2, 1, 10}
	if !bytes.Equal(got, want) {
		t.Fatalf("control bytes=%v, want %v", got, want)
	}
}

func TestSessionAcceptRemoteControlStreamReadsSettings(t *testing.T) {
	conn := newFakeConnection()
	conn.acceptedUni.feed([]byte{byte(codechttp3.StreamTypeControl), byte(codechttp3.FrameSettings), 2, 1, 10})
	session, err := NewSession(conn, Config{})
	if err != nil {
		t.Fatal(err)
	}
	streamCh, err := session.AcceptRemoteControlStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	capture := &captureInbound{}
	if err := streamCh.Channel().Pipeline().AddLast("capture", capture); err != nil {
		t.Fatal(err)
	}
	if _, err := streamCh.ReadOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(capture.messages) != 2 {
		t.Fatalf("messages=%d, want stream type and settings", len(capture.messages))
	}
	settings, ok := capture.messages[1].(codechttp3.SettingsFrame)
	if !ok || len(settings.Settings) != 1 || settings.Settings[0].Value != 10 {
		t.Fatalf("settings=%+v", capture.messages[1])
	}
}

func TestStreamChannelEOFDoesNotCancelRead(t *testing.T) {
	conn := newFakeConnection()
	session, err := NewSession(conn, Config{})
	if err != nil {
		t.Fatal(err)
	}
	streamCh, err := session.AcceptRequestStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := streamCh.ReadOnce(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("err=%v, want EOF", err)
	}
	if conn.acceptedBidi.fakeReceiveStream.canceled {
		t.Fatal("EOF must not cancel QUIC stream reading")
	}
}

func TestStreamChannelCloseCancelsReadAndClosesWriter(t *testing.T) {
	conn := newFakeConnection()
	session, err := NewSession(conn, Config{})
	if err != nil {
		t.Fatal(err)
	}
	streamCh, err := session.AcceptRequestStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if err := streamCh.Close(); err != nil {
		t.Fatal(err)
	}
	if !conn.acceptedBidi.fakeReceiveStream.canceled {
		t.Fatal("Close must cancel QUIC stream reading")
	}
	if !conn.acceptedBidi.fakeSendStream.closed {
		t.Fatal("Close must close QUIC stream writer")
	}
}

type captureInbound struct {
	messages []any
}

func (c *captureInbound) ChannelRead(_ *channel.HandlerContext, msg any) {
	c.messages = append(c.messages, msg)
}

type fakeConnection struct {
	openedBidi   *fakeBidiStream
	acceptedBidi *fakeBidiStream
	openedUni    *fakeSendStream
	acceptedUni  *fakeReceiveStream
	alpn         string
	tlsVersion   uint16
}

func newFakeConnection() *fakeConnection {
	return &fakeConnection{
		openedBidi:   newFakeBidiStream(1),
		acceptedBidi: newFakeBidiStream(5),
		openedUni:    newFakeSendStream(3),
		acceptedUni:  newFakeReceiveStream(7),
		alpn:         "h3",
		tlsVersion:   tls.VersionTLS13,
	}
}

func (c *fakeConnection) LocalAddr() net.Addr {
	return fakeAddr("local")
}

func (c *fakeConnection) RemoteAddr() net.Addr {
	return fakeAddr("remote")
}

func (c *fakeConnection) HandshakeComplete() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}

func (c *fakeConnection) ConnectionState() rfc9000.State {
	return rfc9000.State{TLS: tls.ConnectionState{Version: c.tlsVersion, NegotiatedProtocol: c.alpn}}
}

func (c *fakeConnection) OpenStreamSync(context.Context) (rfc9000.Stream, error) {
	return c.openedBidi, nil
}

func (c *fakeConnection) AcceptStream(context.Context) (rfc9000.Stream, error) {
	return c.acceptedBidi, nil
}

func (c *fakeConnection) OpenUniStreamSync(context.Context) (rfc9000.SendStream, error) {
	return c.openedUni, nil
}

func (c *fakeConnection) AcceptUniStream(context.Context) (rfc9000.ReceiveStream, error) {
	return c.acceptedUni, nil
}

func (c *fakeConnection) SendDatagram([]byte) error {
	return nil
}

func (c *fakeConnection) ReceiveDatagram(context.Context) ([]byte, error) {
	return nil, io.EOF
}

func (c *fakeConnection) CloseWithError(rfc9000.ApplicationErrorCode, string) error {
	return nil
}

type fakeAddr string

func (a fakeAddr) Network() string { return "fake" }
func (a fakeAddr) String() string  { return string(a) }

type fakeBidiStream struct {
	*fakeReceiveStream
	*fakeSendStream
}

func newFakeBidiStream(id rfc9000.StreamID) *fakeBidiStream {
	return &fakeBidiStream{fakeReceiveStream: newFakeReceiveStream(id), fakeSendStream: newFakeSendStream(id)}
}

func (s *fakeBidiStream) ID() rfc9000.StreamID {
	return s.fakeSendStream.ID()
}

func (s *fakeBidiStream) SetDeadline(time.Time) error {
	return nil
}

type fakeSendStream struct {
	id      rfc9000.StreamID
	written bytes.Buffer
	closed  bool
}

func newFakeSendStream(id rfc9000.StreamID) *fakeSendStream {
	return &fakeSendStream{id: id}
}

func (s *fakeSendStream) ID() rfc9000.StreamID {
	return s.id
}

func (s *fakeSendStream) Write(p []byte) (int, error) {
	if s.closed {
		return 0, rfc9000.ErrClosed
	}
	return s.written.Write(p)
}

func (s *fakeSendStream) Close() error {
	s.closed = true
	return nil
}

func (s *fakeSendStream) SetWriteDeadline(time.Time) error {
	return nil
}

func (s *fakeSendStream) CancelWrite(rfc9000.StreamErrorCode) {
	s.closed = true
}

type fakeReceiveStream struct {
	id       rfc9000.StreamID
	mu       sync.Mutex
	readable bytes.Buffer
	canceled bool
}

func newFakeReceiveStream(id rfc9000.StreamID) *fakeReceiveStream {
	return &fakeReceiveStream{id: id}
}

func (s *fakeReceiveStream) ID() rfc9000.StreamID {
	return s.id
}

func (s *fakeReceiveStream) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readable.Len() == 0 {
		return 0, io.EOF
	}
	return s.readable.Read(p)
}

func (s *fakeReceiveStream) SetReadDeadline(time.Time) error {
	return nil
}

func (s *fakeReceiveStream) CancelRead(rfc9000.StreamErrorCode) {
	s.canceled = true
}

func (s *fakeReceiveStream) feed(data []byte) {
	s.mu.Lock()
	_, _ = s.readable.Write(data)
	s.mu.Unlock()
}

var _ rfc9000.Connection = (*fakeConnection)(nil)
var _ rfc9000.Stream = (*fakeBidiStream)(nil)
var _ rfc9000.SendStream = (*fakeSendStream)(nil)
var _ rfc9000.ReceiveStream = (*fakeReceiveStream)(nil)
