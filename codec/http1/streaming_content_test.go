package http1

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/codec"
)

func TestContentEncodingInputStreamsHTTP1GzipBody(t *testing.T) {
	source := buffer.NewHeapBuffer(512)
	plain := []byte(strings.Repeat("http1-stream-", 24))
	if _, err := source.WriteBytes(plain); err != nil {
		t.Fatal(err)
	}
	raw, err := codec.NewChunkedByteBufInput(source, 23)
	if err != nil {
		t.Fatal(err)
	}
	input, err := NewContentEncodingInput(raw, ContentCodingGzip, ContentEncodingInputConfig{ChunkSize: 31})
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()

	var encoded bytes.Buffer
	chunks := 0
	for {
		chunk, done, err := input.ReadChunk(buffer.NewHeapAllocator())
		if err != nil {
			t.Fatal(err)
		}
		if chunk != nil {
			chunks++
			encoded.Write(chunk.Bytes())
			chunk.Release()
		}
		if done {
			break
		}
	}
	if chunks < 2 {
		t.Fatalf("chunks=%d, want streaming output", chunks)
	}
	reader, err := gzip.NewReader(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != string(plain) {
		t.Fatalf("decoded=%q", decoded)
	}
}
