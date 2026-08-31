package webtransport

import (
	"io"

	codechttp3 "goark.dev/gnalloy/codec/http3"
)

func writeBidirectionalPrefix(writer io.Writer, sessionID uint64) error {
	prefix, err := appendQUICVarInt(nil, uint64(codechttp3.FrameWTStream))
	if err != nil {
		return err
	}
	prefix, err = appendQUICVarInt(prefix, sessionID)
	if err != nil {
		return err
	}
	return writeAll(writer, prefix)
}

func writeUnidirectionalPrefix(writer io.Writer, sessionID uint64) error {
	prefix, err := appendQUICVarInt(nil, uint64(codechttp3.StreamTypeWebTransport))
	if err != nil {
		return err
	}
	prefix, err = appendQUICVarInt(prefix, sessionID)
	if err != nil {
		return err
	}
	return writeAll(writer, prefix)
}

func readBidirectionalPrefix(reader io.Reader, sessionID uint64) error {
	frameType, err := readVarIntFrom(reader)
	if err != nil {
		return err
	}
	gotSession, err := readVarIntFrom(reader)
	if err != nil {
		return err
	}
	if frameType != uint64(codechttp3.FrameWTStream) || gotSession != sessionID {
		return ErrInvalidStream
	}
	return nil
}

func readUnidirectionalPrefix(reader io.Reader, sessionID uint64) error {
	streamType, err := readVarIntFrom(reader)
	if err != nil {
		return err
	}
	gotSession, err := readVarIntFrom(reader)
	if err != nil {
		return err
	}
	if streamType != uint64(codechttp3.StreamTypeWebTransport) || gotSession != sessionID {
		return ErrInvalidStream
	}
	return nil
}

func readVarIntFrom(reader io.Reader) (uint64, error) {
	var buf [8]byte
	if _, err := io.ReadFull(reader, buf[:1]); err != nil {
		return 0, err
	}
	size := 1 << (buf[0] >> 6)
	if size > 1 {
		if _, err := io.ReadFull(reader, buf[1:size]); err != nil {
			return 0, err
		}
	}
	value, _, err := parseQUICVarInt(buf[:size])
	return value, err
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return ErrWriteUnsupported
		}
		data = data[n:]
	}
	return nil
}
