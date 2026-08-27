package tls

import (
	"io"
	"net"
	"sync"
	"time"
)

type memoryConn struct {
	in      chan []byte
	out     chan byteChunk
	closed  chan struct{}
	once    sync.Once
	pool    BytePool
	notify  func()
	pending byteChunk
}

func newMemoryConn(pool BytePool, notify func()) *memoryConn {
	return &memoryConn{
		in:     make(chan []byte, 32),
		out:    make(chan byteChunk, 32),
		closed: make(chan struct{}),
		pool:   normalizeBytePool(pool),
		notify: notify,
	}
}

func (c *memoryConn) feed(src []byte) error {
	if len(src) == 0 {
		return nil
	}
	return c.feedOwned(copyBytes(src, c.pool))
}

func (c *memoryConn) feedOwned(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	chunk := newByteChunk(data, c.pool)
	select {
	case c.in <- chunk.data:
		return nil
	case <-c.closed:
		chunk.releaseOwned()
		return io.ErrClosedPipe
	}
}

func (c *memoryConn) Read(dst []byte) (int, error) {
	for len(c.pending.data) == 0 {
		select {
		case data := <-c.in:
			c.pending = newByteChunk(data, c.pool)
		case <-c.closed:
			return 0, io.EOF
		}
	}
	n := copy(dst, c.pending.data)
	c.pending.data = c.pending.data[n:]
	if len(c.pending.data) == 0 {
		c.pending.releaseOwned()
	}
	return n, nil
}

func (c *memoryConn) Write(src []byte) (int, error) {
	if len(src) == 0 {
		return 0, nil
	}
	chunk := newByteChunk(copyBytes(src, c.pool), c.pool)
	select {
	case c.out <- chunk:
		c.notifyDrain()
		return len(src), nil
	case <-c.closed:
		chunk.releaseOwned()
		return 0, io.ErrClosedPipe
	}
}

func (c *memoryConn) Close() error {
	c.once.Do(func() {
		close(c.closed)
		c.drainInput()
	})
	return nil
}

func (c *memoryConn) drainInput() {
	for {
		select {
		case data := <-c.in:
			chunk := newByteChunk(data, c.pool)
			chunk.releaseOwned()
		default:
			return
		}
	}
}

func (c *memoryConn) LocalAddr() net.Addr              { return memoryAddr("local") }
func (c *memoryConn) RemoteAddr() net.Addr             { return memoryAddr("remote") }
func (c *memoryConn) SetDeadline(time.Time) error      { return nil }
func (c *memoryConn) SetReadDeadline(time.Time) error  { return nil }
func (c *memoryConn) SetWriteDeadline(time.Time) error { return nil }

func (c *memoryConn) notifyDrain() {
	if c.notify != nil {
		c.notify()
	}
}

type memoryAddr string

func (a memoryAddr) Network() string { return "memory" }
func (a memoryAddr) String() string  { return string(a) }
