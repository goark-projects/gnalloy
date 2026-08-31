package webtransport

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	h3transport "goark.dev/gnalloy/transport/http3"
	"goark.dev/gnalloy/transport/quic"
)

func TestNewSessionRequiresNegotiatedDatagrams(t *testing.T) {
	conn := newFakeConnection()
	conn.state.SupportsDatagrams = quic.FeatureSupport{Local: true, Remote: false}
	connect := openHTTP3ConnectStream(t, conn)

	_, err := NewSession(conn, connect, Config{})
	if !errors.Is(err, ErrUnsupportedDatagram) {
		t.Fatalf("err=%v, want ErrUnsupportedDatagram", err)
	}
}

func TestNewSessionRequiresClientInitiatedBidirectionalConnectStream(t *testing.T) {
	conn := newFakeConnection()
	conn.openBidi[0].fakeSendStream.id = 1
	connect := openHTTP3ConnectStream(t, conn)

	_, err := NewSession(conn, connect, Config{DisableCapabilityValidation: true})
	if !errors.Is(err, ErrInvalidSessionID) {
		t.Fatalf("err=%v, want ErrInvalidSessionID", err)
	}
}

func TestOpenBidirectionalStreamWritesWebTransportPrefix(t *testing.T) {
	conn := newFakeConnection()
	connect := openHTTP3ConnectStream(t, conn)
	session := newSessionForTest(t, conn, connect)

	stream, err := session.OpenBidirectionalStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	payload := testBuffer("hello")
	if err := stream.Channel().Write(payload); err != nil {
		t.Fatal(err)
	}

	got := conn.openBidi[1].written.Bytes()
	want := []byte{0x40, 0x41, 0x00, 'h', 'e', 'l', 'l', 'o'}
	if !bytes.Equal(got, want) {
		t.Fatalf("written=%v, want %v", got, want)
	}
}

func TestAcceptBidirectionalStreamStripsWebTransportPrefix(t *testing.T) {
	conn := newFakeConnection()
	connect := openHTTP3ConnectStream(t, conn)
	session := newSessionForTest(t, conn, connect)
	conn.acceptBidi[0].feed([]byte{0x40, 0x41, 0x00, 'h', 'e', 'l', 'l', 'o'})

	stream, err := session.AcceptBidirectionalStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	capture := &captureInbound{}
	if err := stream.Channel().Pipeline().AddLast("capture", capture); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.ReadOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(capture.payloads) != 1 || string(capture.payloads[0]) != "hello" {
		t.Fatalf("payloads=%q", capture.payloads)
	}
}

func TestOpenUnidirectionalStreamWritesWebTransportStreamType(t *testing.T) {
	conn := newFakeConnection()
	connect := openHTTP3ConnectStream(t, conn)
	session := newSessionForTest(t, conn, connect)

	stream, err := session.OpenUnidirectionalStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	payload := testBuffer("uni")
	if err := stream.Channel().Write(payload); err != nil {
		t.Fatal(err)
	}

	got := conn.openUni[0].written.Bytes()
	want := []byte{0x40, 0x54, 0x00, 'u', 'n', 'i'}
	if !bytes.Equal(got, want) {
		t.Fatalf("written=%v, want %v", got, want)
	}
}

