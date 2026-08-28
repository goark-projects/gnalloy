package buffer

// ReadableSlices 返回当前可读区域的底层切片视图。
func (c *CompositeByteBuf) ReadableSlices(dst [][]byte) [][]byte {
	if c.refs.Load() <= 0 || c.readerIndex == c.writerIndex {
		return dst
	}
	for i := range c.components {
		comp := &c.components[i]
		from := max(c.readerIndex, comp.start)
		to := min(c.writerIndex, comp.end)
		if from >= to {
			continue
		}
		part, err := comp.buf.Slice(comp.buf.ReaderIndex()+from-comp.start, to-from)
		if err != nil {
			continue
		}
		dst = part.ReadableSlices(dst)
		part.Release()
	}
	return dst
}
