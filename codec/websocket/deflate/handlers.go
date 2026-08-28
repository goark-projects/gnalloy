package deflate

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/codec/websocket"
)

// Compressor 对出站 WebSocket data message 应用 permessage-deflate。
type Compressor struct {
	cfg             Config
	fragmentOpcode  byte
	fragmentMask    bool
	fragmentMaskKey [4]byte
	fragmentData    []byte
}

// NewCompressor 创建无上下文复用的 permessage-deflate 压缩 handler。
func NewCompressor(cfg Config) (*Compressor, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Compressor{cfg: normalized}, nil
}

func (h *Compressor) Write(ctx *channel.HandlerContext, msg any) error {
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
	if isData(frame) {
		return h.writeData(ctx, frame)
	}
	if frame.Opcode == websocket.OpcodeContinuation {
		return h.writeContinuation(ctx, frame)
	}
	frame.Release()
	return ErrInvalidFrame
}

func (h *Compressor) ChannelInactive(ctx *channel.HandlerContext) {
	h.reset()
	ctx.FireChannelInactive()
}

func (h *Compressor) writeData(ctx *channel.HandlerContext, frame websocket.Frame) error {
	if h.fragmentOpcode != 0 {
		frame.Release()
		return websocket.ErrFragmentInProgress
	}
	data := copyFramePayload(frame.Payload)
	frame.Payload = nil
	if len(data) > h.cfg.MaxMessageBytes {
		return codec.ErrFrameTooLong
	}
	if frame.Final {
		return h.writeCompressed(ctx, frame, data)
	}
	h.fragmentOpcode = frame.Opcode
	h.fragmentMask = frame.Masked
	h.fragmentMaskKey = frame.MaskKey
	h.fragmentData = append(h.fragmentData[:0], data...)
	return nil
}

func (h *Compressor) writeContinuation(ctx *channel.HandlerContext, frame websocket.Frame) error {
	if h.fragmentOpcode == 0 {
		frame.Release()
		return websocket.ErrUnexpectedContinuation
	}
	data := copyFramePayload(frame.Payload)
	frame.Payload = nil
	if len(h.fragmentData)+len(data) > h.cfg.MaxMessageBytes {
		h.reset()
		return codec.ErrFrameTooLong
	}
	h.fragmentData = append(h.fragmentData, data...)
	if !frame.Final {
		return nil
	}
	out := websocket.Frame{
		Final:   true,
		Opcode:  h.fragmentOpcode,
		Masked:  h.fragmentMask,
		MaskKey: h.fragmentMaskKey,
	}
	data = append([]byte(nil), h.fragmentData...)
	h.reset()
	return h.writeCompressed(ctx, out, data)
}

func (h *Compressor) writeCompressed(ctx *channel.HandlerContext, frame websocket.Frame, data []byte) error {
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

func (h *Compressor) reset() {
	h.fragmentOpcode = 0
	h.fragmentMask = false
	h.fragmentMaskKey = [4]byte{}
	h.fragmentData = h.fragmentData[:0]
}

// Decompressor 对入站 RSV1 data message 应用 permessage-deflate 解压。
type Decompressor struct {
	cfg            Config
	fragmentOpcode byte
	fragmentData   []byte
}

// NewDecompressor 创建无上下文复用的 permessage-deflate 解压 handler。
func NewDecompressor(cfg Config) (*Decompressor, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Decompressor{cfg: normalized}, nil
}

func (h *Decompressor) ChannelRead(ctx *channel.HandlerContext, msg any) {
	frame, ok := msg.(websocket.Frame)
	if !ok {
		ctx.FireChannelRead(msg)
		return
	}
	if err := h.readFrame(ctx, frame); err != nil {
		ctx.FireExceptionCaught(err)
	}
}

func (h *Decompressor) ChannelInactive(ctx *channel.HandlerContext) {
	h.reset()
	ctx.FireChannelInactive()
}

func (h *Decompressor) readFrame(ctx *channel.HandlerContext, frame websocket.Frame) error {
	if frame.RSV2 || frame.RSV3 || (isControl(frame) && frame.RSV1) {
		frame.Release()
		return ErrInvalidFrame
	}
	if isControl(frame) {
		ctx.FireChannelRead(frame)
		return nil
	}
	if isData(frame) {
		if h.fragmentOpcode != 0 {
			frame.Release()
			return websocket.ErrFragmentInProgress
		}
		if !frame.RSV1 {
			ctx.FireChannelRead(frame)
			return nil
		}
		return h.readCompressedData(ctx, frame)
	}
	if frame.Opcode == websocket.OpcodeContinuation {
		return h.readContinuation(ctx, frame)
	}
	frame.Release()
	return ErrInvalidFrame
}

func (h *Decompressor) readCompressedData(ctx *channel.HandlerContext, frame websocket.Frame) error {
	data := copyFramePayload(frame.Payload)
	frame.Payload = nil
	frame.RSV1 = false
	if len(data) > h.cfg.MaxMessageBytes {
		return codec.ErrFrameTooLong
	}
	if frame.Final {
		return h.fireDecompressed(ctx, frame, data)
	}
	h.fragmentOpcode = frame.Opcode
	h.fragmentData = append(h.fragmentData[:0], data...)
	return nil
}

func (h *Decompressor) readContinuation(ctx *channel.HandlerContext, frame websocket.Frame) error {
	if frame.RSV1 {
		frame.Release()
		return ErrInvalidFrame
	}
	if h.fragmentOpcode == 0 {
		ctx.FireChannelRead(frame)
		return nil
	}
	data := copyFramePayload(frame.Payload)
	frame.Payload = nil
	if len(h.fragmentData)+len(data) > h.cfg.MaxMessageBytes {
		h.reset()
		return codec.ErrFrameTooLong
	}
	h.fragmentData = append(h.fragmentData, data...)
	if !frame.Final {
		return nil
	}
	out := websocket.Frame{Final: true, Opcode: h.fragmentOpcode}
	data = append([]byte(nil), h.fragmentData...)
	h.reset()
	return h.fireDecompressed(ctx, out, data)
}

func (h *Decompressor) fireDecompressed(ctx *channel.HandlerContext, frame websocket.Frame, data []byte) error {
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

func (h *Decompressor) reset() {
	h.fragmentOpcode = 0
	h.fragmentData = h.fragmentData[:0]
}

func copyFramePayload(payload buffer.ByteBuf) []byte {
	if payload == nil {
		return nil
	}
	if payload.ReadableBytes() == 0 {
		payload.Release()
		return nil
	}
	data := append([]byte(nil), payload.Bytes()...)
	payload.Release()
	return data
}

func websocketFrameHasRSV(frame websocket.Frame) bool {
	return frame.RSV1 || frame.RSV2 || frame.RSV3
}

func isData(frame websocket.Frame) bool {
	return frame.Opcode == websocket.OpcodeText || frame.Opcode == websocket.OpcodeBinary
}

func isControl(frame websocket.Frame) bool {
	switch frame.Opcode {
	case websocket.OpcodeClose, websocket.OpcodePing, websocket.OpcodePong:
		return true
	default:
		return false
	}
}
