package http2

import "goark.dev/gnalloy/channel"

// OutboundFlowControlConfig 描述 HTTP/2 出站 DATA 帧流控窗口和待发送队列边界。
type OutboundFlowControlConfig struct {
	// InitialConnectionWindow 是连接级初始发送窗口，0 使用 RFC 默认值。
	InitialConnectionWindow int32
	// InitialStreamWindow 是 stream 级初始发送窗口，0 使用 RFC 默认值。
	InitialStreamWindow int32
	// MaxQueuedFrames 限制因窗口不足而暂存的 DATA 帧数，0 表示不限制。
	MaxQueuedFrames int
	// MaxQueuedBytes 限制因窗口不足而暂存的 DATA 字节数，0 表示不限制。
	MaxQueuedBytes int
}

// OutboundFlowController 在出站路径按连接和 stream 窗口暂存 DATA 帧。
type OutboundFlowController struct {
	cfg                 OutboundFlowControlConfig
	connectionWindow    int32
	initialStreamWindow int32
	streamWindows       map[StreamID]int32
	pending             []pendingDataFrame
	pendingHead         int
	pendingBytes        int
	flushPending        bool
}

type pendingDataFrame struct {
	frame DataFrame
	size  int
}

// NewOutboundFlowController 创建轻量出站流控辅助 handler。
func NewOutboundFlowController(cfg OutboundFlowControlConfig) *OutboundFlowController {
	connWindow := normalizedWindow(cfg.InitialConnectionWindow)
	streamWindow := normalizedWindow(cfg.InitialStreamWindow)
	return &OutboundFlowController{
		cfg:                 cfg,
		connectionWindow:    connWindow,
		initialStreamWindow: streamWindow,
		streamWindows:       make(map[StreamID]int32, 8),
	}
}

// ConnectionWindow 返回当前连接级发送窗口。
func (c *OutboundFlowController) ConnectionWindow() int32 {
	return c.connectionWindow
}

// StreamWindow 返回指定 stream 当前发送窗口；未见过的 stream 返回初始窗口。
func (c *OutboundFlowController) StreamWindow(id StreamID) int32 {
	if window, ok := c.streamWindows[id]; ok {
		return window
	}
	return c.initialStreamWindow
}

// PendingFrames 返回当前因窗口不足暂存的 DATA 帧数。
func (c *OutboundFlowController) PendingFrames() int {
	return len(c.pending) - c.pendingHead
}

// PendingBytes 返回当前因窗口不足暂存的 DATA payload 字节数。
func (c *OutboundFlowController) PendingBytes() int {
	return c.pendingBytes
}

// Write 按 HTTP/2 flow-control 窗口决定立即写出或有界排队。
func (c *OutboundFlowController) Write(ctx *channel.HandlerContext, msg any) error {
	switch frame := msg.(type) {
	case HeadersFrame:
		if err := c.ensureStream(frame.StreamID); err != nil {
			frame.Release()
			return err
		}
	case HeadersBlock:
		if err := c.ensureStream(frame.StreamID); err != nil {
			return err
		}
	case DataFrame:
		return c.writeData(ctx, frame)
	case RSTStreamFrame:
		delete(c.streamWindows, frame.StreamID)
	}
	return ctx.Write(msg)
}

// Flush 在队列未清空时记住 flush 需求，待 WINDOW_UPDATE 释放 DATA 后再下发。
func (c *OutboundFlowController) Flush(ctx *channel.HandlerContext) error {
	if c.PendingFrames() == 0 {
		return ctx.Flush()
	}
	c.flushPending = true
	return nil
}

// ChannelRead 消费 WINDOW_UPDATE 更新发送窗口，并按原始顺序释放可写 DATA。
func (c *OutboundFlowController) ChannelRead(ctx *channel.HandlerContext, msg any) {
	switch frame := msg.(type) {
	case WindowUpdateFrame:
		if err := c.applyWindowUpdate(frame); err != nil {
			ctx.FireExceptionCaught(err)
			return
		}
		if err := c.drain(ctx); err != nil {
			ctx.FireExceptionCaught(err)
			return
		}
	case SettingsFrame:
		if !frame.Ack {
			if err := c.applySettings(frame.Settings); err != nil {
				ctx.FireExceptionCaught(err)
				return
			}
			if err := c.drain(ctx); err != nil {
				ctx.FireExceptionCaught(err)
				return
			}
		}
	}
	ctx.FireChannelRead(msg)
}

// ChannelInactive 连接关闭时确定性释放仍在队列中的 DATA payload。
func (c *OutboundFlowController) ChannelInactive(ctx *channel.HandlerContext) {
	c.releasePending()
	ctx.FireChannelInactive()
}

// HandlerRemoved 被动态移除时同样释放队列，避免 ByteBuf 引用泄漏。
func (c *OutboundFlowController) HandlerRemoved(_ *channel.HandlerContext) error {
	c.releasePending()
	return nil
}

