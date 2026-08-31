package lzf

import (
	"bytes"
	"errors"
	"testing"

	base "goark.dev/gnalloy/codec/compression"
	"goark.dev/gnalloy/codec/compression/internal/testutil"
)

func TestHandlersRoundTrip(t *testing.T) {
	encoder, err := NewEncoder(Config{})
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := NewDecoder(Config{MaxDecodedBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("lzf-frame-"), 80)

	compressed := testutil.EncodeWithHandler(t, encoder, payload)
	decoded := testutil.DecodeWithHandler(t, decoder, compressed)
	if !bytes.Equal(decoded.Bytes(), payload) {
		t.Fatalf("decoded payload mismatch")
	}
	decoded.Release()
}

func TestEncoderWritesNettyLZFHeader(t *testing.T) {
	encoder, err := NewEncoder(Config{})
	if err != nil {
		t.Fatal(err)
	}
	encoded := testutil.EncodeWithHandler(t, encoder, []byte("small"))
	defer encoded.Release()

	data := encoded.Bytes()
	if len(data) != rawHeaderLength+len("small") {
		t.Fatalf("encoded length=%d", len(data))
	}
	if !bytes.Equal(data[:3], []byte{'Z', 'V', blockTypeRaw}) {
		t.Fatalf("header=% x, want ZV raw", data[:3])
	}
}

func TestDecoderEnforcesMaxDecodedBytes(t *testing.T) {
	encoder, err := NewEncoder(Config{})
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := NewDecoder(Config{MaxDecodedBytes: 8})
	if err != nil {
		t.Fatal(err)
	}
	encoded := testutil.EncodeWithHandler(t, encoder, bytes.Repeat([]byte("x"), 64))

	collector := testutil.DecodeWithCollector(t, decoder, encoded)
	if !errors.Is(collector.Err, base.ErrDecodedTooLong) {
		t.Fatalf("err=%v, want %v", collector.Err, base.ErrDecodedTooLong)
	}
}

func TestDecoderRejectsBadMagic(t *testing.T) {
	decoder, err := NewDecoder(Config{})
	if err != nil {
		t.Fatal(err)
	}
	frame := append([]byte(nil), 'B', 'V', blockTypeRaw, 0, 1, 'x')

	collector := testutil.DecodeWithCollector(t, decoder, testutil.Buffer(frame))
	if !errors.Is(collector.Err, ErrCorruptFrame) {
		t.Fatalf("err=%v, want %v", collector.Err, ErrCorruptFrame)
	}
}

func TestAlgorithmRoundTripSamples(t *testing.T) {
	samples := [][]byte{
		[]byte("0123456789abcdef"),
		bytes.Repeat([]byte("a"), 4096),
		bytes.Repeat([]byte("lzf-pattern-"), 512),
	}
	for _, sample := range samples {
		tmp := make([]byte, maxCompressedLength(len(sample)))
		hashes := make([]int, lzfHashSize)
		n, ok := compressBlock(tmp, sample, hashes)
		if !ok {
			t.Fatalf("compress failed length=%d", len(sample))
		}
		out := make([]byte, len(sample))
		decoded, err := decompressBlock(out, tmp[:n])
		if err != nil {
			t.Fatalf("decompress length=%d: %v", len(sample), err)
		}
		if decoded != len(sample) || !bytes.Equal(out[:decoded], sample) {
			t.Fatalf("roundtrip mismatch length=%d decoded=%d", len(sample), decoded)
		}
	}
}