func TestSessionDatagramRoundTripUsesQuarterStreamID(t *testing.T) {
	conn := newFakeConnection()
	conn.openBidi[0].fakeSendStream.id = 4
	connect := openHTTP3ConnectStream(t, conn)
	session := newSessionForTest(t, conn, connect)

	if err := session.SendDatagram([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	if len(conn.sentDatagrams) != 1 || !bytes.Equal(conn.sentDatagrams[0], []byte{0x01, 'o', 'k'}) {
		t.Fatalf("sent datagrams=%v", conn.sentDatagrams)
	}

	conn.recvDatagrams = [][]byte{{0x02, 'n', 'o'}, {0x01, 'o', 'k'}}
	datagram, err := session.ReceiveDatagram(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if datagram.SessionID != 4 || datagram.QuarterStreamID != 1 || string(datagram.Payload) != "ok" {
		t.Fatalf("datagram=%+v", datagram)
	}
}

func TestSessionDatagramPayloadLimit(t *testing.T) {
	conn := newFakeConnection()
	conn.openBidi[0].fakeSendStream.id = 4
	connect := openHTTP3ConnectStream(t, conn)
	session, err := NewSession(conn, connect, Config{MaxDatagramPayload: 1})
	if err != nil {
		t.Fatal(err)
	}

	if err := session.SendDatagram([]byte("ok")); !errors.Is(err, ErrDatagramTooLarge) {
		t.Fatalf("send err=%v, want ErrDatagramTooLarge", err)
	}
	conn.recvDatagrams = [][]byte{{0x01, 'o', 'k'}}
	if _, err := session.ReceiveDatagram(context.Background()); !errors.Is(err, ErrDatagramTooLarge) {
		t.Fatalf("receive err=%v, want ErrDatagramTooLarge", err)
	}
}

func openHTTP3ConnectStream(t *testing.T, conn *fakeConnection) *h3transport.StreamChannel {
	t.Helper()
	h3, err := h3transport.NewSession(conn, h3transport.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connect, err := h3.OpenRequestStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if connect.Kind() != h3transport.StreamKindRequest {
		t.Fatalf("kind=%v, want request", connect.Kind())
	}
	return connect
}

func newSessionForTest(t *testing.T, conn *fakeConnection, connect *h3transport.StreamChannel) *Session {
	t.Helper()
	session, err := NewSession(conn, connect, Config{})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func testBuffer(data string) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes([]byte(data))
	return buf
}

type captureInbound struct {
	payloads [][]byte
}

func (c *captureInbound) ChannelRead(_ *channel.HandlerContext, msg any) {
	if buf, ok := msg.(buffer.ByteBuf); ok {
		c.payloads = append(c.payloads, append([]byte(nil), buf.Bytes()...))
		buf.Release()
	}
}

type fakeConnection struct {
	state          quic.State
	openBidi       []*fakeBidiStream
	openBidiNext   int
	acceptBidi     []*fakeBidiStream
	acceptBidiNext int
	openUni        []*fakeSendStream
	openUniNext    int
	acceptUni      []*fakeReceiveStream
	acceptUniNext  int
	sentDatagrams  [][]byte
	recvDatagrams  [][]byte
}

func newFakeConnection() *fakeConnection {
	state := quic.State{
		TLS:                                rfc9000TestTLSState(),
		SupportsDatagrams:                  quic.FeatureSupport{Local: true, Remote: true},
		SupportsStreamResetPartialDelivery: quic.FeatureSupport{Local: true, Remote: true},
	}
	return &fakeConnection{
		state:      state,
		openBidi:   []*fakeBidiStream{newFakeBidiStream(0), newFakeBidiStream(4)},
		acceptBidi: []*fakeBidiStream{newFakeBidiStream(8)},
		openUni:    []*fakeSendStream{newFakeSendStream(2)},
		acceptUni:  []*fakeReceiveStream{newFakeReceiveStream(6)},
	}
}

func rfc9000TestTLSState() tls.ConnectionState {
	return tls.ConnectionState{Version: tls.VersionTLS13, NegotiatedProtocol: "h3"}
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

func (c *fakeConnection) ConnectionState() quic.State {
	return c.state
}

func (c *fakeConnection) Stats() quic.ConnectionStats {
	return quic.ConnectionStats{}
}

func (c *fakeConnection) OpenStreamSync(context.Context) (quic.Stream, error) {
	if c.openBidiNext >= len(c.openBidi) {
		return nil, io.EOF
	}
	stream := c.openBidi[c.openBidiNext]
	c.openBidiNext++
	return stream, nil
}

func (c *fakeConnection) AcceptStream(context.Context) (quic.Stream, error) {
	if c.acceptBidiNext >= len(c.acceptBidi) {
		return nil, io.EOF
	}
	stream := c.acceptBidi[c.acceptBidiNext]
	c.acceptBidiNext++
	return stream, nil
}

func (c *fakeConnection) OpenUniStreamSync(context.Context) (quic.SendStream, error) {
	if c.openUniNext >= len(c.openUni) {
		return nil, io.EOF
	}
	stream := c.openUni[c.openUniNext]
	c.openUniNext++
	return stream, nil
}

func (c *fakeConnection) AcceptUniStream(context.Context) (quic.ReceiveStream, error) {
	if c.acceptUniNext >= len(c.acceptUni) {
		return nil, io.EOF
	}
	stream := c.acceptUni[c.acceptUniNext]
	c.acceptUniNext++
	return stream, nil
}

func (c *fakeConnection) SendDatagram(payload []byte) error {
	c.sentDatagrams = append(c.sentDatagrams, append([]byte(nil), payload...))
	return nil
}

func (c *fakeConnection) ReceiveDatagram(context.Context) ([]byte, error) {
	if len(c.recvDatagrams) == 0 {
		return nil, io.EOF
	}
	payload := c.recvDatagrams[0]
	c.recvDatagrams = c.recvDatagrams[1:]
	return payload, nil
}

func (c *fakeConnection) CloseWithError(quic.ApplicationErrorCode, string) error {
	return nil
}

type fakeAddr string

func (a fakeAddr) Network() string { return "fake" }
func (a fakeAddr) String() string  { return string(a) }

type fakeBidiStream struct {
	*fakeReceiveStream
	*fakeSendStream
}

func newFakeBidiStream(id quic.StreamID) *fakeBidiStream {
	return &fakeBidiStream{fakeReceiveStream: newFakeReceiveStream(id), fakeSendStream: newFakeSendStream(id)}
}

func (s *fakeBidiStream) ID() quic.StreamID {
	return s.fakeSendStream.ID()
}

func (s *fakeBidiStream) SetDeadline(time.Time) error {
	return nil
}

type fakeSendStream struct {
	id      quic.StreamID
	written bytes.Buffer
	closed  bool
}

func newFakeSendStream(id quic.StreamID) *fakeSendStream {
	return &fakeSendStream{id: id}
}

func (s *fakeSendStream) ID() quic.StreamID {
	return s.id
}

func (s *fakeSendStream) Write(p []byte) (int, error) {
	if s.closed {
		return 0, quic.ErrClosed
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

func (s *fakeSendStream) CancelWrite(quic.StreamErrorCode) {
	s.closed = true
}

type fakeReceiveStream struct {
	id       quic.StreamID
	readable bytes.Buffer
	canceled bool
}

func newFakeReceiveStream(id quic.StreamID) *fakeReceiveStream {
	return &fakeReceiveStream{id: id}
}

func (s *fakeReceiveStream) ID() quic.StreamID {
	return s.id
}

func (s *fakeReceiveStream) Read(p []byte) (int, error) {
	if s.readable.Len() == 0 {
		return 0, io.EOF
	}
	return s.readable.Read(p)
}

func (s *fakeReceiveStream) SetReadDeadline(time.Time) error {
	return nil
}

func (s *fakeReceiveStream) CancelRead(quic.StreamErrorCode) {
	s.canceled = true
}

func (s *fakeReceiveStream) feed(data []byte) {
	_, _ = s.readable.Write(data)
}

var _ quic.Connection = (*fakeConnection)(nil)
var _ quic.Stream = (*fakeBidiStream)(nil)
var _ quic.SendStream = (*fakeSendStream)(nil)
var _ quic.ReceiveStream = (*fakeReceiveStream)(nil)
