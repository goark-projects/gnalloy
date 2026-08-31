package quic

import (
	"time"

	nativequic "github.com/quic-go/quic-go"
)

type bidirectionalStream struct {
	inner *nativequic.Stream
}

type sendStream struct {
	inner *nativequic.SendStream
}

type receiveStream struct {
	inner *nativequic.ReceiveStream
}

func wrapStream(stream *nativequic.Stream) Stream {
	if stream == nil {
		return nil
	}
	return &bidirectionalStream{inner: stream}
}

func wrapSendStream(stream *nativequic.SendStream) SendStream {
	if stream == nil {
		return nil
	}
	return &sendStream{inner: stream}
}

func wrapReceiveStream(stream *nativequic.ReceiveStream) ReceiveStream {
	if stream == nil {
		return nil
	}
	return &receiveStream{inner: stream}
}

func (s *bidirectionalStream) ID() StreamID {
	if s == nil || s.inner == nil {
		return 0
	}
	return StreamID(s.inner.StreamID())
}

func (s *bidirectionalStream) Read(p []byte) (int, error) {
	if s == nil || s.inner == nil {
		return 0, ErrClosed
	}
	return s.inner.Read(p)
}

func (s *bidirectionalStream) Write(p []byte) (int, error) {
	if s == nil || s.inner == nil {
		return 0, ErrClosed
	}
	return s.inner.Write(p)
}

func (s *bidirectionalStream) Close() error {
	if s == nil || s.inner == nil {
		return ErrClosed
	}
	return s.inner.Close()
}

func (s *bidirectionalStream) SetDeadline(t time.Time) error {
	if s == nil || s.inner == nil {
		return ErrClosed
	}
	return s.inner.SetDeadline(t)
}

func (s *bidirectionalStream) SetReadDeadline(t time.Time) error {
	if s == nil || s.inner == nil {
		return ErrClosed
	}
	return s.inner.SetReadDeadline(t)
}

func (s *bidirectionalStream) SetWriteDeadline(t time.Time) error {
	if s == nil || s.inner == nil {
		return ErrClosed
	}
	return s.inner.SetWriteDeadline(t)
}

func (s *bidirectionalStream) CancelRead(code StreamErrorCode) {
	if s == nil || s.inner == nil {
		return
	}
	s.inner.CancelRead(nativequic.StreamErrorCode(code))
}

func (s *bidirectionalStream) CancelWrite(code StreamErrorCode) {
	if s == nil || s.inner == nil {
		return
	}
	s.inner.CancelWrite(nativequic.StreamErrorCode(code))
}

func (s *sendStream) ID() StreamID {
	if s == nil || s.inner == nil {
		return 0
	}
	return StreamID(s.inner.StreamID())
}

func (s *sendStream) Write(p []byte) (int, error) {
	if s == nil || s.inner == nil {
		return 0, ErrClosed
	}
	return s.inner.Write(p)
}

func (s *sendStream) Close() error {
	if s == nil || s.inner == nil {
		return ErrClosed
	}
	return s.inner.Close()
}

func (s *sendStream) SetWriteDeadline(t time.Time) error {
	if s == nil || s.inner == nil {
		return ErrClosed
	}
	return s.inner.SetWriteDeadline(t)
}

func (s *sendStream) CancelWrite(code StreamErrorCode) {
	if s == nil || s.inner == nil {
		return
	}
	s.inner.CancelWrite(nativequic.StreamErrorCode(code))
}

func (s *receiveStream) ID() StreamID {
	if s == nil || s.inner == nil {
		return 0
	}
	return StreamID(s.inner.StreamID())
}

func (s *receiveStream) Read(p []byte) (int, error) {
	if s == nil || s.inner == nil {
		return 0, ErrClosed
	}
	return s.inner.Read(p)
}

func (s *receiveStream) SetReadDeadline(t time.Time) error {
	if s == nil || s.inner == nil {
		return ErrClosed
	}
	return s.inner.SetReadDeadline(t)
}

func (s *receiveStream) CancelRead(code StreamErrorCode) {
	if s == nil || s.inner == nil {
		return
	}
	s.inner.CancelRead(nativequic.StreamErrorCode(code))
}
