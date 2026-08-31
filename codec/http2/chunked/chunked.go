package chunked

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/codec/http2"
)

// DataChunkedInput 将通用 ByteBuf chunk 输入包装为 HTTP/2 DATA frame 输入。
type DataChunkedInput struct {
	streamID       http2.StreamID
	input          codec.ChunkedInput
	endStream      bool
	endStreamSent  bool
	sourceFinished bool
}

// NewDataChunkedInput 创建 HTTP/2 DATA frame 分片输入。
func NewDataChunkedInput(streamID http2.StreamID, input codec.ChunkedInput, endStream bool) (*DataChunkedInput, error) {
	if !streamID.Valid() || input == nil {
		if input != nil {
			_ = input.Close()
		}
		return nil, http2.ErrInvalidStreamID
	}
	return &DataChunkedInput{streamID: streamID, input: input, endStream: endStream}, nil
}

// ReadFrame 读取下一片 DATA frame。
func (i *DataChunkedInput) ReadFrame(alloc buffer.Allocator) (http2.DataFrame, bool, error) {
	if i == nil || i.input == nil {
		return http2.DataFrame{}, true, nil
	}
	for {
		if i.sourceFinished {
			return i.emptyEndStream()
		}
		chunk, done, err := i.input.ReadChunk(alloc)
		if err != nil {
			if chunk != nil {
				chunk.Release()
			}
			return http2.DataFrame{}, false, err
		}
		if done {
			i.sourceFinished = true
		}
		if chunk == nil || chunk.ReadableBytes() == 0 {
			if chunk != nil {
				chunk.Release()
			}
			if done {
				return i.emptyEndStream()
			}
			continue
		}
		flags := http2.Flags(0)
		if done && i.endStream {
			flags = http2.FlagEndStream
			i.endStreamSent = true
		}
		return http2.DataFrame{StreamID: i.streamID, Flags: flags, Data: chunk}, done && (!i.endStream || i.endStreamSent), nil
	}
}

// Close 关闭底层分片输入。
func (i *DataChunkedInput) Close() error {
	if i == nil || i.input == nil {
		return nil
	}
	input := i.input
	i.input = nil
	return input.Close()
}

func (i *DataChunkedInput) emptyEndStream() (http2.DataFrame, bool, error) {
	if !i.endStream || i.endStreamSent {
		return http2.DataFrame{}, true, nil
	}
	i.endStreamSent = true
	return http2.DataFrame{StreamID: i.streamID, Flags: http2.FlagEndStream}, true, nil
}

// WriteHandler 将 DataChunkedInput 展开为 HTTP/2 DATA frame 序列。
type WriteHandler struct{}

// NewWriteHandler 创建 HTTP/2 DATA chunked 写 handler。
func NewWriteHandler() *WriteHandler {
	return &WriteHandler{}
}

// Write 顺序写出所有 DATA frame，并在完成后 flush。
func (h *WriteHandler) Write(ctx *channel.HandlerContext, msg any) error {
	input, ok := msg.(*DataChunkedInput)
	if !ok {
		return ctx.Write(msg)
	}
	defer input.Close()
	for {
		frame, done, err := input.ReadFrame(ctx.Channel().Allocator())
		if err != nil {
			frame.Release()
			return err
		}
		if frame.Data != nil || frame.Flags&http2.FlagEndStream != 0 {
			if err := ctx.Write(frame); err != nil {
				frame.Release()
				return err
			}
		}
		if done {
			return ctx.Flush()
		}
	}
}
