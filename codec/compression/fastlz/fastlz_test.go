package fastlz

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	base "goark.dev/gnalloy/codec/compression"
	"goark.dev/gnalloy/codec/compression/internal/testutil"
)

func TestHandlersRoundTrip(t *testing.T) {
	encoder, err := NewEncoder(Config{Level: LevelAuto, Checksum: true})
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := NewDecoder(Config{Checksum: true, MaxDecodedBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("fastlz-frame-"), 64)

	compressed := testutil.EncodeWithHandler(t, encoder, payload)
	decoded := testutil.DecodeWithHandler(t, decoder, compressed)
	if !bytes.Equal(decoded.Bytes(), payload) {
		t.Fatalf("decoded payload mismatch")
	}
	decoded.Release()
}

func TestEncoderWritesNettyFastLZHeader(t *testing.T) {
	encoder, err := NewEncoder(Config{Level: Level1, Checksum: true})
	if err != nil {
		t.Fatal(err)
	}
	encoded := testutil.EncodeWithHandler(t, encoder, []byte("hello fastlz"))
	defer encoded.Release()

	data := encoded.Bytes()
	if len(data) < minHeaderLength {
		t.Fatalf("encoded length=%d, want >=%d", len(data), minHeaderLength)
	}
	if !bytes.Equal(data[:3], []byte{'F', 'L', 'Z'}) {
		t.Fatalf("magic=%q, want FLZ", data[:3])
	}
	if data[3]&optionChecksum == 0 {
		t.Fatalf("options=%02x, want checksum bit", data[3])
	}
}

func TestDecoderRejectsChecksumMismatch(t *testing.T) {
	encoder, err := NewEncoder(Config{Checksum: true})
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := NewDecoder(Config{Checksum: true, MaxDecodedBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	encoded := testutil.EncodeWithHandler(t, encoder, []byte("checksum"))
	data := append([]byte(nil), encoded.Bytes()...)
	encoded.Release()
	data[len(data)-1] ^= 0xff

	collector := testutil.DecodeWithCollector(t, decoder, testutil.Buffer(data))
	if !errors.Is(collector.Err, ErrChecksumMismatch) {
		t.Fatalf("err=%v, want %v", collector.Err, ErrChecksumMismatch)
	}
}

func TestDecoderEnforcesMaxDecodedBytes(t *testing.T) {
	encoder, err := NewEncoder(Config{Level: Level1})
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

func TestDecoderAccumulatesPartialFrames(t *testing.T) {
	encoder, err := NewEncoder(Config{})
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := NewDecoder(Config{MaxDecodedBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	encoded := testutil.EncodeWithHandler(t, encoder, []byte("split-frame"))
	data := append([]byte(nil), encoded.Bytes()...)
	encoded.Release()

	collector := &testutil.Collector{}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), nil)
	if err := ch.Pipeline().AddLast("decoder", decoder); err != nil {
		t.Fatal(err)
	}
	if err := ch.Pipeline().AddLast("collector", collector); err != nil {
		t.Fatal(err)
	}
	ch.Pipeline().FireChannelRead(testutil.Buffer(data[:2]))
	if len(collector.Reads) != 0 || collector.Err != nil {
		t.Fatalf("collector after partial read=%+v", collector)
	}
	ch.Pipeline().FireChannelRead(testutil.Buffer(data[2:]))
	if collector.Err != nil {
		t.Fatal(collector.Err)
	}
	if len(collector.Reads) != 1 {
		t.Fatalf("reads=%d, want 1", len(collector.Reads))
	}
	out := collector.Reads[0].(buffer.ByteBuf)
	defer out.Release()
	if string(out.Bytes()) != "split-frame" {
		t.Fatalf("decoded=%q", out.Bytes())
	}
}

func TestAlgorithmRoundTripSamples(t *testing.T) {
	samples := [][]byte{
		[]byte("abcd"),
		bytes.Repeat([]byte("a"), 4096),
		bytes.Repeat([]byte("0123456789abcdef"), 512),
		deterministicPayload(8192),
	}
	for _, level := range []Level{Level1, Level2} {
		for _, sample := range samples {
			tmp := make([]byte, maxOutputLength(len(sample)))
			hashes := make([]int, fastHashSize)
			n, ok := compressBlock(tmp, sample, level, hashes)
			if !ok {
				t.Fatalf("compress failed level=%d length=%d", level, len(sample))
			}
			out := make([]byte, len(sample))
			decoded, err := decompressBlock(out, tmp[:n])
			if err != nil {
				t.Fatalf("decompress level=%d length=%d: %v", level, len(sample), err)
			}
			if decoded != len(sample) || !bytes.Equal(out[:decoded], sample) {
				t.Fatalf("roundtrip mismatch level=%d length=%d decoded=%d", level, len(sample), decoded)
			}
		}
	}
}

func deterministicPayload(size int) []byte {
	random := rand.New(rand.NewPCG(1, 2))
	data := make([]byte, size)
	for i := range data {
		if i%11 == 0 {
			data[i] = byte(random.IntN(64))
			continue
		}
		data[i] = byte('a' + i%7)
	}
	return data
}
