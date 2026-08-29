package compression

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"sync"
)

const maxRetainedCompressionBuffer = 1 << 20

type resetWriteCloser interface {
	io.WriteCloser
	Reset(io.Writer)
}

type compressionWriterState struct {
	dst    bytes.Buffer
	writer resetWriteCloser
}

type compressionWriterPool struct {
	format Format
	level  int
	pool   sync.Pool
}

func newCompressionWriterPool(format Format, level int) *compressionWriterPool {
	return &compressionWriterPool{format: format, level: level}
}

func (p *compressionWriterPool) acquire() (*compressionWriterState, error) {
	if value := p.pool.Get(); value != nil {
		state := value.(*compressionWriterState)
		state.dst.Reset()
		state.writer.Reset(&state.dst)
		return state, nil
	}
	state := &compressionWriterState{}
	writer, err := newCompressionWriter(p.format, p.level, &state.dst)
	if err != nil {
		return nil, err
	}
	state.writer = writer
	return state, nil
}

func (p *compressionWriterPool) release(state *compressionWriterState) {
	if state == nil {
		return
	}
	if state.dst.Cap() > maxRetainedCompressionBuffer {
		state.dst = bytes.Buffer{}
	} else {
		state.dst.Reset()
	}
	p.pool.Put(state)
}

func newCompressionWriter(format Format, level int, dst io.Writer) (resetWriteCloser, error) {
	switch format {
	case FormatGzip:
		return gzip.NewWriterLevel(dst, level)
	case FormatZlib:
		return zlib.NewWriterLevel(dst, level)
	default:
		return nil, ErrInvalidConfig
	}
}

type resetReadCloser interface {
	io.ReadCloser
	Reset(io.Reader) error
}

type compressionReaderPool struct {
	format Format
	pool   sync.Pool
}

func newCompressionReaderPool(format Format) *compressionReaderPool {
	return &compressionReaderPool{format: format}
}

func (p *compressionReaderPool) acquire(src io.Reader) (resetReadCloser, error) {
	if value := p.pool.Get(); value != nil {
		reader := value.(resetReadCloser)
		if err := reader.Reset(src); err != nil {
			_ = reader.Close()
			return nil, err
		}
		return reader, nil
	}
	return newCompressionReader(p.format, src)
}

func (p *compressionReaderPool) release(reader resetReadCloser) {
	if reader == nil {
		return
	}
	_ = reader.Close()
	p.pool.Put(reader)
}

func newCompressionReader(format Format, src io.Reader) (resetReadCloser, error) {
	switch format {
	case FormatGzip:
		return gzip.NewReader(src)
	case FormatZlib:
		reader, err := zlib.NewReader(src)
		if err != nil {
			return nil, err
		}
		resetter, ok := reader.(zlib.Resetter)
		if !ok {
			_ = reader.Close()
			return nil, ErrInvalidConfig
		}
		return &zlibResetReader{ReadCloser: reader, resetter: resetter}, nil
	default:
		return nil, ErrInvalidConfig
	}
}

type zlibResetReader struct {
	io.ReadCloser
	resetter zlib.Resetter
}

func (r *zlibResetReader) Reset(src io.Reader) error {
	return r.resetter.Reset(src, nil)
}
