package http2

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
)

const defaultInitialWindowSize int32 = 65535

// StreamEventType 描述 HTTP/2 stream 在 multiplexer 中产生的事件类型。
type StreamEventType uint8

const (
	StreamEventActive StreamEventType = iota + 1
	StreamEventRead
	StreamEventClosed
	StreamEventWindowUpdated
)

// StreamEvent 是 HTTP/2 多路复用层向业务 handler 暴露的 stream 事件。
type StreamEvent struct {
	Type     StreamEventType
	StreamID StreamID
	State    StreamState
	Frame    TypedFrame
}

// Release 释放事件持有的 frame，保持 pipeline 尾部释放语义一致。
func (e StreamEvent) Release() {
	if e.Frame != nil {
		e.Frame.Release()
	}
}

// MultiplexerConfig 描述 HTTP/2 stream 复用层的本地端角色和流控初始值。
type MultiplexerConfig struct {
	// Server 表示本端是服务端；用于校验本端主动发起 stream 的奇偶性。
	Server bool
	// InitialConnectionWindow 是连接级出站窗口，0 表示 RFC 7540 默认值。
	InitialConnectionWindow int32
	// InitialStreamWindow 是 stream 级出站窗口，0 表示 RFC 7540 默认值。
	InitialStreamWindow int32
	// MaxActiveStreams 限制活跃 stream 数，0 表示不额外限制。
	MaxActiveStreams int
}

// StreamMultiplexer 维护 HTTP/2 stream 生命周期和基础 flow-control。
//
// 该 handler 对齐 Netty Http2MultiplexHandler 的核心职责：把连接级 typed frame
// 转换为 stream 事件，并在出站路径校验 stream 状态与窗口。HeaderBlock 的 HPACK
// 编解码保持在 codec 边界之外，由上层或后续 header codec 负责。
type StreamMultiplexer struct {
	cfg              MultiplexerConfig
	initialConnWin   int32
	initialStreamWin int32
	connSendWindow   int32
	streams          map[StreamID]*multiplexedStream
}

type multiplexedStream struct {
	id         StreamID
	state      StreamState
	sendWindow int32
	recvWindow int32
}

// NewStreamMultiplexer 创建 HTTP/2 stream multiplexer。
func NewStreamMultiplexer(cfg MultiplexerConfig) (*StreamMultiplexer, error) {
	connWin := cfg.InitialConnectionWindow
	if connWin == 0 {
		connWin = defaultInitialWindowSize
	}
	streamWin := cfg.InitialStreamWindow
	if streamWin == 0 {
		streamWin = defaultInitialWindowSize
	}
	if connWin < 0 || streamWin < 0 || cfg.MaxActiveStreams < 0 {
		return nil, ErrFlowControl
	}
	return &StreamMultiplexer{
		cfg:              cfg,
		initialConnWin:   connWin,
		initialStreamWin: streamWin,
		connSendWindow:   connWin,
		streams:          make(map[StreamID]*multiplexedStream, 16),
	}, nil
}

// ChannelRead 将连接级 typed frame 映射为 stream 事件。
func (m *StreamMultiplexer) ChannelRead(ctx *channel.HandlerContext, msg any) {
	switch frame := msg.(type) {
	case HeadersFrame:
		m.readHeaders(ctx, frame)
	case HeadersBlock:
		m.readHeadersBlock(ctx, frame)
	case DataFrame:
		m.readData(ctx, frame)
	case RSTStreamFrame:
		m.readRSTStream(ctx, frame)
	case WindowUpdateFrame:
		m.readWindowUpdate(ctx, frame)
	case GoAwayFrame:
		m.readGoAway(ctx, frame)
	default:
		ctx.FireChannelRead(msg)
	}
}

// Write 在出站路径维护本地 stream 状态并执行基础 flow-control 校验。
func (m *StreamMultiplexer) Write(ctx *channel.HandlerContext, msg any) error {
	switch frame := msg.(type) {
	case HeadersFrame:
		if err := m.writeHeaders(frame); err != nil {
			frame.Release()
			return err
		}
	case HeadersBlock:
		if err := m.writeHeadersBlock(frame); err != nil {
			return err
		}
	case DataFrame:
		if err := m.writeData(frame); err != nil {
			frame.Release()
			return err
		}
	case RSTStreamFrame:
		m.closeStream(frame.StreamID)
	case WindowUpdateFrame:
		m.applyInboundWindowUpdate(frame)
	case GoAwayFrame:
		m.closeStreamsAfter(frame.LastStreamID)
	}
	return ctx.Write(msg)
}

