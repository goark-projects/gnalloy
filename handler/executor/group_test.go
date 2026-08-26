package executor

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestGroupRunsSubmittedTasks(t *testing.T) {
	group, err := NewGroup(Config{Size: 2, QueueSize: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer group.Close()

	done := make(chan int, 1)
	if err := group.Submit(func() { done <- 7 }); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got != 7 {
			t.Fatalf("got=%d", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting task")
	}
}

func TestGroupRejectsWhenWorkerQueueFull(t *testing.T) {
	group, err := NewGroup(Config{Size: 1, QueueSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer group.Close()

	started := make(chan struct{})
	block := make(chan struct{})
	if err := group.Submit(func() {
		close(started)
		<-block
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := group.Submit(func() {}); err != nil {
		close(block)
		t.Fatal(err)
	}
	err = group.Submit(func() {})
	close(block)
	if !errors.Is(err, ErrTaskQueueFull) {
		t.Fatalf("err=%v, want %v", err, ErrTaskQueueFull)
	}
}

func TestGroupShutdownDrainsQueuedTasks(t *testing.T) {
	group, err := NewGroup(Config{Size: 1, QueueSize: 4})
	if err != nil {
		t.Fatal(err)
	}

	var (
		mu   sync.Mutex
		seen []int
	)
	for i := 0; i < 3; i++ {
		i := i
		if err := group.Submit(func() {
			mu.Lock()
			seen = append(seen, i)
			mu.Unlock()
		}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := group.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 3 {
		t.Fatalf("seen=%v", seen)
	}
	if err := group.Submit(func() {}); !errors.Is(err, ErrClosedExecutor) {
		t.Fatalf("submit after shutdown err=%v", err)
	}
}
