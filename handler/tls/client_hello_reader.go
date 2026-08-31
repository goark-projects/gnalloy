package tls

type byteReader struct {
	data []byte
	pos  int
}

func (r *byteReader) remaining() int {
	return len(r.data) - r.pos
}

func (r *byteReader) has(n int) bool {
	return n >= 0 && r.remaining() >= n
}

func (r *byteReader) skip(n int) bool {
	if !r.has(n) {
		return false
	}
	r.pos += n
	return true
}

func (r *byteReader) take(n int) ([]byte, bool) {
	if !r.has(n) {
		return nil, false
	}
	out := r.data[r.pos : r.pos+n]
	r.pos += n
	return out, true
}

func (r *byteReader) tail() []byte {
	return r.data[r.pos:]
}

func (r *byteReader) u8() (uint8, bool) {
	data, ok := r.take(1)
	if !ok {
		return 0, false
	}
	return data[0], true
}

func (r *byteReader) u16() (uint16, bool) {
	data, ok := r.take(2)
	if !ok {
		return 0, false
	}
	return uint16(data[0])<<8 | uint16(data[1]), true
}
