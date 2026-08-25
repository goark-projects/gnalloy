package haproxy

import (
	"bytes"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

func TestDecoderParsesV1Header(t *testing.T) {
	decoder, err := NewDecoder(0)
	if err != nil {
		t.Fatal(err)
	}
	in := singleComposite(testBuf([]byte("PROXY TCP4 192.0.2.1 198.51.100.1 12345 443\r\nGET / HTTP/1.1\r\n")))
	defer in.Release()

	out, err := decoder.Decode(nil, in)
	if err != nil {
		t.Fatal(err)
	}
	msg := out.(Message)
	if msg.Version != Version1 || msg.Protocol != ProtocolTCP4 || msg.SourceAddress != "192.0.2.1" || msg.SourcePort != 12345 {
		t.Fatalf("msg=%+v", msg)
	}
	if got := in.ReadableBytes(); got != len("GET / HTTP/1.1\r\n") {
		t.Fatalf("leftover=%d", got)
	}
}

func TestDecoderWaitsForPartialV2Signature(t *testing.T) {
	decoder, err := NewDecoder(0)
	if err != nil {
		t.Fatal(err)
	}
	in := singleComposite(testBuf(v2Signature[:6]))
	defer in.Release()

	out, err := decoder.Decode(nil, in)
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Fatalf("out=%+v, want nil", out)
	}
}

func TestAppendAndDecodeV2TCP6WithTLV(t *testing.T) {
	header, err := AppendHeader(nil, Message{
		Version:            Version2,
		Command:            CommandProxy,
		Protocol:           ProtocolTCP6,
		SourceAddress:      "2001:db8::1",
		DestinationAddress: "2001:db8::2",
		SourcePort:         12345,
		DestinationPort:    443,
		TLVs:               []TLV{{Type: TLVTypeALPN, Value: []byte("h2")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := NewDecoder(0)
	if err != nil {
		t.Fatal(err)
	}
	in := singleComposite(testBuf(header))
	defer in.Release()

	out, err := decoder.Decode(nil, in)
	if err != nil {
		t.Fatal(err)
	}
	msg := out.(Message)
	if msg.Version != Version2 || msg.Protocol != ProtocolTCP6 || msg.SourceAddress != "2001:db8::1" || msg.DestinationPort != 443 {
		t.Fatalf("msg=%+v", msg)
	}
	if len(msg.TLVs) != 1 || msg.TLVs[0].Type != TLVTypeALPN || string(msg.TLVs[0].Value) != "h2" {
		t.Fatalf("tlvs=%+v", msg.TLVs)
	}
}

func TestAppendAndDecodeV2UnixAddress(t *testing.T) {
	header, err := AppendHeader(nil, Message{
		Version:            Version2,
		Command:            CommandProxy,
		Protocol:           ProtocolUnixStream,
		SourceAddress:      "/var/run/source.sock",
		DestinationAddress: "/var/run/dest.sock",
	})
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := NewDecoder(0)
	if err != nil {
		t.Fatal(err)
	}
	in := singleComposite(testBuf(header))
	defer in.Release()

	out, err := decoder.Decode(nil, in)
	if err != nil {
		t.Fatal(err)
	}
	msg := out.(Message)
	if msg.Protocol != ProtocolUnixStream || msg.SourceAddress != "/var/run/source.sock" || msg.DestinationAddress != "/var/run/dest.sock" {
		t.Fatalf("msg=%+v", msg)
	}
}

func TestEncoderWritesV1Header(t *testing.T) {
	sink := &captureSink{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	if err := ch.Pipeline().AddLast("haproxy", NewEncoder()); err != nil {
		t.Fatal(err)
	}
	defer sink.release()

	err := ch.Write(Message{
		Version:            Version1,
		Command:            CommandProxy,
		Protocol:           ProtocolTCP4,
		SourceAddress:      "192.0.2.1",
		DestinationAddress: "198.51.100.1",
		SourcePort:         12345,
		DestinationPort:    443,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.writes) != 1 {
		t.Fatalf("writes=%d, want 1", len(sink.writes))
	}
	got := sink.writes[0].(buffer.ByteBuf).Bytes()
	want := []byte("PROXY TCP4 192.0.2.1 198.51.100.1 12345 443\r\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("header=%q", got)
	}
}

func BenchmarkDecoderV2TCP4(b *testing.B) {
	header, err := AppendHeader(nil, Message{
		Version:            Version2,
		Command:            CommandProxy,
		Protocol:           ProtocolTCP4,
		SourceAddress:      "192.0.2.1",
		DestinationAddress: "198.51.100.1",
		SourcePort:         12345,
		DestinationPort:    443,
	})
	if err != nil {
		b.Fatal(err)
	}
	decoder, err := NewDecoder(0)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		in := singleComposite(testBuf(header))
		out, err := decoder.Decode(nil, in)
		if err != nil {
			b.Fatal(err)
		}
		if out == nil {
			b.Fatal("nil message")
		}
		in.Release()
	}
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

func singleComposite(buf buffer.ByteBuf) *buffer.CompositeByteBuf {
	comp := buffer.NewCompositeByteBuf()
	comp.Append(buf)
	return comp
}
