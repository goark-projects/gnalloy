package buffer

import "testing"

func TestDirectByteBufSliceRetainsParent(t *testing.T) {
	buf := NewHeapBuffer(16)
	if _, err := buf.WriteBytes([]byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	slice, err := buf.Slice(1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if buf.RefCnt() != 2 {
		t.Fatalf("parent ref=%d, want 2", buf.RefCnt())
	}
	if string(slice.Bytes()) != "bcd" {
		t.Fatalf("slice=%q", slice.Bytes())
	}
	if buf.Release() {
		t.Fatal("parent should still be retained by slice")
	}
	if string(slice.Bytes()) != "bcd" {
		t.Fatalf("slice after parent release=%q", slice.Bytes())
	}
	if !slice.Release() {
		t.Fatal("slice release should drop slice itself")
	}
}

func TestCompositeSliceAcrossComponents(t *testing.T) {
	a := NewHeapBuffer(8)
	b := NewHeapBuffer(8)
	_, _ = a.WriteBytes([]byte("abc"))
	_, _ = b.WriteBytes([]byte("def"))

	c := NewCompositeByteBuf()
	c.Append(a)
	c.Append(b)

	frame, err := c.Slice(2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if string(frame.Bytes()) != "cde" {
		t.Fatalf("frame=%q", frame.Bytes())
	}
	if err := c.SkipBytes(3); err != nil {
		t.Fatal(err)
	}
	c.DiscardReadComponents()
	if c.ReaderIndex() != 0 || c.ReadableBytes() != 3 {
		t.Fatalf("reader=%d readable=%d", c.ReaderIndex(), c.ReadableBytes())
	}
	frame.Release()
	c.Release()
}

func TestReadUnsignedWithoutCopy(t *testing.T) {
	a := NewHeapBuffer(2)
	b := NewHeapBuffer(2)
	_, _ = a.WriteBytes([]byte{0x01, 0x02})
	_, _ = b.WriteBytes([]byte{0x03, 0x04})

	c := NewCompositeByteBuf()
	c.Append(a)
	c.Append(b)
	got, err := c.ReadUnsigned(1, 3, BigEndian)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0x020304 {
		t.Fatalf("got=%x", got)
	}
	c.Release()
}

func TestCompositeAppendRetainedKeepsCallerOwnership(t *testing.T) {
	buf := NewHeapBuffer(8)
	_, _ = buf.WriteBytes([]byte("abc"))

	c := NewCompositeByteBuf()
	c.AppendRetained(buf)
	if buf.RefCnt() != 2 {
		t.Fatalf("ref=%d, want 2", buf.RefCnt())
	}
	if c.ComponentCount() != 1 || !c.IsContiguous() {
		t.Fatalf("components=%d contiguous=%v", c.ComponentCount(), c.IsContiguous())
	}
	c.Release()
	if buf.RefCnt() != 1 {
		t.Fatalf("ref after composite release=%d, want 1", buf.RefCnt())
	}
	buf.Release()
}

func TestCompositeReadableSlicesAreViews(t *testing.T) {
	a := NewHeapBuffer(8)
	b := NewHeapBuffer(8)
	_, _ = a.WriteBytes([]byte("ab"))
	_, _ = b.WriteBytes([]byte("cd"))

	c := NewCompositeByteBuf()
	c.Append(a)
	c.Append(b)
	slices := c.ReadableSlices(nil)
	if len(slices) != 2 {
		t.Fatalf("slices=%d, want 2", len(slices))
	}
	slices[0][0] = 'A'
	slices[1][1] = 'D'
	if got := string(c.Bytes()); got != "AbcD" {
		t.Fatalf("bytes=%q", got)
	}
	c.Release()
}

func TestCompositeAppendAfterReleaseDropsInput(t *testing.T) {
	c := NewCompositeByteBuf()
	c.Release()

	buf := NewHeapBuffer(4)
	_, _ = buf.WriteBytes([]byte("x"))
	c.Append(buf)
	if buf.RefCnt() != 0 {
		t.Fatalf("ref=%d, want 0", buf.RefCnt())
	}
}
