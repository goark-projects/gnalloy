package ascii

import (
	"strings"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

// RequestDecoder 解码 Memcached ASCII request。
type RequestDecoder struct {
	*codec.ByteToMessageDecoder
	maxLineBytes  int
	maxValueBytes int
}

// NewRequestDecoder 创建 request decoder。
func NewRequestDecoder(maxLineBytes int, maxValueBytes int) (*RequestDecoder, error) {
	if maxLineBytes <= 0 || maxValueBytes < 0 {
		return nil, codec.ErrInvalidFrameLength
	}
	d := &RequestDecoder{maxLineBytes: maxLineBytes, maxValueBytes: maxValueBytes}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d, nil
}

// Decode 解码一条完整 ASCII request。
func (d *RequestDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	line, lineBytes, ok, err := findLine(in, d.maxLineBytes)
	if err != nil || !ok {
		return nil, err
	}
	req, needsValue, err := parseRequestLine(line)
	if err != nil {
		return nil, err
	}
	if !needsValue {
		if err := in.SkipBytes(lineBytes); err != nil {
			return nil, err
		}
		return req, nil
	}
	if req.Bytes < 0 || req.Bytes > d.maxValueBytes {
		return nil, codec.ErrFrameTooLong
	}
	total := lineBytes + req.Bytes + len(asciiCRLF)
	if in.ReadableBytes() < total {
		return nil, nil
	}
	reader := in.ReaderIndex()
	bodyEnd := reader + lineBytes + req.Bytes
	if !hasCRLFAt(in, bodyEnd) {
		return nil, codec.ErrInvalidFrameLength
	}
	if req.Bytes > 0 {
		req.Value, err = in.Slice(reader+lineBytes, req.Bytes)
		if err != nil {
			return nil, err
		}
	}
	if err := in.SkipBytes(total); err != nil {
		req.Release()
		return nil, err
	}
	return req, nil
}

func parseRequestLine(line string) (Request, bool, error) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return Request{}, false, codec.ErrInvalidFrameLength
	}
	req := Request{Command: commandLower(parts[0])}
	switch req.Command {
	case CommandGet, CommandGets:
		if len(parts) < 2 {
			return Request{}, false, codec.ErrInvalidFrameLength
		}
		req.Keys = append(req.Keys, parts[1:]...)
		return req, false, nil
	case CommandSet, CommandAdd, CommandReplace, CommandAppend, CommandPrepend:
		if len(parts) != 5 && len(parts) != 6 {
			return Request{}, false, codec.ErrInvalidFrameLength
		}
		if len(parts) == 6 && parts[5] != "noreply" {
			return Request{}, false, codec.ErrInvalidFrameLength
		}
		return parseStorageRequest(req, parts, len(parts) == 6)
	case CommandCAS:
		if len(parts) != 6 && len(parts) != 7 {
			return Request{}, false, codec.ErrInvalidFrameLength
		}
		if len(parts) == 7 && parts[6] != "noreply" {
			return Request{}, false, codec.ErrInvalidFrameLength
		}
		return parseCASRequest(req, parts, len(parts) == 7)
	case CommandDelete:
		if len(parts) != 2 && len(parts) != 3 {
			return Request{}, false, codec.ErrInvalidFrameLength
		}
		req.Key = parts[1]
		req.NoReply = len(parts) == 3 && parts[2] == "noreply"
		return req, false, nil
	case CommandIncr, CommandDecr:
		if len(parts) != 3 && len(parts) != 4 {
			return Request{}, false, codec.ErrInvalidFrameLength
		}
		delta, err := parseUint64Token(parts[2])
		if err != nil {
			return Request{}, false, codec.ErrInvalidFrameLength
		}
		req.Key = parts[1]
		req.Delta = delta
		req.NoReply = len(parts) == 4 && parts[3] == "noreply"
		return req, false, nil
	default:
		req.Arguments = append(req.Arguments, parts[1:]...)
		return req, false, nil
	}
}

func parseStorageRequest(req Request, parts []string, noReply bool) (Request, bool, error) {
	flags, err := parseUint32Token(parts[2])
	if err != nil {
		return Request{}, false, codec.ErrInvalidFrameLength
	}
	exptime, err := parseInt64Token(parts[3])
	if err != nil {
		return Request{}, false, codec.ErrInvalidFrameLength
	}
	bytes, err := parseIntToken(parts[4])
	if err != nil || bytes < 0 {
		return Request{}, false, codec.ErrInvalidFrameLength
	}
	req.Key = parts[1]
	req.Flags = flags
	req.Exptime = exptime
	req.Bytes = bytes
	req.NoReply = noReply
	return req, true, nil
}

func parseCASRequest(req Request, parts []string, noReply bool) (Request, bool, error) {
	req, _, err := parseStorageRequest(req, parts, noReply)
	if err != nil {
		return Request{}, false, err
	}
	cas, err := parseUint64Token(parts[5])
	if err != nil {
		return Request{}, false, codec.ErrInvalidFrameLength
	}
	req.CAS = cas
	return req, true, nil
}

func hasCRLFAt(in *buffer.CompositeByteBuf, index int) bool {
	cr, ok := in.GetByte(index)
	if !ok || cr != '\r' {
		return false
	}
	lf, ok := in.GetByte(index + 1)
	return ok && lf == '\n'
}
