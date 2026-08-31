package quic

import (
	"sync"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/internal/message"
	"goark.dev/gnalloy/transport"
)

type streamSink struct {
	mu     sync.Mutex
	stream Stream
	closed bool
}

func newStreamSink(stream Stream) *streamSink {
	return &streamSink{stream: stream}
}

func (s *streamSink) Write(msg any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		releaseStreamMessage(msg)
		return ErrClosed
	}
	buf, ok := msg.(buffer.ByteBuf)
	if !ok || buf == nil {
		releaseStreamMessage(msg)
		return ErrWriteUnsupported
	}
	defer buf.Release()
	var stack [8][]byte
	for _, segment := range buf.ReadableSlices(stack[:0]) {
		if err := writeStreamAll(s.stream, segment); err != nil {
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
	return nil
}

func (s *streamSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.stream == nil {
		return nil
	}
	return s.stream.Close()
}

func (s *streamSink) IsWritable() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed
}

func (s *streamSink) PendingOutboundBytes() int64 {
	return 0
}

func (s *streamSink) WriteBufferWatermark() transport.WriteBufferWatermark {
	return transport.DefaultWriteBufferWatermark()
}

func writeStreamAll(stream Stream, data []byte) error {
	for len(data) > 0 {
		n, err := stream.Write(data)
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

func releaseStreamMessage(msg any) {
	message.Release(msg)
}
