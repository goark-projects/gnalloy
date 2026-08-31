package http3

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel/embedded"
)

func TestQPACKDynamicTableEvictsByCapacity(t *testing.T) {
	table := NewQPACKDynamicTable(72)

	first, err := table.Insert(HeaderField{Name: "x-a", Value: "1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := table.Insert(HeaderField{Name: "x-b", Value: "2"})
	if err != nil {
		t.Fatal(err)
	}
	third, err := table.Insert(HeaderField{Name: "x-c", Value: "3"})
	if err != nil {
		t.Fatal(err)
	}

	if first != 0 || second != 1 || third != 2 {
		t.Fatalf("indices=(%d,%d,%d), want (0,1,2)", first, second, third)
	}
	if table.InsertCount() != 3 || table.Size() != 72 {
		t.Fatalf("insertCount=%d size=%d", table.InsertCount(), table.Size())
	}
	if _, ok := table.GetAbsolute(0); ok {
		t.Fatal("oldest entry should be evicted")
	}
	if field, ok := table.GetRelative(0); !ok || field.Name != "x-c" {
		t.Fatalf("relative newest=%+v ok=%t", field, ok)
	}
	if field, ok := table.GetAbsolute(1); !ok || field.Name != "x-b" {
		t.Fatalf("absolute second=%+v ok=%t", field, ok)
	}
}

func TestQPACKEncoderStreamInstructionRoundTrip(t *testing.T) {
	out, err := embedded.New(NewQPACKEncoderStreamEncoder())
	if err != nil {
		t.Fatal(err)
	}
	defer out.FinishAndReleaseAll()

	if _, err := out.WriteOutbound(QPACKSetDynamicTableCapacity{Capacity: 128}); err != nil {
		t.Fatal(err)
	}
	if _, err := out.WriteOutbound(QPACKInsertWithoutNameRef{Field: HeaderField{Name: "x-token", Value: "abc"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := out.WriteOutbound(QPACKDuplicate{Index: 0}); err != nil {
		t.Fatal(err)
	}

	in, err := embedded.New(NewQPACKEncoderStreamDecoder())
	if err != nil {
		t.Fatal(err)
	}
	defer in.FinishAndReleaseAll()
	for {
		msg, ok := out.ReadOutbound()
		if !ok {
			break
		}
		buf := msg.(buffer.ByteBuf)
		if _, err := in.WriteInbound(buf); err != nil {
			t.Fatal(err)
		}
	}

	want := []any{
		QPACKSetDynamicTableCapacity{Capacity: 128},
		QPACKInsertWithoutNameRef{Field: HeaderField{Name: "x-token", Value: "abc"}},
		QPACKDuplicate{Index: 0},
	}
	for i, expected := range want {
		got, ok := in.ReadInbound()
		if !ok {
			t.Fatalf("missing decoded instruction %d", i)
		}
		if got != expected {
			t.Fatalf("instruction %d=%#v, want %#v", i, got, expected)
		}
	}
}

func TestQPACKDecoderStreamInstructionRoundTrip(t *testing.T) {
	out, err := embedded.New(NewQPACKDecoderStreamEncoder())
	if err != nil {
		t.Fatal(err)
	}
	defer out.FinishAndReleaseAll()

	if _, err := out.WriteOutbound(QPACKSectionAcknowledgment{StreamID: 11}); err != nil {
		t.Fatal(err)
	}
	if _, err := out.WriteOutbound(QPACKStreamCancellation{StreamID: 13}); err != nil {
		t.Fatal(err)
	}
	if _, err := out.WriteOutbound(QPACKInsertCountIncrement{Increment: 2}); err != nil {
		t.Fatal(err)
	}

	in, err := embedded.New(NewQPACKDecoderStreamDecoder())
	if err != nil {
		t.Fatal(err)
	}
	defer in.FinishAndReleaseAll()
	for {
		msg, ok := out.ReadOutbound()
		if !ok {
			break
		}
		buf := msg.(buffer.ByteBuf)
		if _, err := in.WriteInbound(buf); err != nil {
			t.Fatal(err)
		}
	}

	want := []any{
		QPACKSectionAcknowledgment{StreamID: 11},
		QPACKStreamCancellation{StreamID: 13},
		QPACKInsertCountIncrement{Increment: 2},
	}
	for i, expected := range want {
		got, ok := in.ReadInbound()
		if !ok {
			t.Fatalf("missing decoded instruction %d", i)
		}
		if got != expected {
			t.Fatalf("instruction %d=%#v, want %#v", i, got, expected)
		}
	}
}

func TestQPACKStateAppliesEncoderAndDecoderSemantics(t *testing.T) {
	state := NewQPACKDynamicState(QPACKDynamicStateConfig{
		MaxTableCapacity:  96,
		MaxBlockedStreams: 1,
	})
	if err := state.ApplyEncoderInstruction(QPACKSetDynamicTableCapacity{Capacity: 96}); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyEncoderInstruction(QPACKInsertWithoutNameRef{Field: HeaderField{Name: "x-a", Value: "1"}}); err != nil {
		t.Fatal(err)
	}
	if blocked, err := state.StartFieldSection(3, 2); err != nil || !blocked {
		t.Fatalf("blocked=%t err=%v, want blocked", blocked, err)
	}
	if _, err := state.StartFieldSection(5, 3); !errors.Is(err, ErrQPACKBlockedStreamsExceeded) {
		t.Fatalf("err=%v, want ErrQPACKBlockedStreamsExceeded", err)
	}
	if err := state.ApplyEncoderInstruction(QPACKInsertWithoutNameRef{Field: HeaderField{Name: "x-b", Value: "2"}}); err != nil {
		t.Fatal(err)
	}
	if state.BlockedStreams() != 0 {
		t.Fatalf("blocked=%d, want 0", state.BlockedStreams())
	}
	if err := state.TrackFieldSection(3, 2); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyDecoderInstruction(QPACKSectionAcknowledgment{StreamID: 3}); err != nil {
		t.Fatal(err)
	}
	if got := state.KnownReceivedCount(); got != 2 {
		t.Fatalf("knownReceived=%d, want 2", got)
	}
	if err := state.ApplyDecoderInstruction(QPACKInsertCountIncrement{Increment: 0}); !errors.Is(err, ErrQPACKInvalidInstruction) {
		t.Fatalf("err=%v, want ErrQPACKInvalidInstruction", err)
	}
}

func TestQPACKStateRejectsCapacityAboveSettings(t *testing.T) {
	state := NewQPACKDynamicState(QPACKDynamicStateConfig{MaxTableCapacity: 16})
	err := state.ApplyEncoderInstruction(QPACKSetDynamicTableCapacity{Capacity: 17})
	if !errors.Is(err, ErrQPACKCapacityExceeded) {
		t.Fatalf("err=%v, want ErrQPACKCapacityExceeded", err)
	}
}
