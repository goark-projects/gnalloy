package channel

import "sync"

// Future 表示异步 Channel 操作的最终结果。
type Future interface {
	Done() <-chan struct{}
	IsDone() bool
	Err() error
	Await() error
	AddListener(func(Future)) Future
}

// Promise 是框架内部可完成的 Future。
type Promise interface {
	Future
	SetSuccess() bool
	SetFailure(error) bool
}

// DefaultPromise 是无外部依赖的 Future/Promise 实现。
type DefaultPromise struct {
	done      chan struct{}
	once      sync.Once
	mu        sync.Mutex
	err       error
	completed bool
	listeners []func(Future)
}

func NewPromise() *DefaultPromise {
	return &DefaultPromise{done: make(chan struct{})}
}

func SucceededFuture() Future {
	p := NewPromise()
	p.SetSuccess()
	return p
}

func FailedFuture(err error) Future {
	p := NewPromise()
	p.SetFailure(err)
	return p
}

func (p *DefaultPromise) Done() <-chan struct{} {
	return p.done
}

func (p *DefaultPromise) IsDone() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func (p *DefaultPromise) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

func (p *DefaultPromise) Await() error {
	<-p.done
	return p.Err()
}

func (p *DefaultPromise) AddListener(listener func(Future)) Future {
	if listener == nil {
		return p
	}
	callNow := false
	p.mu.Lock()
	if p.completed {
		callNow = true
	} else {
		p.listeners = append(p.listeners, listener)
	}
	p.mu.Unlock()
	if callNow {
		listener(p)
	}
	return p
}

func (p *DefaultPromise) SetSuccess() bool {
	return p.complete(nil)
}

func (p *DefaultPromise) SetFailure(err error) bool {
	if err == nil {
		err = ErrPromiseFailed
	}
	return p.complete(err)
}

func (p *DefaultPromise) complete(err error) bool {
	completed := false
	p.once.Do(func() {
		p.mu.Lock()
		p.err = err
		p.completed = true
		listeners := append([]func(Future){}, p.listeners...)
		p.listeners = nil
		p.mu.Unlock()

		close(p.done)
		for _, listener := range listeners {
			listener(p)
		}
		completed = true
	})
	return completed
}
