package zstd

import (
	"bytes"
	"errors"
	"testing"

	base "goark.dev/gnalloy/codec/compression"
	"goark.dev/gnalloy/codec/compression/internal/testutil"
)

func TestHandlersRoundTrip(t *testing.T) {
	encoder, err := NewEncoder(DefaultLevel)
	if err != nil {
		t.Fatal(err)
	}
	decoder := NewDecoder(1024)
	compressed := testutil.EncodeWithHandler(t, encoder, []byte("hello zstd"))
	decoded := testutil.DecodeWithHandler(t, decoder, compressed)
	if !bytes.Equal(decoded.Bytes(), []byte("hello zstd")) {
		t.Fatalf("decoded=%q", decoded.Bytes())
	}
	decoded.Release()
}

func TestDecoderEnforcesMaxDecodedBytes(t *testing.T) {
	encoder, err := NewEncoder(SpeedFastest)
	if err != nil {
		t.Fatal(err)
	}
	decoder := NewDecoder(4)
	compressed := testutil.EncodeWithHandler(t, encoder, []byte("payload"))
	collector := testutil.DecodeWithCollector(t, decoder, compressed)
	if !errors.Is(collector.Err, base.ErrDecodedTooLong) {
		t.Fatalf("err=%v, want %v", collector.Err, base.ErrDecodedTooLong)
	}
}

func TestEncoderRejectsInvalidLevel(t *testing.T) {
	_, err := NewEncoder(SpeedBest + 1)
	if !errors.Is(err, base.ErrInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, base.ErrInvalidConfig)
	}
}
