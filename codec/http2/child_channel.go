package http2

import (
	"sync"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/internal/message"
	"goark.dev/gnalloy/transport"
)

const (
	// ErrorCodeCancel 是 RFC 7540 定义的 CANCEL，用于本地主动关闭单个 stream。
	ErrorCodeCancel uint32 = 0x8
)

// StreamChildInitializer 初始化 HTTP/2 child channel 的 Pipeline。
type StreamChildInitializer func(ch *StreamChannel) error

// StreamChildConfig 描述 HTTP/2 child-channel handler 的创建策略。
type StreamChildConfig struct {
	// Initializer 在每个新 stream child channel 注册前安装业务 handler。
	Initializer StreamChildInitializer
	// Allocator 覆盖 child channel allocator；nil 表示复用父连接 allocator。
	Allocator buffer.Allocator
	// MaxChildren 限制并发 child channel 数量，0 表示不额外限制。
	MaxChildren int
}

// StreamChildHandler 把连接级 StreamEvent 映射为每个 stream 独立的 child channel。
//
// 该 handler 对齐 Netty Http2MultiplexHandler 的使用体验：业务代码安装到
// StreamChannel 的 Pipeline 上，入站只处理当前 stream 的 frame；出站写入会自动绑定
// 当前 stream id，并交回父连接的 HTTP/2 multiplexer 执行状态与流控校验。
type StreamChildHandler struct {
	cfg      StreamChildConfig
	children map[StreamID]*StreamChannel
}

// StreamChannel 是 HTTP/2 单个 stream 的轻量 Channel 视图。
type StreamChannel struct {
	*channel.LocalChannel

	parent       channel.Channel
	streamID     StreamID
	state        StreamState
	sink         *streamChildSink
	inactiveOnce sync.Once
}

type streamChildSink struct {
	parentCtx *channel.HandlerContext
	streamID  StreamID
	mu        sync.Mutex
	closed    bool
	remote    bool
}

// NewStreamChildHandler 创建 HTTP/2 child-channel handler。
func NewStreamChildHandler(cfg StreamChildConfig) (*StreamChildHandler, error) {
	if cfg.Initializer == nil {
		return nil, ErrMissingChildInit
	}
	if cfg.MaxChildren < 0 {
		return nil, ErrInvalidStreamState
	}
	return &StreamChildHandler{cfg: cfg, children: make(map[StreamID]*StreamChannel, 16)}, nil
}

// ChannelRead 消费 StreamEvent 并把 stream frame 分发到对应 child channel。
func (h *StreamChildHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	event, ok := msg.(StreamEvent)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	h.readEvent(ctx, event)
}

// ChannelInactive 在父连接关闭时关闭全部 child channel。
func (h *StreamChildHandler) ChannelInactive(ctx *channel.HandlerContext) {
	for id, child := range h.children {
		delete(h.children, id)
		child.closeFromParent(StreamClosed)
	}
	ctx.FireChannelInactive()
}

// Child 返回当前仍活跃的 stream child channel。
func (h *StreamChildHandler) Child(id StreamID) (*StreamChannel, bool) {
	child, ok := h.children[id]
	return child, ok
}

// ActiveChildren 返回当前仍活跃的 child channel 数量。
func (h *StreamChildHandler) ActiveChildren() int {
	return len(h.children)
}

// StreamID 返回该 child channel 对应的 HTTP/2 stream id。
func (c *StreamChannel) StreamID() StreamID {
	return c.streamID
}

// Parent 返回承载该 stream 的父连接 Channel。
func (c *StreamChannel) Parent() channel.Channel {
	return c.parent
}

// State 返回 StreamMultiplexer 最近同步到 child channel 的 stream 状态。
func (c *StreamChannel) State() StreamState {
	return c.state
}

// Close 主动关闭 child channel，并通过父连接写出 RST_STREAM。
func (c *StreamChannel) Close() error {
	if c == nil {
		return ErrChildClosed
	}
	err := c.LocalChannel.Close()
	c.fireInactive()
	return err
}

func (h *StreamChildHandler) readEvent(ctx *channel.HandlerContext, event StreamEvent) {
	switch event.Type {
	case StreamEventActive:
		child, err := h.ensureChild(ctx, event.StreamID, event.State)
		if err != nil {
			event.Release()
			ctx.FireExceptionCaught(err)
			return
		}
		child.state = event.State
	case StreamEventRead, StreamEventWindowUpdated:
		if event.StreamID == 0 {
			ctx.FireChannelRead(event)
			return
		}
		child, err := h.ensureChild(ctx, event.StreamID, event.State)
		if err != nil {
			event.Release()
			ctx.FireExceptionCaught(err)
			return
		}
		child.state = event.State
		if event.Frame != nil {
			child.Pipeline().FireChannelRead(event.Frame)
			child.Pipeline().FireChannelReadComplete()
		}
	case StreamEventClosed:
		child := h.children[event.StreamID]
		if child == nil {
			event.Release()
			return
		}
		delete(h.children, event.StreamID)
		child.closeFromParent(event.State)
	default:
		ctx.FireChannelRead(event)
	}
}