// ActiveStreams 返回当前仍未关闭的 stream 数量。
func (m *StreamMultiplexer) ActiveStreams() int {
	return len(m.streams)
}

func (m *StreamMultiplexer) readHeaders(ctx *channel.HandlerContext, frame HeadersFrame) {
	m.readHeaderState(ctx, frame.StreamID, frame.Flags&FlagEndStream != 0, frame)
}

func (m *StreamMultiplexer) readHeadersBlock(ctx *channel.HandlerContext, frame HeadersBlock) {
	m.readHeaderState(ctx, frame.StreamID, frame.EndStream, frame)
}

func (m *StreamMultiplexer) readHeaderState(ctx *channel.HandlerContext, streamID StreamID, endStream bool, frame TypedFrame) {
	stream, created, err := m.stream(streamID, false)
	if err != nil {
		frame.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	if err := stream.openRemote(endStream); err != nil {
		frame.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	if created {
		ctx.FireChannelRead(StreamEvent{Type: StreamEventActive, StreamID: stream.id, State: stream.state})
	}
	ctx.FireChannelRead(StreamEvent{Type: StreamEventRead, StreamID: stream.id, State: stream.state, Frame: frame})
	m.fireClosedIfNeeded(ctx, stream)
}

func (m *StreamMultiplexer) readData(ctx *channel.HandlerContext, frame DataFrame) {
	stream := m.streams[frame.StreamID]
	if stream == nil {
		frame.Release()
		ctx.FireExceptionCaught(ErrInvalidStreamState)
		return
	}
	size := readableBytes(frame.Data)
	if size > int(stream.recvWindow) {
		frame.Release()
		ctx.FireExceptionCaught(ErrFlowControl)
		return
	}
	stream.recvWindow -= int32(size)
	if frame.Flags&FlagEndStream != 0 {
		if err := stream.halfCloseRemote(); err != nil {
			frame.Release()
			ctx.FireExceptionCaught(err)
			return
		}
	}
	ctx.FireChannelRead(StreamEvent{Type: StreamEventRead, StreamID: stream.id, State: stream.state, Frame: frame})
	m.fireClosedIfNeeded(ctx, stream)
}

func (m *StreamMultiplexer) readRSTStream(ctx *channel.HandlerContext, frame RSTStreamFrame) {
	stream := m.streams[frame.StreamID]
	if stream == nil {
		ctx.FireExceptionCaught(ErrInvalidStreamState)
		return
	}
	stream.state = StreamClosed
	ctx.FireChannelRead(StreamEvent{Type: StreamEventRead, StreamID: stream.id, State: stream.state, Frame: frame})
	m.fireClosedIfNeeded(ctx, stream)
}

func (m *StreamMultiplexer) readWindowUpdate(ctx *channel.HandlerContext, frame WindowUpdateFrame) {
	if frame.StreamID == 0 {
		if !addWindow(&m.connSendWindow, frame.Increment) {
			ctx.FireExceptionCaught(ErrFlowControl)
			return
		}
		ctx.FireChannelRead(StreamEvent{Type: StreamEventWindowUpdated, State: StreamOpen, Frame: frame})
		return
	}
	stream := m.streams[frame.StreamID]
	if stream == nil || !addWindow(&stream.sendWindow, frame.Increment) {
		ctx.FireExceptionCaught(ErrFlowControl)
		return
	}
	ctx.FireChannelRead(StreamEvent{Type: StreamEventWindowUpdated, StreamID: stream.id, State: stream.state, Frame: frame})
}

func (m *StreamMultiplexer) readGoAway(ctx *channel.HandlerContext, frame GoAwayFrame) {
	m.closeStreamsAfter(frame.LastStreamID)
	ctx.FireChannelRead(frame)
}

func (m *StreamMultiplexer) writeHeaders(frame HeadersFrame) error {
	stream, _, err := m.stream(frame.StreamID, true)
	if err != nil {
		return err
	}
	if err := stream.openLocal(frame.Flags&FlagEndStream != 0); err != nil {
		return err
	}
	if stream.state == StreamClosed {
		m.closeStream(frame.StreamID)
	}
	return nil
}

func (m *StreamMultiplexer) writeHeadersBlock(frame HeadersBlock) error {
	stream, _, err := m.stream(frame.StreamID, true)
	if err != nil {
		return err
	}
	if err := stream.openLocal(frame.EndStream); err != nil {
		return err
	}
	if stream.state == StreamClosed {
		m.closeStream(frame.StreamID)
	}
	return nil
}

func (m *StreamMultiplexer) writeData(frame DataFrame) error {
	stream := m.streams[frame.StreamID]
	if stream == nil || !stream.canWriteData() {
		return ErrInvalidStreamState
	}
	size := readableBytes(frame.Data)
	if size > int(m.connSendWindow) || size > int(stream.sendWindow) {
		return ErrFlowControl
	}
	m.connSendWindow -= int32(size)
	stream.sendWindow -= int32(size)
	if frame.Flags&FlagEndStream != 0 {
		if err := stream.halfCloseLocal(); err != nil {
			return err
		}
		if stream.state == StreamClosed {
			m.closeStream(frame.StreamID)
		}
	}
	return nil
}

func (m *StreamMultiplexer) applyInboundWindowUpdate(frame WindowUpdateFrame) {
	if frame.StreamID == 0 {
		_ = addWindow(&m.connSendWindow, frame.Increment)
		return
	}
	if stream := m.streams[frame.StreamID]; stream != nil {
		_ = addWindow(&stream.recvWindow, frame.Increment)
	}
}

func (m *StreamMultiplexer) stream(id StreamID, local bool) (*multiplexedStream, bool, error) {
	if !id.Valid() {
		return nil, false, ErrInvalidStreamID
	}
	if stream := m.streams[id]; stream != nil {
		return stream, false, nil
	}
	if !m.validInitiator(id, local) {
		return nil, false, ErrInvalidStreamID
	}
	if m.cfg.MaxActiveStreams > 0 && len(m.streams) >= m.cfg.MaxActiveStreams {
		return nil, false, ErrInvalidStreamState
	}
	stream := &multiplexedStream{
		id:         id,
		state:      StreamIdle,
		sendWindow: m.initialStreamWin,
		recvWindow: m.initialStreamWin,
	}
	m.streams[id] = stream
	return stream, true, nil
}

func (m *StreamMultiplexer) validInitiator(id StreamID, local bool) bool {
	if local {
		if m.cfg.Server {
			return id.ServerInitiated()
		}
		return id.ClientInitiated()
	}
	if m.cfg.Server {
		return id.ClientInitiated()
	}
	return id.ServerInitiated()
}

func (m *StreamMultiplexer) fireClosedIfNeeded(ctx *channel.HandlerContext, stream *multiplexedStream) {
	if stream.state != StreamClosed {
		return
	}
	delete(m.streams, stream.id)
	ctx.FireChannelRead(StreamEvent{Type: StreamEventClosed, StreamID: stream.id, State: StreamClosed})
}

func (m *StreamMultiplexer) closeStream(id StreamID) {
	delete(m.streams, id)
}

func (m *StreamMultiplexer) closeStreamsAfter(last StreamID) {
	for id := range m.streams {
		if id > last {
			delete(m.streams, id)
		}
	}
}

func (s *multiplexedStream) openLocal(endStream bool) error {
	switch s.state {
	case StreamIdle:
		if endStream {
			s.state = StreamHalfClosedLocal
			return nil
		}
		s.state = StreamOpen
		return nil
	case StreamOpen:
		if endStream {
			s.state = StreamHalfClosedLocal
			return nil
		}
		return nil
	case StreamHalfClosedRemote:
		if endStream {
			s.state = StreamClosed
		}
		return nil
	default:
		return ErrInvalidStreamState
	}
}

func (s *multiplexedStream) openRemote(endStream bool) error {
	switch s.state {
	case StreamIdle:
		if endStream {
			s.state = StreamHalfClosedRemote
			return nil
		}
		s.state = StreamOpen
		return nil
	case StreamOpen:
		if endStream {
			s.state = StreamHalfClosedRemote
			return nil
		}
		return nil
	case StreamHalfClosedLocal:
		if endStream {
			s.state = StreamClosed
		}
		return nil
	default:
		return ErrInvalidStreamState
	}
}

func (s *multiplexedStream) halfCloseLocal() error {
	return s.openLocal(true)
}

func (s *multiplexedStream) halfCloseRemote() error {
	return s.openRemote(true)
}

func (s *multiplexedStream) canWriteData() bool {
	return s.state == StreamOpen || s.state == StreamHalfClosedRemote
}

func addWindow(window *int32, increment uint32) bool {
	if increment == 0 || increment > uint32(maxStreamID) {
		return false
	}
	if *window > int32(maxStreamID)-int32(increment) {
		return false
	}
	*window += int32(increment)
	return true
}

func readableBytes(buf buffer.ByteBuf) int {
	if buf == nil {
		return 0
	}
	return buf.ReadableBytes()
}
