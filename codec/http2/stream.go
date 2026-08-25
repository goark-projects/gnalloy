package http2

const maxStreamID StreamID = 1<<31 - 1

type StreamID uint32

func (id StreamID) Valid() bool {
	return id > 0 && id <= maxStreamID
}

func (id StreamID) ClientInitiated() bool {
	return id.Valid() && id%2 == 1
}

func (id StreamID) ServerInitiated() bool {
	return id.Valid() && id%2 == 0
}

type StreamState uint8

const (
	StreamIdle StreamState = iota
	StreamOpen
	StreamHalfClosedLocal
	StreamHalfClosedRemote
	StreamClosed
)

type Stream struct {
	ID    StreamID
	State StreamState
}

func NewStream(id StreamID) Stream {
	return Stream{ID: id, State: StreamIdle}
}

func (s *Stream) Open(endStream bool) error {
	if !s.ID.Valid() {
		return ErrInvalidStreamID
	}
	if s.State != StreamIdle {
		return ErrInvalidStreamState
	}
	if endStream {
		s.State = StreamHalfClosedLocal
		return nil
	}
	s.State = StreamOpen
	return nil
}

func (s *Stream) HalfCloseLocal() error {
	switch s.State {
	case StreamOpen:
		s.State = StreamHalfClosedLocal
		return nil
	case StreamHalfClosedRemote:
		s.State = StreamClosed
		return nil
	default:
		return ErrInvalidStreamState
	}
}

func (s *Stream) HalfCloseRemote() error {
	switch s.State {
	case StreamOpen:
		s.State = StreamHalfClosedRemote
		return nil
	case StreamHalfClosedLocal:
		s.State = StreamClosed
		return nil
	default:
		return ErrInvalidStreamState
	}
}

func (s *Stream) Close() error {
	if s.State == StreamClosed {
		return nil
	}
	if s.State == StreamIdle {
		return ErrInvalidStreamState
	}
	s.State = StreamClosed
	return nil
}
