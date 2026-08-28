package buffer

// CopyReadableBytes 将 src 的可读区复制到 dst，不推进 readerIndex。
//
// 对内置 ByteBuf 类型直接访问已校验的内部布局，避免热路径为 ReadableSlices
// 生成临时切片列表；外部实现仍回退到公开接口。
func CopyReadableBytes(dst []byte, src ByteBuf) int {
	if len(dst) == 0 || src == nil {
		return 0
	}
	switch b := src.(type) {
	case *DirectByteBuf:
		if b.refs.Load() <= 0 {
			return 0
		}
		return copy(dst, b.data[b.readerIndex:b.writerIndex])
	case *slicedByteBuf:
		if b.refs.Load() <= 0 {
			return 0
		}
		return copy(dst, b.data[b.readerIndex:b.writerIndex])
	case *CompositeByteBuf:
		if b.refs.Load() <= 0 {
			return 0
		}
		return b.copyReadableTo(dst)
	default:
		return copyReadableBytesBySlices(dst, src)
	}
}

func copyReadableBytesBySlices(dst []byte, src ByteBuf) int {
	var slices [8][]byte
	written := 0
	for _, part := range src.ReadableSlices(slices[:0]) {
		written += copy(dst[written:], part)
		if written == len(dst) {
			break
		}
	}
	return written
}
