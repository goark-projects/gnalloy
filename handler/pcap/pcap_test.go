package pcap

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/udp"
)

func TestHandlerWritesPCAPHeaderAndReadRecord(t *testing.T) {
	var out bytes.Buffer
	handler, err := NewHandler(Config{
		Writer:       &out,
		SnapLen:      4,
		CaptureRead:  true,
		CaptureWrite: false,
		Clock: func() time.Time {
			return time.Unix(10, 2000)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), &pcapSink{})
	recorder := &pcapRecorder{}
	if err := ch.Pipeline().AddLast("pcap", handler); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("recorder", recorder); err != nil {
		t.Fatal(err)
	}

	payload := pcapBuf(t, "abcdef")
	ch.Pipeline().FireChannelRead(payload)

	if len(recorder.messages) != 1 {
		t.Fatalf("messages=%d, want propagated read", len(recorder.messages))
	}
	data := out.Bytes()
	if len(data) != pcapHeaderSize+pcapRecordSize+4 {
		t.Fatalf("pcap bytes=%d, want header+record+snaplen", len(data))
	}
	if binary.LittleEndian.Uint32(data[0:4]) != pcapMagicNumber {
		t.Fatalf("magic=%x", data[0:4])
	}
	record := data[pcapHeaderSize : pcapHeaderSize+pcapRecordSize]
	if binary.LittleEndian.Uint32(record[0:4]) != 10 || binary.LittleEndian.Uint32(record[4:8]) != 2 {
		t.Fatalf("timestamp record=%v", record[:8])
	}
	if binary.LittleEndian.Uint32(record[8:12]) != 4 || binary.LittleEndian.Uint32(record[12:16]) != 6 {
		t.Fatalf("length record=%v", record[8:16])
	}
	if string(data[pcapHeaderSize+pcapRecordSize:]) != "abcd" {
		t.Fatalf("payload=%q, want abcd", data[pcapHeaderSize+pcapRecordSize:])
	}
	recorder.release()
}

func TestHandlerCapturesWriteAndPreservesOwnership(t *testing.T) {
	var out bytes.Buffer
	handler, err := NewHandler(Config{Writer: &out, CaptureWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	sink := &pcapSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("pcap", handler); err != nil {
		t.Fatal(err)
	}

	payload := pcapBuf(t, "udp")
	if err := ch.Write(udp.Datagram{Payload: payload, Addr: udp.Address{Port: 9000}}); err != nil {
		t.Fatal(err)
	}
	if payload.RefCnt() == 0 {
		t.Fatal("pcap handler must not release outbound message before sink")
	}
	if len(sink.messages) != 1 {
		t.Fatalf("messages=%d, want write propagated", len(sink.messages))
	}
	sink.release()
	if payload.RefCnt() != 0 {
		t.Fatalf("payload ref=%d, want sink release", payload.RefCnt())
	}
	if len(out.Bytes()) <= pcapHeaderSize+pcapRecordSize {
		t.Fatalf("pcap output too short: %d", len(out.Bytes()))
	}
}

type pcapRecorder struct {
	messages []any
}

func (r *pcapRecorder) ChannelRead(_ *channel.HandlerContext, msg any) {
	r.messages = append(r.messages, msg)
}

func (r *pcapRecorder) release() {
	for _, msg := range r.messages {
		if releasable, ok := msg.(interface{ Release() }); ok {
			releasable.Release()
		}
	}
	r.messages = nil
}

type pcapSink struct {
	messages []any
}

func (s *pcapSink) Write(msg any) error {
	s.messages = append(s.messages, msg)
	return nil
}

func (s *pcapSink) Flush() error { return nil }
func (s *pcapSink) Close() error { return nil }

func (s *pcapSink) release() {
	for _, msg := range s.messages {
		if releasable, ok := msg.(interface{ Release() }); ok {
			releasable.Release()
		}
	}
	s.messages = nil
}

func pcapBuf(t *testing.T, data string) buffer.ByteBuf {
	t.Helper()
	buf := buffer.NewHeapBuffer(len(data))
	if _, err := buf.WriteBytes([]byte(data)); err != nil {
		buf.Release()
		t.Fatal(err)
	}
	return buf
}
