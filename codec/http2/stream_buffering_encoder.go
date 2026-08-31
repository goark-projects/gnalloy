package http2

import (
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/internal/message"
)

// StreamBufferingEncoderConfig 描述远端并发 stream 限制下的出站缓冲策略。
type StreamBufferingEncoderConfig struct {
	// MaxConcurrentStreams 是远端允许的最大并发 stream 数，0 表示不限制。
	MaxConcurrentStreams int
	// MaxBufferedStreams 是等待打开的最大 stream 数，0 表示不限制。
	MaxBufferedStreams int
	// MaxBufferedFramesPerStream 是单个等待 stream 可缓冲的最大 frame 数，0 表示不限制。
	MaxBufferedFramesPerStream int
}

// StreamBufferingEncoder 在远端并发 stream 额度不足时有界缓冲新 stream。
type StreamBufferingEncoder struct {
	cfg     StreamBufferingEncoderConfig
	active  map[StreamID]struct{}
	pending map[StreamID]*bufferedStream
	order   []StreamID
}

type bufferedStream struct {
	id     StreamID
	frames []any
}

// NewStreamBufferingEncoder 创建 HTTP/2 stream 缓冲编码器。
func NewStreamBufferingEncoder(cfg StreamBufferingEncoderConfig) *StreamBufferingEncoder {
	return &StreamBufferingEncoder{
		cfg:     cfg,
		active:  make(map[StreamID]struct{}, 8),
		pending: make(map[StreamID]*bufferedStream, 4),
	}
}

// SetMaxConcurrentStreams 更新远端 SETTINGS_MAX_CONCURRENT_STREAMS 预算并允许后续 drain。
func (e *StreamBufferingEncoder) SetMaxConcurrentStreams(max int) {
	if e == nil {
		return
	}
	e.cfg.MaxConcurrentStreams = max
}

// ActiveStreams 返回当前占用远端并发额度的出站 stream 数。
func (e *StreamBufferingEncoder) ActiveStreams() int {
	if e == nil {
		return 0
	}
	return len(e.active)
}

// PendingStreams 返回等待打开的 stream 数。
func (e *StreamBufferingEncoder) PendingStreams() int {
	if e == nil {
		return 0
	}
	return len(e.order)
}

// PendingFrames 返回全部等待 stream 中的 frame 数。
func (e *StreamBufferingEncoder) PendingFrames() int {
	if e == nil {
		return 0
	}
	total := 0
	for _, stream := range e.pending {
		total += len(stream.frames)
	}
	return total
}

// ChannelRead 应用远端 SETTINGS，并在额度增加时释放等待 stream。
func (e *StreamBufferingEncoder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if settings, ok := msg.(SettingsFrame); ok && !settings.Ack {
		for _, setting := range settings.Settings {
			if setting.ID == SettingMaxConcurrentStreams {
				e.SetMaxConcurrentStreams(int(setting.Value))
				if err := e.drain(ctx); err != nil {
					ctx.FireExceptionCaught(err)
					return
				}
				break
			}
		}
	}
	ctx.FireChannelRead(msg)
}

// Write 在并发额度不足时缓冲新 stream，已激活 stream 继续直接写出。
func (e *StreamBufferingEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	if e == nil {
		return ctx.Write(msg)
	}
	switch frame := msg.(type) {
	case HeadersFrame:
		return e.writeHeaders(ctx, frame.StreamID, frame.Flags&FlagEndStream != 0, msg)
	case HeadersBlock:
		return e.writeHeaders(ctx, frame.StreamID, frame.EndStream, msg)
	case DataFrame:
		return e.writeData(ctx, frame)
	case RSTStreamFrame:
		return e.writeRST(ctx, frame)
	default:
		return ctx.Write(msg)
	}
}

// ChannelInactive 释放仍未获得并发额度的待发送帧。
func (e *StreamBufferingEncoder) ChannelInactive(ctx *channel.HandlerContext) {
	e.releasePending()
	ctx.FireChannelInactive()
}

// HandlerRemoved 被动态移除时释放待发送帧。
func (e *StreamBufferingEncoder) HandlerRemoved(_ *channel.HandlerContext) error {
	e.releasePending()
	return nil
}

func (e *StreamBufferingEncoder) writeHeaders(ctx *channel.HandlerContext, id StreamID, endStream bool, msg any) error {
	if !id.Valid() {
		release(msg)
		return ErrInvalidStreamID
	}
	if e.isPending(id) {
		return e.enqueue(id, msg)
	}
	if e.isActive(id) || e.canOpen() {
		return e.writeDirect(ctx, msg, id, endStream)
	}
	return e.enqueue(id, msg)
}

func (e *StreamBufferingEncoder) writeData(ctx *channel.HandlerContext, frame DataFrame) error {
	if !frame.StreamID.Valid() {
		frame.Release()
		return ErrInvalidStreamID
	}
	if e.isPending(frame.StreamID) {
		return e.enqueue(frame.StreamID, frame)
	}
	if !e.isActive(frame.StreamID) {
		frame.Release()
		return ErrInvalidStreamState
	}
	if err := ctx.Write(frame); err != nil {
		frame.Release()
		return err
	}
	if frame.Flags&FlagEndStream != 0 {
		e.closeActive(frame.StreamID)
		return e.drain(ctx)
	}
	return nil
}

