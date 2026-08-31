package content

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

const defaultMaxDecodedBytes = 16 << 20

// Coding 表示 HTTP Content-Encoding 算法。
type Coding string

const (
	// CodingGzip 表示 gzip Content-Encoding。
	CodingGzip Coding = "gzip"
	// CodingDeflate 表示 zlib-wrapped deflate Content-Encoding。
	CodingDeflate Coding = "deflate"
)

func normalizeCodings(codings []Coding) []Coding {
	out := make([]Coding, 0, len(codings))
	for _, coding := range codings {
		coding = normalizeCoding(string(coding))
		if isSupportedCoding(coding) {
			out = append(out, coding)
		}
	}
	if len(out) == 0 {
		return []Coding{CodingGzip, CodingDeflate}
	}
	return out
}

func normalizeCoding(value string) Coding {
	return Coding(strings.ToLower(strings.TrimSpace(value)))
}

func isSupportedCoding(coding Coding) bool {
	return coding == CodingGzip || coding == CodingDeflate
}

func chooseCoding(header string, preferences []Coding) Coding {
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

func encodeBody(ctx *channel.HandlerContext, body buffer.ByteBuf, coding Coding) (buffer.ByteBuf, error) {
	var out bytes.Buffer
	writer, err := newContentWriter(&out, coding)
	if err != nil {
		return nil, err
	}
	if err := writeBody(writer, body); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return byteBufFromBytes(ctx, out.Bytes())
}

func decodeBody(ctx *channel.HandlerContext, body buffer.ByteBuf, coding Coding, maxDecodedBytes int) (buffer.ByteBuf, error) {
	if maxDecodedBytes <= 0 {
		maxDecodedBytes = defaultMaxDecodedBytes
	}
	raw, err := body.Copy()
	if err != nil {
		return nil, err
	}
	defer raw.Release()
	reader, err := newContentReader(bytes.NewReader(raw.Bytes()), coding)
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
	return byteBufFromBytes(ctx, decoded)
}

func newContentWriter(out io.Writer, coding Coding) (io.WriteCloser, error) {
	switch coding {
	case CodingGzip:
		return gzip.NewWriter(out), nil
	case CodingDeflate:
		return zlib.NewWriter(out), nil
	default:
		return nil, codec.ErrInvalidFrameLength
	}
}

func newContentReader(in io.Reader, coding Coding) (io.ReadCloser, error) {
	switch coding {
	case CodingGzip:
		return gzip.NewReader(in)
	case CodingDeflate:
		return zlib.NewReader(in)
	default:
		return nil, codec.ErrInvalidFrameLength
	}
}

func writeBody(w io.Writer, body buffer.ByteBuf) error {
	if body == nil {
		return nil
	}
	var scratch [8][]byte
	for _, part := range body.ReadableSlices(scratch[:0]) {
		if len(part) == 0 {
			continue
		}
		if _, err := w.Write(part); err != nil {
			return err
		}
	}
	return nil
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
