package ascii

import (
	"bytes"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel/embedded"
)

func TestRequestDecoderParsesStorageCommand(t *testing.T) {
	decoder, err := NewRequestDecoder(1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := embedded.New(decoder)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	if _, err := ch.WriteInbound(testBuf([]byte("set key 7 60 5 noreply\r\nvalue\r\n"))); err != nil {
		t.Fatal(err)
	}
	req := readInboundAs[Request](t, ch)
	defer req.Release()
	if req.Command != CommandSet || req.Key != "key" || req.Flags != 7 || req.Exptime != 60 || !req.NoReply {
		t.Fatalf("request=%+v", req)
	}
	if string(req.Value.Bytes()) != "value" {
		t.Fatalf("value=%q", req.Value.Bytes())
	}
}

func TestResponseDecoderParsesMultiValueResponse(t *testing.T) {
	decoder, err := NewResponseDecoder(1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := embedded.New(decoder)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	wire := "VALUE a 1 3\r\none\r\nVALUE b 2 3 99\r\ntwo\r\nEND\r\n"
	if _, err := ch.WriteInbound(testBuf([]byte(wire))); err != nil {
		t.Fatal(err)
	}
	resp := readInboundAs[Response](t, ch)
	defer resp.Release()
	if resp.Status != StatusEnd || len(resp.Values) != 2 {
		t.Fatalf("response=%+v", resp)
	}
	if resp.Values[0].Key != "a" || string(resp.Values[0].Data.Bytes()) != "one" {
		t.Fatalf("first=%+v", resp.Values[0])
	}
	if resp.Values[1].Key != "b" || resp.Values[1].CAS != 99 || string(resp.Values[1].Data.Bytes()) != "two" {
		t.Fatalf("second=%+v", resp.Values[1])
	}
}

func TestRequestEncoderWritesRetrievalAndStorageCommands(t *testing.T) {
	ch, err := embedded.New(NewRequestEncoder())
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	if _, err := ch.WriteOutbound(Request{Command: CommandGet, Keys: []string{"a", "b"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.WriteOutbound(Request{Command: CommandSet, Key: "k", Flags: 1, Exptime: 2, Value: testBuf([]byte("abc"))}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	for {
		msg, ok := ch.ReadOutbound()
		if !ok {
			break
		}
		buf := msg.(buffer.ByteBuf)
		out.Write(buf.Bytes())
		buf.Release()
	}
	if got := out.String(); got != "get a b\r\nset k 1 2 3\r\nabc\r\n" {
		t.Fatalf("wire=%q", got)
	}
}

func testBuf(src []byte) buffer.ByteBuf {
	buf := buffer.NewHeapBuffer(len(src))
	if _, err := buf.WriteBytes(src); err != nil {
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
