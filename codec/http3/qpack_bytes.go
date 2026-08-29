package http3

import "goark.dev/gnalloy/buffer"

func headerBlockBytes(block buffer.ByteBuf) []byte {
	if block == nil || block.ReadableBytes() == 0 {
		return nil
	}
	if data, ok := buffer.ContiguousReadableBytes(block); ok {
		return data
	}
	out := make([]byte, block.ReadableBytes())
	if buffer.CopyReadableBytes(out, block) != len(out) {
		return nil
	}
	return out
}
