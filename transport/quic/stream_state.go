package quic

const defaultInitialStreamWindow uint64 = 65535

// StreamState 描述 QUIC stream 的本地生命周期。
type StreamState uint8

const (
	StreamStateOpen StreamState = iota + 1
	StreamStateHalfClosedLocal
	StreamStateHalfClosedRemote
	StreamStateClosed
)

// Stream 表示 QUIC stream 的轻量状态，不持有应用层 payload。
type Stream struct {
	ID            uint64
	State         StreamState
	SendWindow    uint64
	ReceiveWindow uint64
	SendOffset    uint64
	ReceiveOffset uint64
	FinReceived   bool
	FinSent       bool
}

// StreamManager 维护连接内所有 QUIC stream 的状态。
type StreamManager struct {
	initialSendWindow uint64
	initialRecvWindow uint64
	streams           map[uint64]*Stream
}

// NewStreamManager 创建 stream 管理器。
func NewStreamManager(sendWindow uint64, receiveWindow uint64) *StreamManager {
	if sendWindow == 0 {
		sendWindow = defaultInitialStreamWindow
	}
	if receiveWindow == 0 {
		receiveWindow = defaultInitialStreamWindow
	}
	return &StreamManager{
		initialSendWindow: sendWindow,
		initialRecvWindow: receiveWindow,
		streams:           make(map[uint64]*Stream, 16),
	}
}

// Get 返回 stream 状态。
func (m *StreamManager) Get(streamID uint64) (*Stream, bool) {
	if m == nil {
		return nil, false
	}
	stream, ok := m.streams[streamID]
	return stream, ok
}

// Open 获取或创建 stream。
func (m *StreamManager) Open(streamID uint64) *Stream {
	stream, ok := m.Get(streamID)
	if ok {
		return stream
	}
	stream = &Stream{
		ID:            streamID,
		State:         StreamStateOpen,
		SendWindow:    m.initialSendWindow,
		ReceiveWindow: m.initialRecvWindow,
	}
	m.streams[streamID] = stream
	return stream
}

// Receive 应用入站 STREAM frame，并执行 receive window 校验。
func (m *StreamManager) Receive(frame StreamFrame) (*Stream, error) {
	if m == nil {
		return nil, ErrInvalidStreamState
	}
	stream := m.Open(frame.StreamID)
	if stream.State == StreamStateHalfClosedRemote || stream.State == StreamStateClosed {
		return nil, ErrInvalidStreamState
	}
	size := uint64(readableFrameBytes(frame.Data))
	if ^uint64(0)-frame.Offset < size {
		return nil, ErrFlowControl
	}
	end := frame.Offset + size
	if end > stream.ReceiveWindow {
		return nil, ErrFlowControl
	}
	if end > stream.ReceiveOffset {
		stream.ReceiveOffset = end
	}
	if frame.Fin {
		stream.FinReceived = true
		stream.halfCloseRemote()
	}
	return stream, nil
}

// ReserveSend 为出站 STREAM frame 预留 send window。
func (m *StreamManager) ReserveSend(streamID uint64, bytes int, fin bool) (*Stream, error) {
	if m == nil || bytes < 0 {
		return nil, ErrInvalidStreamState
	}
	stream := m.Open(streamID)
	if stream.State == StreamStateHalfClosedLocal || stream.State == StreamStateClosed {
		return nil, ErrInvalidStreamState
	}
	if uint64(bytes) > stream.SendWindow {
		return nil, ErrFlowControl
	}
	stream.SendWindow -= uint64(bytes)
	stream.SendOffset += uint64(bytes)
	if fin {
		stream.FinSent = true
		stream.halfCloseLocal()
	}
	return stream, nil
}

// AddSendWindow 增加指定 stream 的发送窗口。
func (m *StreamManager) AddSendWindow(streamID uint64, increment uint64) error {
	if m == nil || increment == 0 {
		return ErrFlowControl
	}
	stream := m.Open(streamID)
	if ^uint64(0)-stream.SendWindow < increment {
		return ErrFlowControl
	}
	stream.SendWindow += increment
	return nil
}

func (m *StreamManager) Len() int {
	if m == nil {
		return 0
	}
	return len(m.streams)
}

func (s *Stream) halfCloseLocal() {
	switch s.State {
	case StreamStateOpen:
		s.State = StreamStateHalfClosedLocal
	case StreamStateHalfClosedRemote:
		s.State = StreamStateClosed
	}
}

func (s *Stream) halfCloseRemote() {
	switch s.State {
	case StreamStateOpen:
		s.State = StreamStateHalfClosedRemote
	case StreamStateHalfClosedLocal:
		s.State = StreamStateClosed
	}
}

func readableFrameBytes(data interface{ ReadableBytes() int }) int {
	if data == nil {
		return 0
	}
	return data.ReadableBytes()
}
