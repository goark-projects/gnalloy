package quic

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
)

func TestAppendParseInitialHeader(t *testing.T) {
	header := Header{
		Type:               PacketInitial,
		Version:            Version1,
		DestinationID:      MustConnectionID([]byte{0xde, 0xad}),
		SourceID:           MustConnectionID([]byte{0xbe, 0xef, 0x01}),
		PacketNumberLength: 2,
		PacketNumber:       0x1234,
		Token:              []byte{0xaa, 0xbb},
		Length:             3,
	}
	encoded, err := AppendHeader(nil, header)
	if err != nil {
		t.Fatal(err)
	}

	parsed, n, err := ParseHeaderBytes(encoded, HeaderParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if n != len(encoded) || parsed.HeaderLength != len(encoded) {
		t.Fatalf("header length=%d/%d, want %d", n, parsed.HeaderLength, len(encoded))
	}
	if parsed.Form != HeaderFormLong || parsed.Type != PacketInitial || parsed.Version != Version1 {
		t.Fatalf("parsed header=%+v", parsed)
	}
	if !parsed.DestinationID.Equal(header.DestinationID) || !parsed.SourceID.Equal(header.SourceID) {
		t.Fatalf("parsed cid=%s/%s", parsed.DestinationID, parsed.SourceID)
	}
	if parsed.TokenLength != 2 || parsed.Length != 3 || parsed.PacketNumberLength != 2 || parsed.PacketNumber != 0x1234 {
		t.Fatalf("parsed=%+v", parsed)
	}
}

func TestAppendParseHandshakeHeaderFromByteBuf(t *testing.T) {
	header := Header{
		Type:               PacketHandshake,
		Version:            Version1,
		DestinationID:      MustConnectionID([]byte{1}),
		SourceID:           MustConnectionID([]byte{2}),
		Flags:              0x0c,
		PacketNumberLength: 4,
		PacketNumber:       0x01020304,
		Length:             8,
	}
	encoded, err := AppendHeader(nil, header)
	if err != nil {
		t.Fatal(err)
	}
	buf := buffer.NewHeapBuffer(len(encoded))
	_, _ = buf.WriteBytes(encoded)
	defer buf.Release()

	parsed, n, err := ParseHeader(buf, HeaderParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if n != len(encoded) || parsed.Type != PacketHandshake || parsed.Flags != 0x0c {
		t.Fatalf("parsed=%+v len=%d", parsed, n)
	}
	if parsed.PacketNumber != 0x01020304 || parsed.Length != 8 {
		t.Fatalf("parsed packet number=%#x length=%d", parsed.PacketNumber, parsed.Length)
	}
}

func TestAppendParseShortHeaderRequiresConfiguredCIDLength(t *testing.T) {
	header := Header{
		Form:               HeaderFormShort,
		Type:               PacketShort,
		DestinationID:      MustConnectionID([]byte{9, 8, 7, 6}),
		Flags:              0x24,
		PacketNumberLength: 1,
		PacketNumber:       0x7f,
	}
	encoded, err := AppendHeader(nil, header)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = ParseHeaderBytes(encoded, HeaderParseOptions{})
	if !errors.Is(err, ErrInvalidConnectionID) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidConnectionID)
	}
	parsed, n, err := ParseHeaderBytes(encoded, HeaderParseOptions{ShortDestinationIDLength: header.DestinationID.Len()})
	if err != nil {
		t.Fatal(err)
	}
	if n != len(encoded) || parsed.Form != HeaderFormShort || parsed.Type != PacketShort {
		t.Fatalf("parsed=%+v len=%d", parsed, n)
	}
	if !parsed.DestinationID.Equal(header.DestinationID) || parsed.Flags != 0x24 || parsed.PacketNumber != 0x7f {
		t.Fatalf("parsed=%+v", parsed)
	}
}

func TestAppendParseRetryHeader(t *testing.T) {
	header := Header{
		Type:          PacketRetry,
		Version:       Version1,
		DestinationID: MustConnectionID([]byte{1, 2}),
		SourceID:      MustConnectionID([]byte{3, 4}),
		Token:         []byte("retry-token-and-tag"),
	}
	encoded, err := AppendHeader(nil, header)
	if err != nil {
		t.Fatal(err)
	}
	parsed, n, err := ParseHeaderBytes(encoded, HeaderParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if n != len(encoded) || parsed.Type != PacketRetry || parsed.TokenLength != uint64(len(header.Token)) {
		t.Fatalf("parsed=%+v len=%d", parsed, n)
	}
}

func TestVarIntRoundTripUsesMinimalEncoding(t *testing.T) {
	tests := []struct {
		value uint64
		size  int
	}{
		{value: 0, size: 1},
		{value: 63, size: 1},
		{value: 64, size: 2},
		{value: 16383, size: 2},
		{value: 16384, size: 4},
		{value: 1073741823, size: 4},
		{value: 1073741824, size: 8},
		{value: maxVarInt, size: 8},
	}
	for _, tt := range tests {
		encoded, err := AppendVarInt(nil, tt.value)
		if err != nil {
			t.Fatalf("value=%d err=%v", tt.value, err)
		}
		if len(encoded) != tt.size {
			t.Fatalf("value=%d encoded size=%d, want %d", tt.value, len(encoded), tt.size)
		}
		got, n, err := ParseVarInt(encoded)
		if err != nil {
			t.Fatalf("value=%d parse err=%v", tt.value, err)
		}
		if got != tt.value || n != tt.size {
			t.Fatalf("value=%d got=%d n=%d", tt.value, got, n)
		}
	}
}

func TestHeaderRejectsMalformedInput(t *testing.T) {
	_, _, err := ParseHeaderBytes([]byte{0x80}, HeaderParseOptions{})
	if !errors.Is(err, ErrInvalidHeader) {
		t.Fatalf("short long header err=%v, want %v", err, ErrInvalidHeader)
	}
	_, _, err = ParseHeaderBytes([]byte{0x80, 0, 0, 0, 1, 0, 0}, HeaderParseOptions{})
	if !errors.Is(err, ErrInvalidHeader) {
		t.Fatalf("missing fixed bit err=%v, want %v", err, ErrInvalidHeader)
	}
	_, _, err = ParseHeaderBytes([]byte{0xc0, 0, 0, 0, 2, 0, 0}, HeaderParseOptions{})
	if !errors.Is(err, ErrInvalidVersion) {
		t.Fatalf("version err=%v, want %v", err, ErrInvalidVersion)
	}
	_, _, err = ParseHeaderBytes([]byte{0x40, 1, 2}, HeaderParseOptions{ShortDestinationIDLength: 4})
	if !errors.Is(err, ErrInvalidHeader) {
		t.Fatalf("truncated short err=%v, want %v", err, ErrInvalidHeader)
	}
}

func TestVarIntRejectsMalformedInput(t *testing.T) {
	_, _, err := ParseVarInt(nil)
	if !errors.Is(err, ErrInvalidVarInt) {
		t.Fatalf("empty err=%v, want %v", err, ErrInvalidVarInt)
	}
	_, _, err = ParseVarInt([]byte{0x40})
	if !errors.Is(err, ErrInvalidVarInt) {
		t.Fatalf("truncated err=%v, want %v", err, ErrInvalidVarInt)
	}
	_, err = AppendVarInt(nil, maxVarInt+1)
	if !errors.Is(err, ErrInvalidVarInt) {
		t.Fatalf("overflow err=%v, want %v", err, ErrInvalidVarInt)
	}
}
