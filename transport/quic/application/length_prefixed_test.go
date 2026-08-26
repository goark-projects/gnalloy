package application

import (
	"bytes"
	"errors"
	"testing"
)

func TestLengthPrefixedCodecRoundTrip(t *testing.T) {
	var wire bytes.Buffer
	codec := LengthPrefixedCodec{MaxFrameSize: 32}
	if err := codec.WriteFrame(&wire, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if got := wire.Bytes(); !bytes.Equal(got, []byte{0, 5, 'h', 'e', 'l', 'l', 'o'}) {
		t.Fatalf("wire=%v", got)
	}
	payload, err := codec.ReadFrame(&wire)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "hello" {
		t.Fatalf("payload=%q", payload)
	}
}

func TestLengthPrefixedCodecRejectsOversizedFrame(t *testing.T) {
	codec := LengthPrefixedCodec{MaxFrameSize: 4}
	if err := codec.WriteFrame(&bytes.Buffer{}, []byte("hello")); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("err=%v, want %v", err, ErrFrameTooLarge)
	}
	_, err := codec.ReadFrame(bytes.NewReader([]byte{0, 5, 'h', 'e', 'l', 'l', 'o'}))
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("err=%v, want %v", err, ErrFrameTooLarge)
	}
}
