package buffer

import (
	"io"
	"sync/atomic"
)

type component struct {
	buf   ByteBuf
	start int
	end   int
}

// CompositeByteBuf 通过多个 ByteBuf 组成逻辑连续视图，不复制底层字节。
type CompositeByteBuf struct {
	components  []component
	readerIndex int
	writerIndex int
	refs        atomic.Int32
}

func NewCompositeByteBuf() *CompositeByteBuf {
	c := &CompositeByteBuf{}
	c.refs.Store(1)
	return c
}

func (c *CompositeByteBuf) checkAlive() error {
	if c.refs.Load() <= 0 {
		return ErrReleasedBuffer
	}
	return nil
}

func (c *CompositeByteBuf) Append(buf ByteBuf) {
	if buf == nil || buf.ReadableBytes() == 0 {
		if buf != nil {
			buf.Release()
		}
		return
	}
	start := c.writerIndex
	c.writerIndex += buf.ReadableBytes()
	c.components = append(c.components, component{buf: buf, start: start, end: c.writerIndex})
}

func (c *CompositeByteBuf) ReadableBytes() int { return c.writerIndex - c.readerIndex }
func (c *CompositeByteBuf) WritableBytes() int { return 0 }
func (c *CompositeByteBuf) Capacity() int      { return c.writerIndex }
func (c *CompositeByteBuf) ReaderIndex() int   { return c.readerIndex }
func (c *CompositeByteBuf) WriterIndex() int   { return c.writerIndex }

func (c *CompositeByteBuf) SetReaderIndex(index int) error {
	if err := c.checkAlive(); err != nil {
		return err
	}
	if index < 0 || index > c.writerIndex {
		return ErrInvalidIndex
	}
	c.readerIndex = index
	return nil
}

func (c *CompositeByteBuf) SetWriterIndex(index int) error {
	if err := c.checkAlive(); err != nil {
		return err
	}
	if index < c.readerIndex || index > c.writerIndex {
		return ErrInvalidIndex
	}
	c.writerIndex = index
	return nil
}

func (c *CompositeByteBuf) Clear() {
	for _, comp := range c.components {
		comp.buf.Release()
	}
	c.components = nil
	c.readerIndex = 0
	c.writerIndex = 0
}

func (c *CompositeByteBuf) GetByte(index int) (byte, bool) {
	if c.refs.Load() <= 0 || index < c.readerIndex || index >= c.writerIndex {
		return 0, false
	}
	for _, comp := range c.components {
		if index >= comp.start && index < comp.end {
			return comp.buf.GetByte(comp.buf.ReaderIndex() + index - comp.start)
		}
	}
	return 0, false
}

func (c *CompositeByteBuf) ReadByte() (byte, error) {
	if err := c.checkAlive(); err != nil {
		return 0, err
	}
	b, ok := c.GetByte(c.readerIndex)
	if !ok {
		return 0, ErrNotEnoughBytes
	}
	c.readerIndex++
	return b, nil
}

func (c *CompositeByteBuf) ReadUnsigned(offset int, length int, order ByteOrder) (uint64, error) {
	if err := c.checkAlive(); err != nil {
		return 0, err
	}
	if length <= 0 || length > 8 || offset < c.readerIndex || offset+length > c.writerIndex {
		return 0, ErrInvalidIndex
	}
	var v uint64
	if order == LittleEndian {
		for i := length - 1; i >= 0; i-- {
			b, ok := c.GetByte(offset + i)
			if !ok {
				return 0, ErrNotEnoughBytes
			}
			v = (v << 8) | uint64(b)
		}
		return v, nil
	}
	for i := 0; i < length; i++ {
		b, ok := c.GetByte(offset + i)
		if !ok {
			return 0, ErrNotEnoughBytes
		}
		v = (v << 8) | uint64(b)
	}
	return v, nil
}

func (c *CompositeByteBuf) Read(p []byte) (int, error) {
	if err := c.checkAlive(); err != nil {
		return 0, err
	}
	if c.ReadableBytes() == 0 {
		return 0, io.EOF
	}
	n := 0
	for n < len(p) && c.readerIndex < c.writerIndex {
		b, ok := c.GetByte(c.readerIndex)
		if !ok {
			break
		}
		p[n] = b
		n++
		c.readerIndex++
	}
	return n, nil
}

func (c *CompositeByteBuf) Write([]byte) (int, error) {
	return 0, ErrNoWritableBytes
}

func (c *CompositeByteBuf) WriteBytes([]byte) (int, error) {
	return 0, ErrNoWritableBytes
}

