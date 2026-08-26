package http2

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestFrameDecoderDecodesPayloadZeroCopy(t *testing.T) {
	decoder, err := NewFrameDecoder(DefaultMaxFrameSize)
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("h2", decoder); err != nil {
		t.Fatal(err)
	}
	recorder := &frameRecorder{}
	if err := ch.Pipeline().AddLast("recorder", recorder); err != nil {
		t.Fatal(err)
	}

	in, err := ch.Allocator().Acquire(FrameHeaderSize + 4)
	if err != nil {
		t.Fatal(err)
	}
	appendRawFrame(t, in, FrameHeader{Length: 4, Type: FrameData, Flags: FlagEndStream, StreamID: 1}, []byte("ping"))
	ch.Pipeline().FireChannelRead(in)

	frame, ok := recorder.msg.(Frame)
	if !ok {
		t.Fatalf("msg=%T, want Frame", recorder.msg)
	}
	defer frame.Release()
	if frame.Type != FrameData || frame.Flags != FlagEndStream || frame.StreamID != 1 {
		t.Fatalf("frame=%+v", frame)
	}
	if string(frame.Payload.Bytes()) != "ping" {
		t.Fatalf("payload=%q, want ping", frame.Payload.Bytes())
	}
}

func TestFrameEncoderWritesHeaderThenPayload(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("h2", NewFrameEncoder()); err != nil {
		t.Fatal(err)
	}
	payload, err := ch.Allocator().Acquire(4)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = payload.WriteBytes([]byte("pong"))
	if err := ch.Write(Frame{Type: FrameData, Flags: FlagEndStream, StreamID: 3, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if len(sink.messages) != 2 {
		t.Fatalf("writes=%d, want header and payload", len(sink.messages))
	}
	header := sink.messages[0].(buffer.ByteBuf)
	defer header.Release()
	body := sink.messages[1].(buffer.ByteBuf)
	defer body.Release()
	decoded, err := ParseFrameHeader(header.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Length != 4 || decoded.Type != FrameData || decoded.Flags != FlagEndStream || decoded.StreamID != 3 {
		t.Fatalf("header=%+v", decoded)
	}
	if string(body.Bytes()) != "pong" {
		t.Fatalf("body=%q, want pong", body.Bytes())
	}
}

func TestSettingsAckHasNoPayload(t *testing.T) {
	frame := SettingsAck()
	if frame.Type != FrameSettings || frame.Flags != FlagAck || frame.StreamID != 0 || frame.Payload != nil {
		t.Fatalf("settings ack=%+v", frame)
	}
}

func TestTypedFrameDecoderAcceptsEmptyPayloadFrames(t *testing.T) {
	settings, err := DecodeTypedFrame(Frame{Type: FrameSettings})
	if err != nil {
		t.Fatal(err)
	}
	settings.Release()
	continuation, err := DecodeTypedFrame(Frame{Type: FrameContinuation, StreamID: 1, Flags: FlagEndHeaders})
	if err != nil {
		t.Fatal(err)
	}
	defer continuation.Release()
	frame := continuation.(ContinuationFrame)
	if frame.StreamID != 1 || frame.HeaderBlock != nil {
		t.Fatalf("continuation=%+v", frame)
	}
}

func TestTypedFrameDecoderParsesHeadersWithPriority(t *testing.T) {
	decoder, err := NewFrameDecoder(DefaultMaxFrameSize)
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("h2", decoder); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("typed", NewTypedFrameDecoder()); err != nil {
		t.Fatal(err)
	}
	recorder := &frameRecorder{}
	if err := ch.Pipeline().AddLast("recorder", recorder); err != nil {
		t.Fatal(err)
	}

	in, err := ch.Allocator().Acquire(FrameHeaderSize + 8)
	if err != nil {
		t.Fatal(err)
	}
	appendRawFrame(t, in, FrameHeader{Length: 8, Type: FrameHeaders, Flags: FlagEndHeaders | FlagPriority, StreamID: 1}, []byte{0x80, 0, 0, 3, 16, 'h', 'd', 'r'})
	ch.Pipeline().FireChannelRead(in)

	frame, ok := recorder.msg.(HeadersFrame)
	if !ok {
		t.Fatalf("msg=%T, want HeadersFrame", recorder.msg)
	}
	defer frame.Release()
	if frame.StreamID != 1 || frame.Priority == nil || !frame.Priority.Exclusive || frame.Priority.StreamDependency != 3 || frame.Priority.Weight != 16 {
		t.Fatalf("headers=%+v priority=%+v", frame, frame.Priority)
	}
	if string(frame.HeaderBlock.Bytes()) != "hdr" {
		t.Fatalf("header block=%q", frame.HeaderBlock.Bytes())
	}
}

func TestTypedFrameDecoderRetainsUnknownFramePayload(t *testing.T) {
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("typed", NewTypedFrameDecoder()); err != nil {
		t.Fatal(err)
	}
	recorder := &frameRecorder{}
	if err := ch.Pipeline().AddLast("recorder", recorder); err != nil {
		t.Fatal(err)
	}

	payload := buffer.NewHeapBuffer(3)
	_, _ = payload.WriteBytes([]byte("raw"))
	ch.Pipeline().FireChannelRead(Frame{Type: FrameType(0xff), Payload: payload})

	frame, ok := recorder.msg.(UnknownFrame)
	if !ok {
		t.Fatalf("msg=%T, want UnknownFrame", recorder.msg)
	}
	if got := string(frame.Frame.Payload.Bytes()); got != "raw" {
		t.Fatalf("payload=%q, want raw", got)
	}
	frame.Release()
}

func TestTypedFrameDecoderReleasesInvalidFrameOnce(t *testing.T) {
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("typed", NewTypedFrameDecoder()); err != nil {
		t.Fatal(err)
	}

	payload := buffer.NewHeapBuffer(1)
	_, _ = payload.WriteBytes([]byte{0x00})
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		ch.Pipeline().FireChannelRead(Frame{Type: FrameSettings, StreamID: 1, Payload: payload})
	}()
	if panicked {
		t.Fatal("invalid typed frame should be released by the decoder template exactly once")
	}
	if refs := payload.RefCnt(); refs != 0 {
		t.Fatalf("payload refs=%d, want 0", refs)
	}
}

func TestTypedFrameEncoderWritesSettingsFrame(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("frame", NewFrameEncoder()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("typed", NewTypedFrameEncoder()); err != nil {
		t.Fatal(err)
	}

	err := ch.Write(SettingsFrame{Settings: []Setting{{ID: 1, Value: 4096}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.messages) != 2 {
		t.Fatalf("writes=%d, want header and payload", len(sink.messages))
	}
	header := sink.messages[0].(buffer.ByteBuf)
	defer header.Release()
	body := sink.messages[1].(buffer.ByteBuf)
	defer body.Release()
	decoded, err := ParseFrameHeader(header.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Length != 6 || decoded.Type != FrameSettings || decoded.StreamID != 0 {
		t.Fatalf("header=%+v", decoded)
	}
	if got := body.Bytes(); string(got) != string([]byte{0, 1, 0, 0, 0x10, 0}) {
		t.Fatalf("settings payload=%v", got)
	}
}

func TestTypedFrameEncoderKeepsDataPayloadZeroCopy(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("frame", NewFrameEncoder()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("typed", NewTypedFrameEncoder()); err != nil {
		t.Fatal(err)
	}

	body := buffer.NewHeapBuffer(4)
	_, _ = body.WriteBytes([]byte("data"))
	if err := ch.Write(DataFrame{StreamID: 1, Flags: FlagEndStream, Data: body}); err != nil {
		t.Fatal(err)
	}
	if len(sink.messages) != 2 {
		t.Fatalf("writes=%d, want header and payload", len(sink.messages))
	}
	defer sink.messages[0].(buffer.ByteBuf).Release()
	defer sink.messages[1].(buffer.ByteBuf).Release()
	if sink.messages[1] != body {
		t.Fatal("data payload should pass through without copy")
	}
}

func TestTypedFrameEncoderReleasesFrameOnWriteError(t *testing.T) {
	writeErr := errors.New("write failed")
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), failingSink{err: writeErr})
	if err := ch.Pipeline().AddLast("typed", NewTypedFrameEncoder()); err != nil {
		t.Fatal(err)
	}

	body := buffer.NewHeapBuffer(4)
	_, _ = body.WriteBytes([]byte("data"))
	err := ch.Write(DataFrame{StreamID: 1, Data: body})
	if !errors.Is(err, writeErr) {
		t.Fatalf("err=%v, want %v", err, writeErr)
	}
	if refs := body.RefCnt(); refs != 0 {
		body.Release()
		t.Fatalf("body refs=%d, want 0", refs)
	}
}

func TestStreamStateTransitions(t *testing.T) {
	s := NewStream(1)
	if !s.ID.ClientInitiated() || s.State != StreamIdle {
		t.Fatalf("stream=%+v", s)
	}
	if err := s.Open(false); err != nil {
		t.Fatal(err)
	}
	if err := s.HalfCloseRemote(); err != nil {
		t.Fatal(err)
	}
	if s.State != StreamHalfClosedRemote {
		t.Fatalf("state=%v, want half-closed remote", s.State)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Open(false); !errors.Is(err, ErrInvalidStreamState) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidStreamState)
	}
}

func appendRawFrame(t *testing.T, out buffer.ByteBuf, header FrameHeader, payload []byte) {
	t.Helper()
	raw, err := AppendFrameHeader(nil, header)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, payload...)
	if _, err := out.WriteBytes(raw); err != nil {
		t.Fatal(err)
	}
}

type captureSink struct {
	messages []any
}

func (s *captureSink) Write(msg any) error {
	s.messages = append(s.messages, msg)
	return nil
}

func (s *captureSink) Flush() error { return nil }
func (s *captureSink) Close() error { return nil }

type failingSink struct {
	err error
}

func (s failingSink) Write(any) error { return s.err }
func (s failingSink) Flush() error    { return nil }
func (s failingSink) Close() error    { return nil }

type frameRecorder struct {
	msg any
}

func (r *frameRecorder) ChannelRead(_ *channel.HandlerContext, msg any) {
	r.msg = msg
}
