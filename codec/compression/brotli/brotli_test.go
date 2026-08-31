package brotli

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
	compressed := testutil.EncodeWithHandler(t, encoder, []byte("hello brotli"))
	decoded := testutil.DecodeWithHandler(t, decoder, compressed)
	if !bytes.Equal(decoded.Bytes(), []byte("hello brotli")) {
		t.Fatalf("decoded=%q", decoded.Bytes())
	}
	decoded.Release()
}

func TestDecoderEnforcesMaxDecodedBytes(t *testing.T) {
	encoder, err := NewEncoder(BestSpeed)
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
	_, err := NewEncoder(12)
	if !errors.Is(err, base.ErrInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, base.ErrInvalidConfig)
	}
}
