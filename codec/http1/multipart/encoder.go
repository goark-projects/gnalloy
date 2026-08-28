package multipart

import (
	"fmt"
	"io"
	"mime"
	stdmultipart "mime/multipart"
	"net/textproto"
	"sync"
)

const copyBufferSize = 32 << 10

var copyBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, copyBufferSize)
		return &buf
	},
}

// Encoder 以追加写入方式生成 multipart/form-data。
type Encoder struct {
	writer *stdmultipart.Writer
}

// NewEncoder 创建使用随机 boundary 的 Encoder。
func NewEncoder(writer io.Writer) (*Encoder, error) {
	if writer == nil {
		return nil, ErrInvalidConfig
	}
	return &Encoder{writer: stdmultipart.NewWriter(writer)}, nil
}

// NewEncoderWithBoundary 创建使用固定 boundary 的 Encoder，便于测试和协议互操作。
func NewEncoderWithBoundary(writer io.Writer, boundary string) (*Encoder, error) {
	encoder, err := NewEncoder(writer)
	if err != nil {
		return nil, err
	}
	if boundary == "" {
		return nil, ErrMissingBoundary
	}
	if err := encoder.writer.SetBoundary(boundary); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	return encoder, nil
}

// Boundary 返回当前 encoder 使用的 boundary。
func (e *Encoder) Boundary() string {
	return e.writer.Boundary()
}

// ContentType 返回可直接写入 HTTP Header 的 Content-Type。
func (e *Encoder) ContentType() string {
	return e.writer.FormDataContentType()
}

// WriteField 写入普通 form-data 字段。
func (e *Encoder) WriteField(name string, value string) error {
	return e.writer.WriteField(name, value)
}

// WritePart 写入一个自定义 part，并返回复制的正文长度。
func (e *Encoder) WritePart(info PartInfo, body io.Reader) (int64, error) {
	if info.Headers == nil {
		info.Headers = FormDataHeader(info.Name, info.FileName, "")
	}
	part, err := e.writer.CreatePart(info.Headers)
	if err != nil {
		return 0, err
	}
	if body == nil {
		return 0, nil
	}
	bufp := copyBufferPool.Get().(*[]byte)
	buf := *bufp
	n, err := io.CopyBuffer(part, body, buf)
	copyBufferPool.Put(bufp)
	return n, err
}

// Close 写入最终 boundary。
func (e *Encoder) Close() error {
	return e.writer.Close()
}

// FormDataHeader 构造标准 form-data part 头部。
func FormDataHeader(name string, fileName string, contentType string) textproto.MIMEHeader {
	headers := textproto.MIMEHeader{}
	params := map[string]string{"name": name}
	if fileName != "" {
		params["filename"] = fileName
	}
	headers.Set("Content-Disposition", mime.FormatMediaType("form-data", params))
	if contentType != "" {
		headers.Set("Content-Type", contentType)
	}
	return headers
}
