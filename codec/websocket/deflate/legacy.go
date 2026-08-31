package deflate

import (
	"fmt"
	"strings"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/codec/websocket"
)

const (
	// FrameExtensionName 是早期 WebSocket deflate-frame 扩展名。
	FrameExtensionName = "deflate-frame"
	// WebKitFrameExtensionName 是 WebKit 早期实现使用的 deflate-frame 扩展名。
	WebKitFrameExtensionName = "x-webkit-deflate-frame"
)

// OfferFrameExtension 返回 Sec-WebSocket-Extensions 可使用的 legacy frame 扩展名。
func OfferFrameExtension() string {
	return FrameExtensionName
}

// ParseFrameExtension 在 Sec-WebSocket-Extensions 头部中查找 legacy deflate-frame。
func ParseFrameExtension(header string) (string, bool, error) {
	for _, ext := range strings.Split(header, ",") {
		parts := strings.Split(ext, ";")
		if len(parts) == 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(parts[0]))
		if name != FrameExtensionName && name != WebKitFrameExtensionName {
			continue
		}
		if len(parts) > 1 {
			return "", false, fmt.Errorf("%w: %s has parameters", ErrInvalidExtension, name)
		}
		return name, true, nil
	}
	return "", false, nil
}

// LegacyFrameCompressor 对每个 WebSocket data frame 独立应用 deflate。
type LegacyFrameCompressor struct {
	cfg Config
}

// NewLegacyFrameCompressor 创建 legacy deflate-frame 压缩 handler。
func NewLegacyFrameCompressor(cfg Config) (*LegacyFrameCompressor, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &LegacyFrameCompressor{cfg: normalized}, nil
}

// Write 压缩每个 data/continuation frame，并设置 RSV1。
func (h *LegacyFrameCompressor) Write(ctx *channel.HandlerContext, msg any) error {
	frame, ok := msg.(websocket.Frame)
	if !ok {
		return ctx.Write(msg)
	}
	if websocketFrameHasRSV(frame) {
		frame.Release()
		return ErrInvalidFrame
	}
	if isControl(frame) {
		return ctx.Write(frame)
	}
	if !isLegacyData(frame) {
		frame.Release()
		return ErrInvalidFrame
	}
	data := copyFramePayload(frame.Payload)
	frame.Payload = nil
	if len(data) > h.cfg.MaxMessageBytes {
		return codec.ErrFrameTooLong
	}
	compressed, err := compressMessage(data, h.cfg.CompressionLevel)
	if err != nil {
		return err
	}
	payload, err := byteBufFromBytes(ctx.Channel().Allocator(), compressed)
	if err != nil {
		return err
	}
	frame.Payload = payload
	frame.RSV1 = true
	if err := ctx.Write(frame); err != nil {
		frame.Release()
		return err
	}
	return nil
}

// LegacyFrameDecompressor 对 RSV1 data/continuation frame 独立解压。
type LegacyFrameDecompressor struct {
	cfg Config
}

// NewLegacyFrameDecompressor 创建 legacy deflate-frame 解压 handler。
func NewLegacyFrameDecompressor(cfg Config) (*LegacyFrameDecompressor, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &LegacyFrameDecompressor{cfg: normalized}, nil
}

// ChannelRead 解压 RSV1 标记的 legacy compressed frame。
func (h *LegacyFrameDecompressor) ChannelRead(ctx *channel.HandlerContext, msg any) {
	frame, ok := msg.(websocket.Frame)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if err := h.readFrame(ctx, frame); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (h *LegacyFrameDecompressor) readFrame(ctx *channel.HandlerContext, frame websocket.Frame) error {
	if frame.RSV2 || frame.RSV3 || (isControl(frame) && frame.RSV1) {
		frame.Release()
		return ErrInvalidFrame
	}
	if isControl(frame) || !frame.RSV1 {
		ctx.FireChannelRead(frame)
		return nil
	}
	if !isLegacyData(frame) {
		frame.Release()
		return ErrInvalidFrame
	}
	data := copyFramePayload(frame.Payload)
	frame.Payload = nil
	frame.RSV1 = false
	if len(data) > h.cfg.MaxMessageBytes {
		return codec.ErrFrameTooLong
	}
	decompressed, err := decompressMessage(data, h.cfg.MaxMessageBytes)
	if err != nil {
		return err
	}
	payload, err := byteBufFromBytes(ctx.Channel().Allocator(), decompressed)
	if err != nil {
		return err
	}
	frame.Payload = payload
	ctx.FireChannelRead(frame)
	return nil
}

func isLegacyData(frame websocket.Frame) bool {
	return isData(frame) || frame.Opcode == websocket.OpcodeContinuation
}