func (c *CompositeByteBuf) WriteTo(w io.Writer) (int64, error) {
	if err := c.checkAlive(); err != nil {
		return 0, err
	}
	total := int64(0)
	for c.readerIndex < c.writerIndex {
		comp := c.findComponent(c.readerIndex)
		if comp == nil {
			break
		}
		offset := comp.buf.ReaderIndex() + c.readerIndex - comp.start
		length := comp.end - c.readerIndex
		part, err := comp.buf.Slice(offset, length)
		if err != nil {
			return total, err
		}
		n, err := part.WriteTo(w)
		part.Release()
		c.readerIndex += int(n)
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func (c *CompositeByteBuf) ReadFrom(io.Reader) (int64, error) {
	return 0, ErrNoWritableBytes
}

func (c *CompositeByteBuf) Bytes() []byte {
	out, err := c.Copy()
	if err != nil {
		return nil
	}
	defer out.Release()
	return append([]byte(nil), out.Bytes()...)
}

func (c *CompositeByteBuf) WritableBytesView() []byte {
	return nil
}

func (c *CompositeByteBuf) AdvanceWriter(int) error {
	return ErrNoWritableBytes
}

func (c *CompositeByteBuf) SkipBytes(n int) error {
	if err := c.checkAlive(); err != nil {
		return err
	}
	if n < 0 || n > c.ReadableBytes() {
		return ErrNotEnoughBytes
	}
	c.readerIndex += n
	return nil
}

func (c *CompositeByteBuf) Slice(index int, length int) (ByteBuf, error) {
	if err := c.checkAlive(); err != nil {
		return nil, err
	}
	if index < c.readerIndex || length < 0 || index+length > c.writerIndex {
		return nil, ErrInvalidIndex
	}
	if length == 0 {
		return NewHeapBuffer(0), nil
	}
	if comp := c.singleComponent(index, index+length); comp != nil {
		return comp.buf.Slice(comp.buf.ReaderIndex()+index-comp.start, length)
	}
	out := NewCompositeByteBuf()
	end := index + length
	for i := range c.components {
		comp := &c.components[i]
		if comp.end <= index || comp.start >= end {
			continue
		}
		from := max(index, comp.start)
		to := min(end, comp.end)
		part, err := comp.buf.Slice(comp.buf.ReaderIndex()+from-comp.start, to-from)
		if err != nil {
			out.Release()
			return nil, err
		}
		out.Append(part)
	}
	return out, nil
}

func (c *CompositeByteBuf) Copy() (ByteBuf, error) {
	if err := c.checkAlive(); err != nil {
		return nil, err
	}
	out := NewHeapBuffer(c.ReadableBytes())
	for idx := c.readerIndex; idx < c.writerIndex; idx++ {
		b, ok := c.GetByte(idx)
		if !ok {
			out.Release()
			return nil, ErrInvalidIndex
		}
		_, _ = out.WriteBytes([]byte{b})
	}
	return out, nil
}

func (c *CompositeByteBuf) Retain() ByteBuf {
	for {
		refs := c.refs.Load()
		if refs <= 0 {
			panic(ErrReleasedBuffer)
		}
		if c.refs.CompareAndSwap(refs, refs+1) {
			return c
		}
	}
}

func (c *CompositeByteBuf) Release() bool {
	refs := c.refs.Add(-1)
	if refs > 0 {
		return false
	}
	if refs < 0 {
		panic(ErrReleasedBuffer)
	}
	for _, comp := range c.components {
		comp.buf.Release()
	}
	c.components = nil
	return true
}

func (c *CompositeByteBuf) RefCnt() int32 { return c.refs.Load() }

func (c *CompositeByteBuf) DiscardReadComponents() {
	if c.readerIndex == 0 || len(c.components) == 0 {
		return
	}
	drop := 0
	for drop < len(c.components) && c.components[drop].end <= c.readerIndex {
		c.components[drop].buf.Release()
		c.components[drop] = component{}
		drop++
	}
	if drop == 0 {
		return
	}
	base := c.readerIndex
	if drop == len(c.components) {
		c.components = c.components[:0]
	} else {
		copy(c.components, c.components[drop:])
		keep := len(c.components) - drop
		clear(c.components[keep:])
		c.components = c.components[:keep]
	}
	for i := range c.components {
		c.components[i].start -= base
		c.components[i].end -= base
	}
	c.writerIndex -= base
	c.readerIndex = 0
}

func (c *CompositeByteBuf) findComponent(index int) *component {
	for i := range c.components {
		if index >= c.components[i].start && index < c.components[i].end {
			return &c.components[i]
		}
	}
	return nil
}

func (c *CompositeByteBuf) singleComponent(start int, end int) *component {
	for i := range c.components {
		comp := &c.components[i]
		if start >= comp.start && end <= comp.end {
			return comp
		}
	}
	return nil
}
