package chunked

import (
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel/embedded"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/codec/http2"
)

func TestDataChunkedInputSplitsByteBufIntoDataFrames(t *testing.T) {
	src := buffer.NewHeapBuffer(8)
	if _, err := src.WriteBytes([]byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	byteInput, err := codec.NewChunkedByteBufInput(src, 2)
	if err != nil {
		t.Fatal(err)
	}
	input, err := NewDataChunkedInput(3, byteInput, true)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := embedded.New(NewWriteHandler())
	if err != nil {
		t.Fatal(err)
	}
	defer ch.FinishAndReleaseAll()

	if _, err := ch.WriteOutbound(input); err != nil {
		t.Fatal(err)
	}
	got := readFrames(t, ch)
	if len(got) != 3 {
		t.Fatalf("frames=%d, want 3", len(got))
	}
	defer releaseFrames(got)
	for i, frame := range got {
		if frame.StreamID != 3 {
			t.Fatalf("stream id=%d", frame.StreamID)
		}
		wantEnd := i == len(got)-1
		if (frame.Flags&http2.FlagEndStream != 0) != wantEnd {
			t.Fatalf("frame %d flags=%x", i, frame.Flags)
		}
	}
}

func TestDataChunkedInputEmitsEmptyEndStream(t *testing.T) {
	src := buffer.NewHeapBuffer(1)
	src.Clear()
	byteInput, err := codec.NewChunkedByteBufInput(src, 2)
	if err != nil {
		t.Fatal(err)
	}
	input, err := NewDataChunkedInput(1, byteInput, true)
	if err != nil {
		t.Fatal(err)
	}
	frame, done, err := input.ReadFrame(buffer.NewHeapAllocator())
	if err != nil {
		t.Fatal(err)
	}
	if !done || frame.StreamID != 1 || frame.Flags&http2.FlagEndStream == 0 || frame.Data != nil {
		t.Fatalf("frame=%+v done=%v", frame, done)
	}
}

func readFrames(t *testing.T, ch *embedded.EmbeddedChannel) []http2.DataFrame {
	t.Helper()
	var frames []http2.DataFrame
	for {
		msg, ok := ch.ReadOutbound()
		if !ok {
			return frames
		}
		frame, ok := msg.(http2.DataFrame)
		if !ok {
			t.Fatalf("message=%T, want DataFrame", msg)
		}
		frames = append(frames, frame)
	}
}

func releaseFrames(frames []http2.DataFrame) {
	for _, frame := range frames {
		frame.Release()
	}
}
