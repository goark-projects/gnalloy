package tls

import (
	"testing"

	"goark.dev/gnalloy/buffer"
)

func TestCloseReleasesQueuedApplicationBuffers(t *testing.T) {
	released := 0
	handler := newHandler(ModeServer, Config{})
	buf := buffer.NewOwnedBuffer([]byte("queued"), func([]byte) {
		released++
	})
	handler.pending.Add(1)
	handler.app <- buf

	handler.close()

	if released != 1 {
		t.Fatalf("released=%d, want 1", released)
	}
	if pending := handler.pending.Load(); pending != 0 {
		t.Fatalf("pending=%d, want 0", pending)
	}
}
