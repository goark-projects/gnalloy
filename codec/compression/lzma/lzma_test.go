package lzma

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
	decoder, err := NewDecoder(1024, Config{})
	if err != nil {
		t.Fatal(err)
	}
	compressed := testutil.EncodeWithHandler(t, encoder, []byte("hello lzma"))
	decoded := testutil.DecodeWithHandler(t, decoder, compressed)
	if !bytes.Equal(decoded.Bytes(), []byte("hello lzma")) {
		t.Fatalf("decoded=%q", decoded.Bytes())
	}
	decoded.Release()
}

func TestDecoderEnforcesMaxDecodedBytes(t *testing.T) {
	encoder, err := NewEncoder(Config{})
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := NewDecoder(4, Config{})
	if err != nil {
		t.Fatal(err)
	}
	compressed := testutil.EncodeWithHandler(t, encoder, []byte("payload"))
	collector := testutil.DecodeWithCollector(t, decoder, compressed)
	if !errors.Is(collector.Err, base.ErrDecodedTooLong) {
		t.Fatalf("err=%v, want %v", collector.Err, base.ErrDecodedTooLong)
	}
}

func TestRejectsInvalidConfig(t *testing.T) {
	_, err := NewEncoder(Config{DictCap: 1})
	if !errors.Is(err, base.ErrInvalidConfig) {
		t.Fatalf("encoder err=%v, want %v", err, base.ErrInvalidConfig)
	}
	_, err = NewDecoder(-1, Config{})
	if !errors.Is(err, base.ErrInvalidConfig) {
		t.Fatalf("decoder err=%v, want %v", err, base.ErrInvalidConfig)
	}
}
