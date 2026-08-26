package tls

import (
	"io"
	"net"
	"sync"
	"time"
)

type memoryConn struct {
	in      chan []byte
	out     chan []byte
	closed  chan struct{}
	once    sync.Once
	pending []byte
}

func newMemoryConn() *memoryConn {
	return &memoryConn{
		in:     make(chan []byte, 32),
		out:    make(chan []byte, 32),
		closed: make(chan struct{}),
	}
}

func (c *memoryConn) feed(src []byte) error {
	if len(src) == 0 {
		return nil
	}
	return c.feedOwned(append([]byte(nil), src...))
}

func (c *memoryConn) feedOwned(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	select {
	case c.in <- data:
		return nil
	case <-c.closed:
		return io.ErrClosedPipe
	}
}

func (c *memoryConn) Read(dst []byte) (int, error) {
	for len(c.pending) == 0 {
		select {
		case data := <-c.in:
			c.pending = data
		case <-c.closed:
			return 0, io.EOF
		}
	}
	n := copy(dst, c.pending)
	c.pending = c.pending[n:]
	return n, nil
}

func (c *memoryConn) Write(src []byte) (int, error) {
	if len(src) == 0 {
		return 0, nil
	}
	data := append([]byte(nil), src...)
	select {
	case c.out <- data:
		return len(src), nil
	case <-c.closed:
		return 0, io.ErrClosedPipe
	}
}

func (c *memoryConn) Close() error {
	c.once.Do(func() {
		close(c.closed)
	})
	return nil
}

func (c *memoryConn) LocalAddr() net.Addr              { return memoryAddr("local") }
func (c *memoryConn) RemoteAddr() net.Addr             { return memoryAddr("remote") }
func (c *memoryConn) SetDeadline(time.Time) error      { return nil }
func (c *memoryConn) SetReadDeadline(time.Time) error  { return nil }
func (c *memoryConn) SetWriteDeadline(time.Time) error { return nil }

type memoryAddr string

func (a memoryAddr) Network() string { return "memory" }
func (a memoryAddr) String() string  { return string(a) }