func (e *StreamBufferingEncoder) writeRST(ctx *channel.HandlerContext, frame RSTStreamFrame) error {
	if e.isPending(frame.StreamID) {
		e.dropPending(frame.StreamID)
		return e.drain(ctx)
	}
	if err := ctx.Write(frame); err != nil {
		return err
	}
	e.closeActive(frame.StreamID)
	return e.drain(ctx)
}

func (e *StreamBufferingEncoder) writeDirect(ctx *channel.HandlerContext, msg any, id StreamID, endStream bool) error {
	if err := ctx.Write(msg); err != nil {
		release(msg)
		return err
	}
	if endStream {
		e.closeActive(id)
		return e.drain(ctx)
	}
	e.active[id] = struct{}{}
	return nil
}

func (e *StreamBufferingEncoder) enqueue(id StreamID, msg any) error {
	stream := e.pending[id]
	if stream == nil {
		if e.cfg.MaxBufferedStreams > 0 && len(e.pending) >= e.cfg.MaxBufferedStreams {
			release(msg)
			return ErrStreamBufferFull
		}
		stream = &bufferedStream{id: id}
		e.pending[id] = stream
		e.order = append(e.order, id)
	}
	if e.cfg.MaxBufferedFramesPerStream > 0 && len(stream.frames) >= e.cfg.MaxBufferedFramesPerStream {
		release(msg)
		return ErrStreamBufferFull
	}
	stream.frames = append(stream.frames, msg)
	return nil
}

func (e *StreamBufferingEncoder) drain(ctx *channel.HandlerContext) error {
	for e.canOpen() && len(e.order) > 0 {
		id := e.order[0]
		stream := e.pending[id]
		e.removePendingHead(id)
		if stream == nil {
			continue
		}
		if err := e.writeBufferedStream(ctx, stream); err != nil {
			stream.release()
			return err
		}
	}
	return nil
}

func (e *StreamBufferingEncoder) writeBufferedStream(ctx *channel.HandlerContext, stream *bufferedStream) error {
	for i, frame := range stream.frames {
		endStream := frameEndsStream(frame)
		if err := ctx.Write(frame); err != nil {
			release(frame)
			stream.frames[i] = nil
			return err
		}
		stream.frames[i] = nil
		if endStream {
			e.closeActive(stream.id)
			stream.releaseFrom(i + 1)
			return e.drain(ctx)
		}
		e.active[stream.id] = struct{}{}
	}
	stream.frames = nil
	return nil
}

func (e *StreamBufferingEncoder) removePendingHead(id StreamID) {
	delete(e.pending, id)
	if len(e.order) == 0 {
		return
	}
	copy(e.order, e.order[1:])
	e.order[len(e.order)-1] = 0
	e.order = e.order[:len(e.order)-1]
}

func (e *StreamBufferingEncoder) dropPending(id StreamID) {
	stream := e.pending[id]
	if stream != nil {
		stream.release()
	}
	delete(e.pending, id)
	for i, candidate := range e.order {
		if candidate != id {
			continue
		}
		copy(e.order[i:], e.order[i+1:])
		e.order[len(e.order)-1] = 0
		e.order = e.order[:len(e.order)-1]
		return
	}
}

func (e *StreamBufferingEncoder) releasePending() {
	for _, stream := range e.pending {
		stream.release()
	}
	for i := range e.order {
		e.order[i] = 0
	}
	e.order = nil
	e.pending = make(map[StreamID]*bufferedStream)
}

func (e *StreamBufferingEncoder) isActive(id StreamID) bool {
	_, ok := e.active[id]
	return ok
}

func (e *StreamBufferingEncoder) isPending(id StreamID) bool {
	_, ok := e.pending[id]
	return ok
}

func (e *StreamBufferingEncoder) canOpen() bool {
	return e.cfg.MaxConcurrentStreams <= 0 || len(e.active) < e.cfg.MaxConcurrentStreams
}

func (e *StreamBufferingEncoder) closeActive(id StreamID) {
	delete(e.active, id)
}

func (s *bufferedStream) release() {
	if s == nil {
		return
	}
	s.releaseFrom(0)
}

func (s *bufferedStream) releaseFrom(start int) {
	for i := start; i < len(s.frames); i++ {
		release(s.frames[i])
		s.frames[i] = nil
	}
	s.frames = nil
}

func frameEndsStream(msg any) bool {
	switch frame := msg.(type) {
	case HeadersFrame:
		return frame.Flags&FlagEndStream != 0
	case HeadersBlock:
		return frame.EndStream
	case DataFrame:
		return frame.Flags&FlagEndStream != 0
	default:
		return false
	}
}

func release(msg any) {
	message.Release(msg)
}