func (c *OutboundFlowController) writeData(ctx *channel.HandlerContext, frame DataFrame) error {
	if err := c.ensureStream(frame.StreamID); err != nil {
		frame.Release()
		return err
	}
	size := readableBytes(frame.Data)
	if size > int(maxStreamID) {
		frame.Release()
		return ErrFlowControl
	}
	if !c.canSend(frame.StreamID, size) {
		return c.enqueue(frame, size)
	}
	c.consume(frame.StreamID, size)
	if err := ctx.Write(frame); err != nil {
		frame.Release()
		return err
	}
	return nil
}

func (c *OutboundFlowController) enqueue(frame DataFrame, size int) error {
	if c.cfg.MaxQueuedFrames > 0 && c.PendingFrames() >= c.cfg.MaxQueuedFrames {
		frame.Release()
		return ErrFlowControl
	}
	if c.cfg.MaxQueuedBytes > 0 && c.pendingBytes > c.cfg.MaxQueuedBytes-size {
		frame.Release()
		return ErrFlowControl
	}
	c.pending = append(c.pending, pendingDataFrame{frame: frame, size: size})
	c.pendingBytes += size
	return nil
}

func (c *OutboundFlowController) drain(ctx *channel.HandlerContext) error {
	wrote := false
	for c.pendingHead < len(c.pending) {
		pending := c.pending[c.pendingHead]
		if !c.canSend(pending.frame.StreamID, pending.size) {
			break
		}
		c.pending[c.pendingHead] = pendingDataFrame{}
		c.pendingHead++
		c.pendingBytes -= pending.size
		c.consume(pending.frame.StreamID, pending.size)
		if err := ctx.Write(pending.frame); err != nil {
			pending.frame.Release()
			c.compactPending()
			return err
		}
		wrote = true
	}
	c.compactPending()
	if wrote && c.flushPending && c.PendingFrames() == 0 {
		c.flushPending = false
		return ctx.Flush()
	}
	return nil
}

func (c *OutboundFlowController) canSend(id StreamID, size int) bool {
	return size <= int(c.connectionWindow) && size <= int(c.StreamWindow(id))
}

func (c *OutboundFlowController) consume(id StreamID, size int) {
	if size == 0 {
		return
	}
	c.connectionWindow -= int32(size)
	c.streamWindows[id] = c.StreamWindow(id) - int32(size)
}

func (c *OutboundFlowController) applyWindowUpdate(frame WindowUpdateFrame) error {
	if frame.StreamID == 0 {
		if !addWindow(&c.connectionWindow, frame.Increment) {
			return ErrFlowControl
		}
		return nil
	}
	if err := c.ensureStream(frame.StreamID); err != nil {
		return err
	}
	window := c.StreamWindow(frame.StreamID)
	if !addWindow(&window, frame.Increment) {
		return ErrFlowControl
	}
	c.streamWindows[frame.StreamID] = window
	return nil
}

func (c *OutboundFlowController) applySettings(settings []Setting) error {
	for _, setting := range settings {
		if setting.ID != SettingInitialWindowSize {
			continue
		}
		if setting.Value > uint32(maxStreamID) {
			return ErrFlowControl
		}
		nextInitial := int32(setting.Value)
		delta := nextInitial - c.initialStreamWindow
		for id, window := range c.streamWindows {
			next := int64(window) + int64(delta)
			if next > int64(maxStreamID) || next < -int64(maxStreamID) {
				return ErrFlowControl
			}
			c.streamWindows[id] = int32(next)
		}
		c.initialStreamWindow = nextInitial
	}
	return nil
}

func (c *OutboundFlowController) ensureStream(id StreamID) error {
	if !id.Valid() {
		return ErrInvalidStreamID
	}
	if _, ok := c.streamWindows[id]; !ok {
		c.streamWindows[id] = c.initialStreamWindow
	}
	return nil
}

func (c *OutboundFlowController) releasePending() {
	for i := c.pendingHead; i < len(c.pending); i++ {
		c.pending[i].frame.Release()
		c.pending[i] = pendingDataFrame{}
	}
	c.pending = nil
	c.pendingHead = 0
	c.pendingBytes = 0
	c.flushPending = false
}

func (c *OutboundFlowController) compactPending() {
	if c.pendingHead == 0 {
		return
	}
	if c.pendingHead == len(c.pending) {
		c.pending = c.pending[:0]
		c.pendingHead = 0
		return
	}
	if c.pendingHead < len(c.pending)/2 {
		return
	}
	copy(c.pending, c.pending[c.pendingHead:])
	tail := len(c.pending) - c.pendingHead
	for i := tail; i < len(c.pending); i++ {
		c.pending[i] = pendingDataFrame{}
	}
	c.pending = c.pending[:tail]
	c.pendingHead = 0
}

func normalizedWindow(window int32) int32 {
	if window == 0 {
		return defaultInitialWindowSize
	}
	if window < 0 {
		return 0
	}
	return window
}
