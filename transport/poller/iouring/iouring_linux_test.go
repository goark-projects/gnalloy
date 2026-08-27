//go:build linux

package iouring

import (
	"errors"
	"sync"
	"testing"
	"time"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport/poller"
)

func TestNewWithConfigRejectsInvalidSQPollAffinity(t *testing.T) {
	_, err := NewWithConfig(Config{SQPollAffinity: true})
	if !errors.Is(err, poller.ErrInvalidIORequest) {
		t.Fatalf("err=%v, want %v", err, poller.ErrInvalidIORequest)
	}
}

func TestStatsReflectsMultishotAccept(t *testing.T) {
	raw, err := NewWithConfig(Config{Entries: 8, MultishotAccept: true})
	if err != nil {
		t.Skip(err)
	}
	p := raw.(*Poller)
	defer p.Close()

	stats := p.Stats()
	if !stats.MultishotAccept {
		t.Fatalf("stats=%+v, want multishot accept", stats)
	}
	if stats.SQAvailable == 0 {
		t.Fatalf("stats=%+v, want available SQ entries", stats)
	}
}

func TestRegisterBuffersStatsAndUnregister(t *testing.T) {
	raw, err := NewWithConfig(Config{Entries: 8})
	if err != nil {
		t.Skip(err)
	}
	p := raw.(*Poller)
	defer p.Close()

	if err := p.RegisterBuffers(nil); !errors.Is(err, poller.ErrInvalidIORequest) {
		t.Fatalf("empty register err=%v, want %v", err, poller.ErrInvalidIORequest)
	}
	mem := make([]byte, 4096)
	if err := p.RegisterBuffers([][]byte{mem}); err != nil {
		t.Skip(err)
	}
	if stats := p.Stats(); !stats.RegisteredBuffers {
		t.Fatalf("stats=%+v, want registered buffers", stats)
	}
	if err := p.UnregisterBuffers(); err != nil {
		t.Fatal(err)
	}
	if stats := p.Stats(); stats.RegisteredBuffers {
		t.Fatalf("stats=%+v, want no registered buffers", stats)
	}
}

func TestRegisterMmapAllocatorFixedBuffers(t *testing.T) {
	raw, err := NewWithConfig(Config{Entries: 8})
	if err != nil {
		t.Skip(err)
	}
	p := raw.(*Poller)
	defer p.Close()

	alloc, err := buffer.NewMmapAllocator(buffer.MmapAllocatorConfig{BlockSize: 4096, Blocks: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer alloc.Close()

	fixed := alloc.(buffer.FixedBufferProvider).FixedBuffers()
	if err := p.RegisterBuffers(fixed); err != nil {
		t.Skip(err)
	}
	if stats := p.Stats(); !stats.RegisteredBuffers {
		t.Fatalf("stats=%+v, want registered buffers", stats)
	}
	buf, err := alloc.Acquire(128)
	if err != nil {
		t.Fatal(err)
	}
	idx, ok := buffer.FixedBufferIndex(buf)
	if !ok || int(idx) >= len(fixed) {
		t.Fatalf("fixed index=(%d,%v), fixed buffers=%d", idx, ok, len(fixed))
	}
	buf.Release()
	if err := p.UnregisterBuffers(); err != nil {
		t.Fatal(err)
	}
}

func TestSubmitEnterFlagsWakeSQPollThread(t *testing.T) {
	var flags uint32
	p := &Poller{sq: sq{flags: &flags}}
	if got := p.submitEnterFlags(); got != 0 {
		t.Fatalf("flags=%d, want 0", got)
	}

	flags = sqNeedWakeup
	if got := p.submitEnterFlags(); got != enterSQWakeup {
		t.Fatalf("flags=%d, want %d", got, enterSQWakeup)
	}
}

func TestWakeupConcurrent(t *testing.T) {
	raw, err := NewWithConfig(Config{Entries: 1024})
	if err != nil {
		t.Skip(err)
	}
	p := raw.(*Poller)
	defer p.Close()

	const (
		goroutines        = 8
		wakeupsPerRoutine = 64
	)
	start := make(chan struct{})
	errs := make(chan error, goroutines)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for range wakeupsPerRoutine {
				if err := p.Wakeup(); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	events := make([]poller.Event, 16)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		n, err := p.Poll(events, 10)
		if err != nil {
			t.Fatal(err)
		}
		for i := range n {
			if events[i].Op == poller.OpWakeup {
				return
			}
		}
	}
	t.Fatal("未收到 io_uring 唤醒事件")
}