func (h *StreamChildHandler) ensureChild(ctx *channel.HandlerContext, streamID StreamID, state StreamState) (*StreamChannel, error) {
	if !streamID.Valid() {
		return nil, ErrInvalidStreamID
	}
	if child := h.children[streamID]; child != nil {
		child.state = state
		return child, nil
	}
	if h.cfg.MaxChildren > 0 && len(h.children) >= h.cfg.MaxChildren {
		return nil, ErrInvalidStreamState
	}
	allocator := h.cfg.Allocator
	if allocator == nil {
		allocator = ctx.Channel().Allocator()
	}
	sink := &streamChildSink{parentCtx: ctx, streamID: streamID}
	child := &StreamChannel{
		parent:   ctx.Channel(),
		streamID: streamID,
		state:    state,
		sink:     sink,
	}
	child.LocalChannel = channel.NewLocalChannel(streamChildChannelID(ctx.Channel().ID(), streamID), allocator, sink)
	h.children[streamID] = child
	if err := h.cfg.Initializer(child); err != nil {
		delete(h.children, streamID)
		child.closeFromParent(StreamClosed)
		return nil, err
	}
	child.Pipeline().FireChannelRegistered()
	child.Pipeline().FireChannelActive()
	return child, nil
}

func (c *StreamChannel) closeFromParent(state StreamState) {
	c.state = state
	c.sink.closeFromParent()
	c.fireInactive()
}

func (c *StreamChannel) fireInactive() {
	c.inactiveOnce.Do(func() {
		c.Pipeline().FireChannelInactive()
		c.Pipeline().FireChannelUnregistered()
	})
}

func (s *streamChildSink) Write(msg any) error {
	if s.isClosed() {
		releaseChildMessage(msg)
		return ErrChildClosed
	}
	streamMsg, err := s.bindStream(msg)
	if err != nil {
		releaseChildMessage(msg)
		return err
	}
	return s.parentCtx.Write(streamMsg)
}

func (s *streamChildSink) Flush() error {
	if s.isClosed() {
		return ErrChildClosed
	}
	return s.parentCtx.Flush()
}

func (s *streamChildSink) Close() error {
	if !s.markClosed(false) {
		return nil
	}
	if s.remote {
		return nil
	}
	return s.parentCtx.Write(RSTStreamFrame{StreamID: s.streamID, ErrorCode: ErrorCodeCancel})
}

func (s *streamChildSink) bindStream(msg any) (any, error) {
	switch frame := msg.(type) {
	case buffer.ByteBuf:
		return DataFrame{StreamID: s.streamID, Data: frame}, nil
	case DataFrame:
		frame.StreamID = s.validStreamID(frame.StreamID)
		if !frame.StreamID.Valid() {
			return nil, ErrInvalidStreamID
		}
		return frame, nil
	case HeadersFrame:
		frame.StreamID = s.validStreamID(frame.StreamID)
		if !frame.StreamID.Valid() {
			return nil, ErrInvalidStreamID
		}
		return frame, nil
	case HeadersBlock:
		frame.StreamID = s.validStreamID(frame.StreamID)
		if !frame.StreamID.Valid() {
			return nil, ErrInvalidStreamID
		}
		return frame, nil
	case PushPromiseFrame:
		frame.StreamID = s.validStreamID(frame.StreamID)
		if !frame.StreamID.Valid() {
			return nil, ErrInvalidStreamID
		}
		return frame, nil
	case PushPromiseBlock:
		frame.StreamID = s.validStreamID(frame.StreamID)
		if !frame.StreamID.Valid() {
			return nil, ErrInvalidStreamID
		}
		return frame, nil
	case ContinuationFrame:
		frame.StreamID = s.validStreamID(frame.StreamID)
		if !frame.StreamID.Valid() {
			return nil, ErrInvalidStreamID
		}
		return frame, nil
	case RSTStreamFrame:
		frame.StreamID = s.validStreamID(frame.StreamID)
		if !frame.StreamID.Valid() {
			return nil, ErrInvalidStreamID
		}
		return frame, nil
	case WindowUpdateFrame:
		frame.StreamID = s.validStreamID(frame.StreamID)
		if !frame.StreamID.Valid() {
			return nil, ErrInvalidStreamID
		}
		return frame, nil
	case PriorityFrame:
		frame.StreamID = s.validStreamID(frame.StreamID)
		if !frame.StreamID.Valid() {
			return nil, ErrInvalidStreamID
		}
		return frame, nil
	default:
		return nil, ErrChildMessage
	}
}

func (s *streamChildSink) validStreamID(id StreamID) StreamID {
	if id == 0 {
		return s.streamID
	}
	if id != s.streamID {
		return 0
	}
	return id
}

func (s *streamChildSink) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *streamChildSink) markClosed(remote bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.closed = true
	s.remote = remote
	return true
}

func (s *streamChildSink) closeFromParent() {
	_ = s.markClosed(true)
}

func streamChildChannelID(parent transport.ChannelID, streamID StreamID) transport.ChannelID {
	return parent<<32 | transport.ChannelID(streamID)
}

func releaseChildMessage(msg any) {
	message.Release(msg)
}

var _ channel.Channel = (*StreamChannel)(nil)
