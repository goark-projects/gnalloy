package multipart

import (
	"bytes"
	"fmt"
	"io"
	stdmultipart "mime/multipart"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/codec/http1"
)

// Decoder 基于 boundary 解析 multipart body。
type Decoder struct {
	boundary string
	limits   Limits
}

// NewDecoder 使用显式 boundary 创建 Decoder。
func NewDecoder(boundary string, limits Limits) (*Decoder, error) {
	if boundary == "" {
		return nil, ErrMissingBoundary
	}
	normalized, err := normalizeLimits(limits)
	if err != nil {
		return nil, err
	}
	return &Decoder{boundary: boundary, limits: normalized}, nil
}

// NewDecoderFromContentType 从 Content-Type 创建 Decoder。
func NewDecoderFromContentType(contentType string, limits Limits) (*Decoder, error) {
	boundary, err := ParseBoundary(contentType)
	if err != nil {
		return nil, err
	}
	return NewDecoder(boundary, limits)
}

// Decode 以流式方式解析 reader，handler 必须在返回前消费或复制 body。
func (d *Decoder) Decode(reader io.Reader, handler PartHandler) error {
	if reader == nil || handler == nil {
		return ErrInvalidConfig
	}
	mr := stdmultipart.NewReader(reader, d.boundary)
	var total int64
	for partIndex := 0; ; partIndex++ {
		part, err := mr.NextPart()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if partIndex >= d.limits.MaxParts {
			_ = part.Close()
			return fmt.Errorf("%w: parts", ErrLimitExceeded)
		}
		info, err := partInfo(part, d.limits.MaxHeaderBytes)
		if err != nil {
			_ = part.Close()
			return err
		}
		body := &limitedPartReader{
			reader:        part,
			maxPartBytes:  d.limits.MaxPartBytes,
			maxTotalBytes: d.limits.MaxTotalBytes,
			total:         &total,
		}
		if err := handler.HandlePart(info, body); err != nil {
			_ = part.Close()
			return err
		}
		if _, err := io.Copy(io.Discard, body); err != nil {
			_ = part.Close()
			return err
		}
		if err := part.Close(); err != nil {
			return err
		}
	}
}

// DecodeBytes 将完整 body 解码为内存 part，适合小型表单。
func (d *Decoder) DecodeBytes(body []byte) ([]Part, error) {
	parts := make([]Part, 0, 4)
	err := d.Decode(bytes.NewReader(body), PartHandlerFunc(func(info PartInfo, reader io.Reader) error {
		var out bytes.Buffer
		if _, err := io.Copy(&out, reader); err != nil {
			return err
		}
		parts = append(parts, Part{PartInfo: info, Data: out.Bytes()})
		return nil
	}))
	if err != nil {
		return nil, err
	}
	return parts, nil
}

// DecodeBuffer 以零拷贝视图读取 ByteBuf 可读区并解码为内存 part。
func (d *Decoder) DecodeBuffer(body buffer.ByteBuf) ([]Part, error) {
	if body == nil {
		return nil, ErrMissingBody
	}
	return d.DecodeBytes(body.Bytes())
}

// DecodeRequest 使用 Request 的 Content-Type 和 Body 解码 multipart/form-data。
func DecodeRequest(req http1.Request, limits Limits) ([]Part, error) {
	decoder, err := NewDecoderFromContentType(req.Headers.Get("Content-Type"), limits)
	if err != nil {
		return nil, err
	}
	return decoder.DecodeBuffer(req.Body)
}

// StreamRequest 以流式 handler 解析 Request 的 multipart/form-data。
func StreamRequest(req http1.Request, limits Limits, handler PartHandler) error {
	if req.Body == nil {
		return ErrMissingBody
	}
	decoder, err := NewDecoderFromContentType(req.Headers.Get("Content-Type"), limits)
	if err != nil {
		return err
	}
	return decoder.Decode(bytes.NewReader(req.Body.Bytes()), handler)
}

func partInfo(part *stdmultipart.Part, maxHeaderBytes int64) (PartInfo, error) {
	if headerBytes(part.Header) > maxHeaderBytes {
		return PartInfo{}, fmt.Errorf("%w: headers", ErrLimitExceeded)
	}
	return PartInfo{
		Headers:  part.Header,
		Name:     part.FormName(),
		FileName: part.FileName(),
	}, nil
}

func headerBytes(headers map[string][]string) int64 {
	var total int64
	for name, values := range headers {
		total += int64(len(name) + 2)
		for _, value := range values {
			total += int64(len(value) + 2)
		}
	}
	return total
}

type limitedPartReader struct {
	reader        io.Reader
	maxPartBytes  int64
	maxTotalBytes int64
	total         *int64
	partBytes     int64
}

func (r *limitedPartReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	remaining := r.remaining()
	if remaining < int64(len(p)) {
		p = p[:remaining+1]
	}
	n, err := r.reader.Read(p)
	if n > 0 {
		r.partBytes += int64(n)
		*r.total += int64(n)
		if r.partBytes > r.maxPartBytes || *r.total > r.maxTotalBytes {
			return n, fmt.Errorf("%w: body", ErrLimitExceeded)
		}
	}
	return n, err
}

func (r *limitedPartReader) remaining() int64 {
	partRemaining := r.maxPartBytes - r.partBytes
	totalRemaining := r.maxTotalBytes - *r.total
	if partRemaining < totalRemaining {
		return partRemaining
	}
	return totalRemaining
}
