package compression

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/codec"
)

func TestCompressingChunkedInputStreamsGzip(t *testing.T) {
	source := buffer.NewHeapBuffer(256)
	plain := []byte(strings.Repeat("streaming-body-", 16))
	if _, err := source.WriteBytes(plain); err != nil {
		t.Fatal(err)
	}
	raw, err := codec.NewChunkedByteBufInput(source, 17)
	if err != nil {
		t.Fatal(err)
	}
	input, err := NewCompressingChunkedInput(raw, ChunkedEncoderConfig{
		Format:    FormatGzip,
		Level:     gzip.BestSpeed,
		ChunkSize: 19,
	})
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
