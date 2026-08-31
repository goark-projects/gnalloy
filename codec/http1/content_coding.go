package http1

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"strconv"
	"strings"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

const (
	DefaultMaxDecodedContentBytes = 16 << 20
)

// ContentCoding 表示 HTTP Content-Encoding 算法。
type ContentCoding string

const (
	ContentCodingGzip    ContentCoding = "gzip"
	ContentCodingDeflate ContentCoding = "deflate"
)

func encodeContent(ctx *channel.HandlerContext, body buffer.ByteBuf, coding ContentCoding) (buffer.ByteBuf, error) {
	var out bytes.Buffer
	writer, err := newContentWriter(&out, coding)
	if err != nil {
		return nil, err
	}
	if _, err := writer.Write(body.Bytes()); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return byteBufFromBytes(ctx, out.Bytes())
}

func decodeContent(body buffer.ByteBuf, coding ContentCoding, maxDecodedBytes int) ([]byte, error) {
	reader, err := newContentReader(bytes.NewReader(body.Bytes()), coding)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	limited := io.LimitReader(reader, int64(maxDecodedBytes)+1)
	decoded, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(decoded) > maxDecodedBytes {
		return nil, codec.ErrFrameTooLong
	}
	return decoded, nil
}

func newContentWriter(out io.Writer, coding ContentCoding) (io.WriteCloser, error) {
	switch coding {
	case ContentCodingGzip:
		return gzip.NewWriter(out), nil
	case ContentCodingDeflate:
		return zlib.NewWriter(out), nil
	default:
		return nil, codec.ErrInvalidFrameLength
	}
}

func newContentReader(in io.Reader, coding ContentCoding) (io.ReadCloser, error) {
	switch coding {
	case ContentCodingGzip:
		return gzip.NewReader(in)
	case ContentCodingDeflate:
		return zlib.NewReader(in)
	default:
		return nil, codec.ErrInvalidFrameLength
	}
}

func byteBufFromBytes(ctx *channel.HandlerContext, src []byte) (buffer.ByteBuf, error) {
	if len(src) == 0 {
		return nil, nil
	}
	out, err := ctx.Channel().Allocator().Acquire(len(src))
	if err != nil {
		return nil, err
	}
	if _, err := out.WriteBytes(src); err != nil {
		out.Release()
		return nil, err
	}
	return out, nil
}

func readableBytes(body buffer.ByteBuf) int {
	if body == nil {
		return 0
	}
	return body.ReadableBytes()
}

func normalizeContentCodings(codings []ContentCoding) []ContentCoding {
	out := make([]ContentCoding, 0, len(codings))
	for _, coding := range codings {
		coding = normalizeContentCoding(string(coding))
		if isSupportedContentCoding(coding) {
			out = append(out, coding)
		}
	}
	if len(out) == 0 {
		return []ContentCoding{ContentCodingGzip, ContentCodingDeflate}
	}
	return out
}

func normalizeContentCoding(value string) ContentCoding {
	return ContentCoding(strings.ToLower(strings.TrimSpace(value)))
}

func isSupportedContentCoding(coding ContentCoding) bool {
	return coding == ContentCodingGzip || coding == ContentCodingDeflate
}

func chooseContentCoding(header string, preferences []ContentCoding) ContentCoding {
	accepted := parseAcceptEncoding(header)
	for _, coding := range preferences {
		if q, ok := accepted[string(coding)]; ok {
			if q > 0 {
				return coding
			}
			continue
		}
		if accepted["*"] > 0 {
			return coding
		}
	}
	return ""
}

func parseAcceptEncoding(header string) map[string]float64 {
	accepted := make(map[string]float64, 4)
	for part := range strings.SplitSeq(header, ",") {
		token, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" {
			continue
		}
		accepted[token] = acceptEncodingQ(params)
	}
	return accepted
}

func acceptEncodingQ(params string) float64 {
	if params == "" {
		return 1
	}
	for part := range strings.SplitSeq(params, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "q") {
			continue
		}
		q, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 0
		}
		return q
	}
	return 1
}
