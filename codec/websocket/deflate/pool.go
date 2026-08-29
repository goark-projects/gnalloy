package deflate

import (
	"bytes"
	"compress/flate"
	"io"
	"sync"
)

const maxRetainedMessageBuffer = 1 << 20

var (
	messageWriters sync.Pool
	messageReaders sync.Pool
)

type messageWriterState struct {
	level  int
	dst    bytes.Buffer
	writer *flate.Writer
}

func acquireMessageWriter(level int) (*messageWriterState, error) {
	if value := messageWriters.Get(); value != nil {
		state := value.(*messageWriterState)
		if state.level == level {
			state.dst.Reset()
			state.writer.Reset(&state.dst)
			return state, nil
		}
	}
	state := &messageWriterState{level: level}
	writer, err := flate.NewWriter(&state.dst, level)
	if err != nil {
		return nil, err
	}
	state.writer = writer
	return state, nil
}

func releaseMessageWriter(state *messageWriterState) {
	if state == nil {
		return
	}
	if state.dst.Cap() > maxRetainedMessageBuffer {
		state.dst = bytes.Buffer{}
	} else {
		state.dst.Reset()
	}
	messageWriters.Put(state)
}

type messageReadCloser interface {
	io.ReadCloser
	Reset(io.Reader) error
}

func acquireMessageReader(src io.Reader) (messageReadCloser, error) {
	if value := messageReaders.Get(); value != nil {
		reader := value.(messageReadCloser)
		if err := reader.Reset(src); err != nil {
			_ = reader.Close()
			return nil, err
		}
		return reader, nil
	}
	reader := flate.NewReader(src)
	resetter, ok := reader.(flate.Resetter)
	if !ok {
		_ = reader.Close()
		return nil, ErrInvalidConfig
	}
	return &messageResetReader{ReadCloser: reader, resetter: resetter}, nil
}

func releaseMessageReader(reader messageReadCloser) {
	if reader == nil {
		return
	}
	_ = reader.Close()
	messageReaders.Put(reader)
}

type messageResetReader struct {
	io.ReadCloser
	resetter flate.Resetter
}

func (r *messageResetReader) Reset(src io.Reader) error {
	return r.resetter.Reset(src, nil)
}

type syncTailReader struct {
	src    []byte
	offset int
}

func newSyncTailReader(src []byte) *syncTailReader {
	return &syncTailReader{src: src}
}

func (r *syncTailReader) Read(dst []byte) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}
	written := 0
	if r.offset < len(r.src) {
		n := copy(dst, r.src[r.offset:])
		written += n
		r.offset += n
	}
	tailOffset := r.offset - len(r.src)
	if written < len(dst) && tailOffset >= 0 && tailOffset < len(syncFlushTail) {
		n := copy(dst[written:], syncFlushTail[tailOffset:])
		written += n
		r.offset += n
	}
	if written == 0 {
		return 0, io.EOF
	}
	return written, nil
}
