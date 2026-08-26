package executor

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
)

const defaultQueueSize = 1024

// Task 是提交给业务执行器运行的函数。
type Task func()

// Executor 是单个串行业务执行器。
type Executor interface {
	Submit(Task) error
}

// Chooser 表示可选择串行业务执行器的执行器组。
type Chooser interface {
	Next() Executor
}

// Config 描述业务执行器组参数。
type Config struct {
	// Size 是 worker 数量，0 表示使用 runtime.GOMAXPROCS(0)。
	Size int
	// QueueSize 是每个 worker 的有界任务队列大小，0 表示使用默认值。
	QueueSize int
}

// Group 是固定大小的业务执行器组。
type Group struct {
	workers []*worker
	next    atomic.Uint64

	closed atomic.Bool
	once   sync.Once
	done   chan struct{}
}

// NewGroup 创建并启动固定大小的业务执行器组。
func NewGroup(cfg Config) (*Group, error) {
	if cfg.Size < 0 || cfg.QueueSize < 0 {
		return nil, ErrInvalidConfig
	}
	size := cfg.Size
	if size == 0 {
		size = runtime.GOMAXPROCS(0)
	}
	if size <= 0 {
		return nil, ErrInvalidConfig
	}
	queueSize := cfg.QueueSize
	if queueSize == 0 {
		queueSize = defaultQueueSize
	}

	g := &Group{
		workers: make([]*worker, 0, size),
		done:    make(chan struct{}),
	}
	for i := 0; i < size; i++ {
		g.workers = append(g.workers, newWorker(queueSize))
	}
	return g, nil
}

// NewDefaultGroup 创建指定 worker 数的默认业务执行器组。
func NewDefaultGroup(size int) (*Group, error) {
	return NewGroup(Config{Size: size})
}

// Size 返回执行器组 worker 数量。
func (g *Group) Size() int {
	if g == nil {
		return 0
	}
	return len(g.workers)
}

// Next 按 Round-Robin 返回下一个串行业务执行器。
func (g *Group) Next() Executor {
	if g == nil || len(g.workers) == 0 {
		return closedExecutor{}
	}
	idx := g.next.Add(1) - 1
	return g.workers[idx%uint64(len(g.workers))]
}

// Submit 把任务提交到执行器组中的下一个 worker。
func (g *Group) Submit(task Task) error {
	if g == nil || g.closed.Load() {
		return ErrClosedExecutor
	}
	return g.Next().Submit(task)
}

// Shutdown 关闭执行器组，并等待已入队任务执行完成。
func (g *Group) Shutdown(ctx context.Context) error {
	if g == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	g.closed.Store(true)
	g.once.Do(func() {
		for _, w := range g.workers {
			w.closeQueue()
		}
		go func() {
			for _, w := range g.workers {
				w.await()
			}
			close(g.done)
		}()
	})
	select {
	case <-g.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close 关闭执行器组，并等待已入队任务执行完成。
func (g *Group) Close() error {
	return g.Shutdown(context.Background())
}

type worker struct {
	mu     sync.RWMutex
	tasks  chan Task
	closed bool
	done   chan struct{}
}

func newWorker(queueSize int) *worker {
	w := &worker{
		tasks: make(chan Task, queueSize),
		done:  make(chan struct{}),
	}
	go w.run()
	return w
}

func (w *worker) Submit(task Task) error {
	if task == nil {
		return nil
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.closed {
		return ErrClosedExecutor
	}
	select {
	case w.tasks <- task:
		return nil
	default:
		return ErrTaskQueueFull
	}
}

func (w *worker) closeQueue() {
	w.mu.Lock()
	if !w.closed {
		w.closed = true
		close(w.tasks)
	}
	w.mu.Unlock()
}

func (w *worker) await() {
	<-w.done
}

func (w *worker) run() {
	defer close(w.done)
	for task := range w.tasks {
		task()
	}
}

type closedExecutor struct{}

func (closedExecutor) Submit(Task) error {
	return ErrClosedExecutor
}
