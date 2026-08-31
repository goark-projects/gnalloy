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
	stats  *sessionStats
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
	written := 0
	for _, segment := range buf.ReadableSlices(stack[:0]) {
		n, err := writeAll(s.writer, segment)
		written += n
		if err != nil {
			s.stats.recordWriteBytes(written)
			return err
		}
	}
	s.stats.recordWriteBytes(written)
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

func writeAll(writer streamWriter, data []byte) (int, error) {
	written := 0
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return written, err
		}
		if n <= 0 {
			return written, ErrWriteUnsupported
		}
		written += n
		data = data[n:]
	}
	return written, nil
}

func releaseMessage(msg any) {
	message.Release(msg)
}
