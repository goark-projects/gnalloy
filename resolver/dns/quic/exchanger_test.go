package quic

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"testing"
	"time"

	dnscodec "goark.dev/gnalloy/codec/dns"
	"goark.dev/gnalloy/transport/quic"
	"goark.dev/gnalloy/transport/quic/application"
)

func TestExchangerUsesDoQALPNAndLengthPrefixedStream(t *testing.T) {
	answer, err := dnscodec.NewAResource("example.com", 60, net.IPv4(1, 2, 3, 4))
	if err != nil {
		t.Fatal(err)
	}
	response := dnscodec.Message{
		ID:                 7,
		Response:           true,
		RecursionAvailable: true,
		Questions: []dnscodec.Question{{
			Name:  "example.com",
			Type:  dnscodec.TypeA,
			Class: dnscodec.ClassIN,
		}},
		Answers: []dnscodec.Resource{answer},
	}
	responseWire, err := dnscodec.AppendMessage(nil, response)
	if err != nil {
		t.Fatal(err)
	}
	stream := newFakeDoQStream(frame(t, responseWire))
	dialer := &fakeDoQDialer{conn: &fakeDoQConnection{stream: stream}}
	exchanger := Exchanger{
		Dialer: dialer,
		Config: quic.Config{TLS: &tls.Config{InsecureSkipVerify: true}},
	}
	query := dnscodec.NewQuery(7, "example.com", dnscodec.TypeA)
	reply, err := exchanger.Exchange(context.Background(), "dns.example.com", query)
	if err != nil {
		t.Fatal(err)
	}
	if !reply.Response || len(reply.Answers) != 1 || !reply.Answers[0].IP().Equal(net.IPv4(1, 2, 3, 4)) {
		t.Fatalf("reply=%+v", reply)
	}
	if dialer.addr != "dns.example.com:853" {
		t.Fatalf("addr=%q, want dns.example.com:853", dialer.addr)
	}
	if len(dialer.cfg.NextProtos) != 1 || dialer.cfg.NextProtos[0] != DefaultALPN {
		t.Fatalf("alpn=%v, want %q", dialer.cfg.NextProtos, DefaultALPN)
	}
	queryWire, err := dnscodec.AppendMessage(nil, query)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stream.written.Bytes(), frame(t, queryWire)) {
		t.Fatalf("query frame=%v", stream.written.Bytes())
	}
}

func frame(t *testing.T, payload []byte) []byte {
	t.Helper()
	var wire bytes.Buffer
	if err := (application.LengthPrefixedCodec{}).WriteFrame(&wire, payload); err != nil {
		t.Fatal(err)
	}
	return wire.Bytes()
}

type fakeDoQDialer struct {
	conn *fakeDoQConnection
	addr string
	cfg  quic.Config
}

func (d *fakeDoQDialer) DialAddr(_ context.Context, addr string, cfg quic.Config) (quic.Connection, error) {
	d.addr = addr
	d.cfg = cfg
	return d.conn, nil
}

type fakeDoQConnection struct {
	stream *fakeDoQStream
	closed bool
}

func (c *fakeDoQConnection) LocalAddr() net.Addr  { return fakeAddr("local") }
func (c *fakeDoQConnection) RemoteAddr() net.Addr { return fakeAddr("remote") }
func (c *fakeDoQConnection) HandshakeComplete() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}
func (c *fakeDoQConnection) ConnectionState() quic.State { return quic.State{} }
func (c *fakeDoQConnection) Stats() quic.ConnectionStats { return quic.ConnectionStats{} }
func (c *fakeDoQConnection) OpenStreamSync(context.Context) (quic.Stream, error) {
	return c.stream, nil
}
func (c *fakeDoQConnection) AcceptStream(context.Context) (quic.Stream, error) {
	return nil, io.EOF
}
func (c *fakeDoQConnection) OpenUniStreamSync(context.Context) (quic.SendStream, error) {
	return nil, io.EOF
}
func (c *fakeDoQConnection) AcceptUniStream(context.Context) (quic.ReceiveStream, error) {
	return nil, io.EOF
}
func (c *fakeDoQConnection) SendDatagram([]byte) error {
	return io.EOF
}
func (c *fakeDoQConnection) ReceiveDatagram(context.Context) ([]byte, error) {
	return nil, io.EOF
}
func (c *fakeDoQConnection) CloseWithError(quic.ApplicationErrorCode, string) error {
	c.closed = true
	return nil
}

type fakeDoQStream struct {
	read    *bytes.Reader
	written bytes.Buffer
	closed  bool
}

func newFakeDoQStream(read []byte) *fakeDoQStream {
	return &fakeDoQStream{read: bytes.NewReader(read)}
}

func (s *fakeDoQStream) ID() quic.StreamID { return 1 }
func (s *fakeDoQStream) Read(p []byte) (int, error) {
	return s.read.Read(p)
}
func (s *fakeDoQStream) Write(p []byte) (int, error) {
	return s.written.Write(p)
}
func (s *fakeDoQStream) Close() error {
	s.closed = true
	return nil
}
func (s *fakeDoQStream) SetDeadline(time.Time) error      { return nil }
func (s *fakeDoQStream) SetReadDeadline(time.Time) error  { return nil }
func (s *fakeDoQStream) SetWriteDeadline(time.Time) error { return nil }
func (s *fakeDoQStream) CancelRead(quic.StreamErrorCode) {
	s.closed = true
}
func (s *fakeDoQStream) CancelWrite(quic.StreamErrorCode) {
	s.closed = true
}

type fakeAddr string

func (a fakeAddr) Network() string { return "fake" }
func (a fakeAddr) String() string  { return string(a) }
