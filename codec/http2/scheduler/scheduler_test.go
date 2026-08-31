package scheduler

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/codec/http2"
)

func TestWeightedFairQueueDistributorHonorsWeights(t *testing.T) {
	d := NewWeightedFairQueueByteDistributor(Config{Quantum: 100})
	if err := d.UpdateStream(StreamState{ID: 1, Weight: 1, PendingBytes: 1000}); err != nil {
		t.Fatal(err)
	}
	if err := d.UpdateStream(StreamState{ID: 3, Weight: 3, PendingBytes: 1000}); err != nil {
		t.Fatal(err)
	}

	writes := map[http2.StreamID]int{}
	total, err := d.Distribute(400, func(id http2.StreamID, maxBytes int) (int, error) {
		writes[id] += maxBytes
		return maxBytes, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 400 || writes[1] != 100 || writes[3] != 300 {
		t.Fatalf("total=%d writes=%v", total, writes)
	}
}

func TestWeightedFairQueueDistributorSkipsInactiveStreams(t *testing.T) {
	d := NewWeightedFairQueueByteDistributor(Config{Quantum: 64})
	if err := d.UpdateStream(StreamState{ID: 1, Weight: 16, PendingBytes: 0}); err != nil {
		t.Fatal(err)
	}
	if err := d.UpdateStream(StreamState{ID: 3, Weight: 16, PendingBytes: 128}); err != nil {
		t.Fatal(err)
	}

	total, err := d.Distribute(64, func(id http2.StreamID, maxBytes int) (int, error) {
		if id != 3 {
			t.Fatalf("id=%d, want 3", id)
		}
		return maxBytes, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 64 || d.PendingBytes(3) != 64 {
		t.Fatalf("total=%d pending=%d", total, d.PendingBytes(3))
	}
}

func TestWeightedFairQueueDistributorPropagatesWriterError(t *testing.T) {
	d := NewWeightedFairQueueByteDistributor(Config{Quantum: 64})
	if err := d.UpdateStream(StreamState{ID: 1, Weight: 16, PendingBytes: 64}); err != nil {
		t.Fatal(err)
	}
	want := errors.New("stop")
	_, err := d.Distribute(64, func(http2.StreamID, int) (int, error) {
		return 0, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err=%v, want %v", err, want)
	}
}
