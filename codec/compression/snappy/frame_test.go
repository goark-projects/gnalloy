package snappy

import (
	"bytes"
	"errors"
	"testing"

	base "goark.dev/gnalloy/codec/compression"
	"goark.dev/gnalloy/codec/compression/internal/testutil"
)

func TestFrameEncoderWritesNettyUncompressedChunk(t *testing.T) {
	encoded := testutil.EncodeWithHandler(t, NewFrameEncoder(), []byte("hello"))
	defer encoded.Release()

	got := encoded.Bytes()
	if !bytes.Equal(got[:len(streamIdentifierChunk)], streamIdentifierChunk) {
		t.Fatalf("stream identifier=%v", got[:len(streamIdentifierChunk)])
	}
	offset := len(streamIdentifierChunk)
	if got[offset] != chunkTypeUncompressed {
		t.Fatalf("chunk type=%x, want uncompressed", got[offset])
	}
	if length := readLittleMedium(got[offset+1:]); length != 9 {
		t.Fatalf("chunk length=%d, want 9", length)
	}
	if payload := string(got[offset+8:]); payload != "hello" {
		t.Fatalf("payload=%q", payload)
	}
}

func TestFrameEncoderDecoderRoundTrip(t *testing.T) {
	encoder := NewFramedEncoder()
	decoder := NewFramedDecoder(FrameDecoderConfig{MaxDecodedBytes: 4096, ValidateChecksums: true})
	payload := bytes.Repeat([]byte("abcdefghijklmnop"), 16)

	encoded := testutil.EncodeWithHandler(t, encoder, payload)
	decoded := testutil.DecodeWithHandler(t, decoder, encoded)
	defer decoded.Release()
	if !bytes.Equal(decoded.Bytes(), payload) {
		t.Fatalf("decoded length=%d, want %d", decoded.ReadableBytes(), len(payload))
	}
}

func TestFrameDecoderSkipsSkippableChunk(t *testing.T) {
	encoded := testutil.EncodeWithHandler(t, NewFrameEncoder(), []byte("ok"))
	defer encoded.Release()
	raw := append([]byte{}, streamIdentifierChunk...)
	raw = append(raw, 0x80, 3, 0, 0, 's', 'k', 'p')
	raw = append(raw, encoded.Bytes()[len(streamIdentifierChunk):]...)

	decoded := testutil.DecodeWithHandler(t, NewFrameDecoder(FrameDecoderConfig{ValidateChecksums: true}), testutil.Buffer(raw))
	defer decoded.Release()
	if string(decoded.Bytes()) != "ok" {
		t.Fatalf("decoded=%q", decoded.Bytes())
	}
}

func TestFrameDecoderRejectsReservedUnskippableChunk(t *testing.T) {
	raw := append([]byte{}, streamIdentifierChunk...)
	raw = append(raw, 0x02, 0, 0, 0)
	collector := testutil.DecodeWithCollector(t, NewFrameDecoder(FrameDecoderConfig{}), testutil.Buffer(raw))
	if !errors.Is(collector.Err, ErrReservedChunkType) {
		t.Fatalf("err=%v, want %v", collector.Err, ErrReservedChunkType)
	}
}

func TestFrameDecoderEnforcesMaxDecodedBytes(t *testing.T) {
	encoded := testutil.EncodeWithHandler(t, NewFrameEncoder(), []byte("payload"))
	collector := testutil.DecodeWithCollector(t, NewFrameDecoder(FrameDecoderConfig{MaxDecodedBytes: 4}), encoded)
	if !errors.Is(collector.Err, base.ErrDecodedTooLong) {
		t.Fatalf("err=%v, want %v", collector.Err, base.ErrDecodedTooLong)
	}
}
