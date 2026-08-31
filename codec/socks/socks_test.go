package socks

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestGreetingDecodeEncode(t *testing.T) {
	collector := &captureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("greeting", NewGreetingDecoder()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(testBuf([]byte{0x05, 0x02, 0x00, 0x02}))
	greeting := collector.msgs[0].(Greeting)
	if len(greeting.Methods) != 2 || greeting.Methods[1] != MethodUserPassword {
		t.Fatalf("greeting=%+v", greeting)
	}

	sink := &captureSink{}
	outCh := channel.NewLocalChannel(2, buffer.NewHeapAllocator(), sink)
	if err := outCh.Pipeline().AddLast("greeting", NewGreetingEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()
	if err := outCh.Write(Greeting{Methods: []byte{MethodNoAuth}}); err != nil {
		t.Fatal(err)
	}
	if got := sink.writes[0].(buffer.ByteBuf).Bytes(); string(got) != string([]byte{0x05, 0x01, 0x00}) {
		t.Fatalf("wire=%v", got)
	}
}

func TestCommandRequestDecodeEncodeDomain(t *testing.T) {
	collector := &captureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("cmd", NewCommandRequestDecoder()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	wire := append([]byte{0x05, 0x01, 0x00, 0x03, 11}, []byte("example.com")...)
	wire = append(wire, 0x01, 0xbb)
	ch.Pipeline().FireChannelRead(testBuf(wire))
	req := collector.msgs[0].(CommandRequest)
	if req.Command != CommandConnect || req.Address != "example.com:443" {
		t.Fatalf("req=%+v", req)
	}

	sink := &captureSink{}
	outCh := channel.NewLocalChannel(2, buffer.NewHeapAllocator(), sink)
	if err := outCh.Pipeline().AddLast("cmd", NewCommandRequestEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()
	if err := outCh.Write(CommandRequest{Command: CommandConnect, Address: "example.com:443"}); err != nil {
		t.Fatal(err)
	}
	if got := sink.writes[0].(buffer.ByteBuf).Bytes(); string(got) != string(wire) {
		t.Fatalf("wire=%v want=%v", got, wire)
	}
}

func TestCommandRequestHelpers(t *testing.T) {
	if req := NewConnectRequest("example.com:443"); req.Version != Version5 || req.Command != CommandConnect || req.Address != "example.com:443" {
		t.Fatalf("connect req=%+v", req)
	}
	if req := NewBindRequest("127.0.0.1:1080"); req.Version != Version5 || req.Command != CommandBind || req.Address != "127.0.0.1:1080" {
		t.Fatalf("bind req=%+v", req)
	}
	if req := NewUDPAssociateRequest("[::1]:1080"); req.Version != Version5 || req.Command != CommandUDPAssociate || req.Address != "[::1]:1080" {
		t.Fatalf("udp req=%+v", req)
	}
}

func TestUsernamePasswordAuthDecodeEncode(t *testing.T) {
	wire := []byte{0x01, 0x04, 'u', 's', 'e', 'r', 0x04, 'p', 'a', 's', 's'}
	collector := &captureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("auth", NewUsernamePasswordAuthRequestDecoder()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(testBuf(wire))
	req := collector.msgs[0].(UsernamePasswordAuthRequest)
	if req.Username != "user" || req.Password != "pass" {
		t.Fatalf("req=%+v", req)
	}

	sink := &captureSink{}
	outCh := channel.NewLocalChannel(2, buffer.NewHeapAllocator(), sink)
	if err := outCh.Pipeline().AddLast("auth", NewUsernamePasswordAuthRequestEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()
	if err := outCh.Write(req); err != nil {
		t.Fatal(err)
	}
	if got := sink.writes[0].(buffer.ByteBuf).Bytes(); string(got) != string(wire) {
		t.Fatalf("wire=%v want=%v", got, wire)
	}
}

func TestUsernamePasswordAuthResponseDecodeEncode(t *testing.T) {
	collector := &captureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("auth", NewUsernamePasswordAuthResponseDecoder()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(testBuf([]byte{0x01, 0x00}))
	resp := collector.msgs[0].(UsernamePasswordAuthResponse)
	if resp.Status != AuthStatusSuccess {
		t.Fatalf("resp=%+v", resp)
	}

	sink := &captureSink{}
	outCh := channel.NewLocalChannel(2, buffer.NewHeapAllocator(), sink)
	if err := outCh.Pipeline().AddLast("auth", NewUsernamePasswordAuthResponseEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()
	if err := outCh.Write(UsernamePasswordAuthResponse{Status: AuthStatusFailure}); err != nil {
		t.Fatal(err)
	}
	if got := sink.writes[0].(buffer.ByteBuf).Bytes(); string(got) != string([]byte{0x01, 0x01}) {
		t.Fatalf("wire=%v", got)
	}
}

func TestPrivateAuthResponseDecodeEncode(t *testing.T) {
	if !IsPrivateMethod(0x80) || !IsPrivateMethod(0xfe) || IsPrivateMethod(MethodUserPassword) || IsPrivateMethod(MethodNoAcceptable) {
		t.Fatal("private method range mismatch")
	}

	collector := &captureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("private-auth", NewPrivateAuthResponseDecoder()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(testBuf([]byte{0x01, 0xff}))
	resp := collector.msgs[0].(PrivateAuthResponse)
	if resp.Status != PrivateAuthStatusFailure {
		t.Fatalf("resp=%+v", resp)
	}

	sink := &captureSink{}
	outCh := channel.NewLocalChannel(2, buffer.NewHeapAllocator(), sink)
	if err := outCh.Pipeline().AddLast("private-auth", NewPrivateAuthResponseEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()
	if err := outCh.Write(PrivateAuthResponse{Status: PrivateAuthStatusSuccess}); err != nil {
		t.Fatal(err)
	}
	if got := sink.writes[0].(buffer.ByteBuf).Bytes(); string(got) != string([]byte{0x01, 0x00}) {
		t.Fatalf("wire=%v", got)
	}
}

func TestCommandReplyDecodeIPv4(t *testing.T) {
	collector := &captureInbound{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("reply", NewCommandReplyDecoder()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}

	ch.Pipeline().FireChannelRead(testBuf([]byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x1f, 0x90}))
	reply := collector.msgs[0].(CommandReply)
	if reply.Status != 0 || reply.Address != "127.0.0.1:8080" {
		t.Fatalf("reply=%+v", reply)
	}
}

func TestSOCKS4RequestEncodeAndReplyDecode(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("socks4", NewSOCKS4RequestEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()
	if err := ch.Write(SOCKS4Request{Command: CommandConnect, Address: "example.com:80", UserID: "u"}); err != nil {
		t.Fatal(err)
	}
	got := sink.writes[0].(buffer.ByteBuf).Bytes()
	want := append([]byte{0x04, 0x01, 0x00, 0x50, 0, 0, 0, 1, 'u', 0}, []byte("example.com")...)
	want = append(want, 0)
	if string(got) != string(want) {
		t.Fatalf("wire=%v want=%v", got, want)
	}

	collector := &captureInbound{}
	inCh := channel.NewLocalChannel(2, buffer.NewHeapAllocator(), nil)
	if err := inCh.Pipeline().AddLast("reply", NewSOCKS4ReplyDecoder()); err != nil {
		t.Fatal(err)
	}
	if err := inCh.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}
	inCh.Pipeline().FireChannelRead(testBuf([]byte{0, 0x5a, 0x00, 0x50, 127, 0, 0, 1}))
	reply := collector.msgs[0].(SOCKS4Reply)
	if reply.Status != 0x5a || reply.Address != "127.0.0.1:80" {
		t.Fatalf("reply=%+v", reply)
	}
}

type captureInbound struct {
	msgs []any
}

func (h *captureInbound) ChannelRead(_ *channel.HandlerContext, msg any) {
	h.msgs = append(h.msgs, msg)
}

type captureSink struct {
	writes []any
}

func (s *captureSink) Write(msg any) error {
	s.writes = append(s.writes, msg)
	return nil
}

func (s *captureSink) Flush() error {
	return nil
}

func (s *captureSink) Close() error {
	return nil
}

func (s *captureSink) release() {
	for _, msg := range s.writes {
		if releasable, ok := msg.(interface{ Release() }); ok {
			releasable.Release()
		}
	}
}

func testBuf(data []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(data))
	_, _ = buf.WriteBytes(data)
	return buf
}
