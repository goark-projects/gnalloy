package http3

import (
	"sync"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/internal/message"
)

type streamSink struct {
	mu     sync.Mutex
	writer streamWriter
	closed bool
}

func (s *streamSink) Write(msg any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		releaseMessage(msg)
		return ErrClosed
	}
	if s.writer == nil {
		releaseMessage(msg)
		return ErrWriteUnsupported
	}
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		releaseMessage(msg)
		return ErrWriteUnsupported
	}
	defer buf.Release()
	var stack [8][]byte
	for _, segment := range buf.ReadableSlices(stack[:0]) {
		if err := writeAll(s.writer, segment); err != nil {
			return err
		}
	}
	return nil
}

func (s *streamSink) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if flusher, ok := s.writer.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}
	return nil
}

func (s *streamSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.writer == nil {
		return nil
	}
	return s.writer.Close()
}

func writeAll(writer streamWriter, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return ErrWriteUnsupported
		}
		data = data[n:]
	}
	return nil
}

func releaseMessage(msg any) {
	message.Release(msg)
}
