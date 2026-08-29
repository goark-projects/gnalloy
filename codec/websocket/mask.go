package websocket

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

func newMaskedFrameBuffer(alloc buffer.Allocator, frame Frame, headerLength int, payloadLength int) (buffer.ByteBuf, error) {
	out, err := alloc.Acquire(headerLength + payloadLength)
	if err != nil {
		return nil, err
	}
	if err := writeWebSocketHeader(out, frame, payloadLength); err != nil {
		out.Release()
		return nil, err
	}
	if err := writeMaskedReadableBytes(out, frame.Payload, frame.MaskKey); err != nil {
		out.Release()
		return nil, err
	}
	return out, nil
}

func unmask(in buffer.ByteBuf, key [4]byte) (buffer.ByteBuf, error) {
	offset := 0
	buffer.ForEachReadableSlice(in, func(data []byte) bool {
		maskBytesInPlace(data, key, offset)
		offset += len(data)
		return true
	})
	return in, nil
}

func writeMaskedPayload(ctx *channel.HandlerContext, in buffer.ByteBuf, key [4]byte) error {
	out, err := ctx.Channel().Allocator().Acquire(in.ReadableBytes())
	if err != nil {
		in.Release()
		return err
	}
	if err := writeMaskedReadableBytes(out, in, key); err != nil {
		out.Release()
		in.Release()
		return err
	}
	in.Release()
	return codec.WriteOutboundBuffer(ctx, out)
}

func writeMaskedReadableBytes(out buffer.ByteBuf, in buffer.ByteBuf, key [4]byte) error {
	n := in.ReadableBytes()
	view := out.WritableBytesView()
	if len(view) < n {
		return writeMaskedReadableBytesFallback(out, in, key)
	}
	written := 0
	ok := buffer.ForEachReadableSlice(in, func(data []byte) bool {
		for i, b := range data {
			view[written+i] = b ^ key[(written+i)&3]
		}
		written += len(data)
		return true
	})
	if !ok || written != n {
		return buffer.ErrNotEnoughBytes
	}
	return out.AdvanceWriter(n)
}

func writeMaskedReadableBytesFallback(out buffer.ByteBuf, in buffer.ByteBuf, key [4]byte) error {
	var tmp [1024]byte
	var writeErr error
	written := 0
	ok := buffer.ForEachReadableSlice(in, func(data []byte) bool {
		for len(data) > 0 {
			chunk := data
			if len(chunk) > len(tmp) {
				chunk = data[:len(tmp)]
			}
			for i, b := range chunk {
				tmp[i] = b ^ key[(written+i)&3]
			}
			if _, writeErr = out.WriteBytes(tmp[:len(chunk)]); writeErr != nil {
				return false
			}
			written += len(chunk)
			data = data[len(chunk):]
		}
		return true
	})
	if writeErr != nil {
		return writeErr
	}
	if !ok || written != in.ReadableBytes() {
		return buffer.ErrNotEnoughBytes
	}
	return nil
}

func maskBytesInPlace(data []byte, key [4]byte, offset int) {
	for i := range data {
		data[i] ^= key[(offset+i)&3]
	}
}
