package tls

import "goark.dev/gnalloy/buffer"

// copyReadableBytes 把 ByteBuf 的可读区复制为稳定切片，用于跨 TLS goroutine 边界传递。
//
// crypto/tls 在 net.Conn 语义下不会接管调用方 ByteBuf 的生命周期。这里按
// ReadableSlices 复制，避免 CompositeByteBuf.Bytes 造成的额外中间拷贝。
func copyReadableBytes(buf buffer.ByteBuf) []byte {
	if buf == nil || buf.ReadableBytes() == 0 {
		return nil
	}
	out := make([]byte, 0, buf.ReadableBytes())
	for _, part := range buf.ReadableSlices(nil) {
		out = append(out, part...)
	}
	return out
}
