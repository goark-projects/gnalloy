package content

import (
	"bytes"
	"strings"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel/embedded"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/codec/http2"
	h2chunked "goark.dev/gnalloy/codec/http2/chunked"
)

func TestDataCompressingInputStreamsHTTP2DataFrames(t *testing.T) {
	source := buffer.NewHeapBuffer(512)
	plain := []byte(strings.Repeat("http2-stream-", 24))
	if _, err := source.WriteBytes(plain); err != nil {
		t.Fatal(err)
	}
	raw, err := codec.NewChunkedByteBufInput(source, 23)
	if err != nil {
		t.Fatal(err)
	}
	input, err := NewDataCompressingInput(3, raw, CodingGzip, DataCompressingInputConfig{ChunkSize: 31}, true)
	if err != nil {
		t.Fatal(err)
	}

	ch, err := embedded.New(h2chunked.NewWriteHandler())
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()
	if _, err := ch.WriteOutbound(input); err != nil {
		t.Fatal(err)
	}

	var encoded bytes.Buffer
	frames := 0
	last := false
	for {
		msg, ok := ch.ReadOutbound()
		if !ok {
			break
		}
		frame := msg.(http2.DataFrame)
		frames++
		if frame.Data != nil {
			encoded.Write(frame.Data.Bytes())
		}
		last = frame.Flags&http2.FlagEndStream != 0
		frame.Release()
	}
	if frames < 2 || !last {
		t.Fatalf("frames=%d last=%t", frames, last)
	}
	if got := gunzipBytes(t, encoded.Bytes()); string(got) != string(plain) {
		t.Fatalf("decoded=%q", got)
	}
}
