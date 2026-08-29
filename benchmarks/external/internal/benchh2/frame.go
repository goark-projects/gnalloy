package benchh2

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	frameData         byte = 0x0
	frameHeaders      byte = 0x1
	frameSettings     byte = 0x4
	framePing         byte = 0x6
	frameWindowUpdate byte = 0x8

	flagEndStream  byte = 0x1
	flagAck        byte = 0x1
	flagEndHeaders byte = 0x4

	frameHeaderSize          = 9
	maxFramePayload          = 16*1024*1024 - 1
	maxClientStreamCount     = (1<<30 - 1)
	connectionWindowIncrease = 1<<30 - 1
)

var clientPreface = []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")

type frameHeader struct {
	length   int
	typ      byte
	flags    byte
	streamID uint32
}

func readFrameHeader(r io.Reader) (frameHeader, error) {
	var raw [frameHeaderSize]byte
	if _, err := io.ReadFull(r, raw[:]); err != nil {
		return frameHeader{}, err
	}
	length := int(raw[0])<<16 | int(raw[1])<<8 | int(raw[2])
	streamID := binary.BigEndian.Uint32(raw[5:9]) & 0x7fffffff
	return frameHeader{length: length, typ: raw[3], flags: raw[4], streamID: streamID}, nil
}

func writeFrame(w io.Writer, typ byte, flags byte, streamID uint32, payload []byte) error {
	if len(payload) > maxFramePayload {
		return fmt.Errorf("benchh2: frame payload too large")
	}
	var header [frameHeaderSize]byte
	header[0] = byte(len(payload) >> 16)
	header[1] = byte(len(payload) >> 8)
	header[2] = byte(len(payload))
	header[3] = typ
	header[4] = flags
	binary.BigEndian.PutUint32(header[5:9], streamID&0x7fffffff)
	if err := writeAll(w, header[:]); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	return writeAll(w, payload)
}

func writeSettingsAck(w io.Writer) error {
	return writeFrame(w, frameSettings, flagAck, 0, nil)
}

func writeConnectionWindowUpdate(w io.Writer) error {
	var payload [4]byte
	binary.BigEndian.PutUint32(payload[:], connectionWindowIncrease)
	return writeFrame(w, frameWindowUpdate, 0, 0, payload[:])
}

func skipFramePayload(r io.Reader, length int) error {
	if length == 0 {
		return nil
	}
	_, err := io.CopyN(io.Discard, r, int64(length))
	return err
}

func writeAll(w io.Writer, src []byte) error {
	for len(src) > 0 {
		n, err := w.Write(src)
		if n > 0 {
			src = src[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
