package deflate

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel/embedded"
	"goark.dev/gnalloy/codec/websocket"
)

func TestLegacyFrameExtensionParsesDeflateFrameNames(t *testing.T) {
	for _, header := range []string{"deflate-frame", "x-webkit-deflate-frame"} {
		name, ok, err := ParseFrameExtension(header)
		if err != nil {
			t.Fatal(err)
		}
		if !ok || name != header {
			t.Fatalf("name=%q ok=%t, want %q", name, ok, header)
		}
	}
}

func TestLegacyFrameCompressorCompressesEachFrame(t *testing.T) {
	compressor, err := NewLegacyFrameCompressor(Config{MaxMessageBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	decompressor, err := NewLegacyFrameDecompressor(Config{MaxMessageBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	out, err := embedded.New(compressor)
	if err != nil {
		t.Fatal(err)
	}
	defer out.FinishAndReleaseAll()
	in, err := embedded.New(decompressor)
	if err != nil {
		t.Fatal(err)
	}
	defer in.FinishAndReleaseAll()

	if _, err := out.WriteOutbound(websocket.Frame{Opcode: websocket.OpcodeText, Payload: testPayload("hello"), Final: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := out.WriteOutbound(websocket.Frame{Opcode: websocket.OpcodeContinuation, Payload: testPayload(" world"), Final: true}); err != nil {
		t.Fatal(err)
	}
	for {
		msg, ok := out.ReadOutbound()
		if !ok {
			break
		}
		frame := msg.(websocket.Frame)
		if !frame.RSV1 {
			t.Fatalf("compressed frame missing RSV1: %+v", frame)
		}
		if _, err := in.WriteInbound(frame); err != nil {
			t.Fatal(err)
		}
	}

	first := readInboundAs[websocket.Frame](t, in)
	defer first.Release()
	if first.RSV1 || first.Opcode != websocket.OpcodeText || first.Final || string(first.Payload.Bytes()) != "hello" {
		t.Fatalf("first=%+v body=%q", first, first.Payload.Bytes())
	}
	second := readInboundAs[websocket.Frame](t, in)
	defer second.Release()
	if second.RSV1 || second.Opcode != websocket.OpcodeContinuation || !second.Final || string(second.Payload.Bytes()) != " world" {
		t.Fatalf("second=%+v body=%q", second, second.Payload.Bytes())
	}
}

func testPayload(value string) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(value))
	if _, err := buf.WriteBytes([]byte(value)); err != nil {
		panic(err)
	}
	return buf
}

func readInboundAs[T any](t *testing.T, ch *embedded.EmbeddedChannel) T {
	t.Helper()
	msg, ok := ch.ReadInbound()
	if !ok {
		t.Fatal("missing inbound message")
	}
	value, ok := msg.(T)
	if !ok {
		t.Fatalf("message=%T, want requested type", msg)
	}
	return value
}
