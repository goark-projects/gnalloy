package application

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"testing"
	"time"

	"goark.dev/gnalloy/transport/quic/rfc9000"
)

func TestStreamExchangerUsesLengthPrefixedStream(t *testing.T) {
	response := encodeTestFrame(t, []byte("pong"))
	conn := newFakeConnection(newFakeStream(1, response))
	exchanger := StreamExchanger{
		Dialer: fakeDialer{conn: conn},
		Config: rfc9000.Config{
			TLS:        &tls.Config{InsecureSkipVerify: true},
			NextProtos: []string{"app-test"},
		},
		Codec: LengthPrefixedCodec{MaxFrameSize: 16},
	}
	got, err := exchanger.Exchange(context.Background(), "127.0.0.1:4433", []byte("ping"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pong" {
		t.Fatalf("response=%q, want pong", got)
	}
	if !bytes.Equal(conn.stream.written.Bytes(), encodeTestFrame(t, []byte("ping"))) {
		t.Fatalf("request frame=%v", conn.stream.written.Bytes())
	}
	if !conn.closed {
		t.Fatal("exchange must close QUIC connection")
	}
}

func TestDatagramExchangerFiltersUnmatchedPayloads(t *testing.T) {
	conn := newFakeConnection(nil)
	conn.datagrams = [][]byte{[]byte("noise"), []byte("pong")}
	exchanger := DatagramExchanger{
		Dialer: fakeDialer{conn: conn},
		Config: rfc9000.Config{
			TLS:             &tls.Config{InsecureSkipVerify: true},
			NextProtos:      []string{"app-dgram"},
			EnableDatagrams: true,
		},
		Match: func(_, response []byte) bool {
			return string(response) == "pong"
		},
	}
	got, err := exchanger.Exchange(context.Background(), "127.0.0.1:4433", []byte("ping"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pong" {
		t.Fatalf("datagram=%q, want pong", got)
	}
	if len(conn.sentDatagrams) != 1 || string(conn.sentDatagrams[0]) != "ping" {
		t.Fatalf("sent=%q", conn.sentDatagrams)
	}
}

func encodeTestFrame(t *testing.T, payload []byte) []byte {
	t.Helper()
	var wire bytes.Buffer
	if err := (LengthPrefixedCodec{}).WriteFrame(&wire, payload); err != nil {
		t.Fatal(err)
	}
	return wire.Bytes()
}

type fakeDialer struct {
	conn *fakeConnection
	cfg  rfc9000.Config
}

func (d fakeDialer) DialAddr(context.Context, string, rfc9000.Config) (rfc9000.Connection, error) {
	d.conn.dialed = true
	return d.conn, nil
}

type fakeConnection struct {
	stream        *fakeStream
	dialed        bool
	closed        bool
	datagrams     [][]byte
	sentDatagrams [][]byte
}

func newFakeConnection(stream *fakeStream) *fakeConnection {
	return &fakeConnection{stream: stream}
}

func (c *fakeConnection) LocalAddr() net.Addr  { return fakeAddr("local") }
func (c *fakeConnection) RemoteAddr() net.Addr { return fakeAddr("remote") }
func (c *fakeConnection) HandshakeComplete() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}
func (c *fakeConnection) ConnectionState() rfc9000.State { return rfc9000.State{} }
func (c *fakeConnection) OpenStreamSync(context.Context) (rfc9000.Stream, error) {
	return c.stream, nil
}
func (c *fakeConnection) AcceptStream(context.Context) (rfc9000.Stream, error) {
	return nil, io.EOF
}
func (c *fakeConnection) OpenUniStreamSync(context.Context) (rfc9000.SendStream, error) {
	return nil, io.EOF
}
func (c *fakeConnection) AcceptUniStream(context.Context) (rfc9000.ReceiveStream, error) {
	return nil, io.EOF
}
func (c *fakeConnection) SendDatagram(payload []byte) error {
	c.sentDatagrams = append(c.sentDatagrams, append([]byte(nil), payload...))
	return nil
}
func (c *fakeConnection) ReceiveDatagram(context.Context) ([]byte, error) {
	if len(c.datagrams) == 0 {
		return nil, io.EOF
	}
	out := c.datagrams[0]
	c.datagrams = c.datagrams[1:]
	return out, nil
}
func (c *fakeConnection) CloseWithError(rfc9000.ApplicationErrorCode, string) error {
	c.closed = true
	return nil
}

type fakeStream struct {
	id      rfc9000.StreamID
	read    *bytes.Reader
	written bytes.Buffer
	closed  bool
}

func newFakeStream(id rfc9000.StreamID, read []byte) *fakeStream {
	return &fakeStream{id: id, read: bytes.NewReader(read)}
}

func (s *fakeStream) ID() rfc9000.StreamID { return s.id }
func (s *fakeStream) Read(p []byte) (int, error) {
	return s.read.Read(p)
}
func (s *fakeStream) Write(p []byte) (int, error) {
	return s.written.Write(p)
}
func (s *fakeStream) Close() error {
	s.closed = true
	return nil
}
func (s *fakeStream) SetDeadline(time.Time) error      { return nil }
func (s *fakeStream) SetReadDeadline(time.Time) error  { return nil }
func (s *fakeStream) SetWriteDeadline(time.Time) error { return nil }
func (s *fakeStream) CancelRead(rfc9000.StreamErrorCode) {
	s.closed = true
}
func (s *fakeStream) CancelWrite(rfc9000.StreamErrorCode) {
	s.closed = true
}

type fakeAddr string

func (a fakeAddr) Network() string { return "fake" }
func (a fakeAddr) String() string  { return string(a) }
