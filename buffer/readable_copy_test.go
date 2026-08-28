package buffer

import "testing"

func TestCopyReadableBytesCopiesDirectWithoutAdvancing(t *testing.T) {
	buf := NewHeapBuffer(8)
	_, _ = buf.WriteBytes([]byte("abcdef"))
	if err := buf.SkipBytes(2); err != nil {
		t.Fatal(err)
	}
	defer buf.Release()

	dst := make([]byte, buf.ReadableBytes())
	n := CopyReadableBytes(dst, buf)
	if n != 4 || string(dst) != "cdef" {
		t.Fatalf("n=%d dst=%q, want cdef", n, dst)
	}
	if got := buf.ReaderIndex(); got != 2 {
		t.Fatalf("readerIndex=%d, want 2", got)
	}
}

func TestCopyReadableBytesCopiesCompositeWithoutAdvancing(t *testing.T) {
	first := NewHeapBuffer(4)
	second := NewHeapBuffer(4)
	_, _ = first.WriteBytes([]byte("abcd"))
	_, _ = second.WriteBytes([]byte("efgh"))
	composite := NewCompositeByteBuf()
	composite.Append(first)
	composite.Append(second)
	if err := composite.SkipBytes(3); err != nil {
		t.Fatal(err)
	}
	defer composite.Release()

	dst := make([]byte, composite.ReadableBytes())
	n := CopyReadableBytes(dst, composite)
	if n != 5 || string(dst) != "defgh" {
		t.Fatalf("n=%d dst=%q, want defgh", n, dst)
	}
	if got := composite.ReaderIndex(); got != 3 {
		t.Fatalf("readerIndex=%d, want 3", got)
	}
}
