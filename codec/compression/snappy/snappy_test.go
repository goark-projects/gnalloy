package snappy

import (
	"bytes"
	"errors"
	"testing"

	base "goark.dev/gnalloy/codec/compression"
	"goark.dev/gnalloy/codec/compression/internal/testutil"
)

func TestHandlersRoundTrip(t *testing.T) {
	encoder := NewEncoder()
	decoder := NewDecoder(1024)
	compressed := testutil.EncodeWithHandler(t, encoder, []byte("hello snappy"))
	decoded := testutil.DecodeWithHandler(t, decoder, compressed)
	if !bytes.Equal(decoded.Bytes(), []byte("hello snappy")) {
		t.Fatalf("decoded=%q", decoded.Bytes())
	}
	decoded.Release()
}

func TestDecoderEnforcesMaxDecodedBytes(t *testing.T) {
	encoder := NewEncoder()
	decoder := NewDecoder(4)
	compressed := testutil.EncodeWithHandler(t, encoder, []byte("payload"))
	collector := testutil.DecodeWithCollector(t, decoder, compressed)
	if !errors.Is(collector.Err, base.ErrDecodedTooLong) {
		t.Fatalf("err=%v, want %v", collector.Err, base.ErrDecodedTooLong)
	}
}
