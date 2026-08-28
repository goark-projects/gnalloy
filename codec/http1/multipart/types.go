package multipart

import (
	"io"
	"net/textproto"
)

const (
	defaultMaxParts       = 128
	defaultMaxHeaderBytes = 64 << 10
	defaultMaxPartBytes   = 32 << 20
	defaultMaxTotalBytes  = 128 << 20
)

// Limits 描述 multipart 解析时的内存与对象数量预算，0 表示使用安全默认值。
type Limits struct {
	MaxParts       int
	MaxHeaderBytes int64
	MaxPartBytes   int64
	MaxTotalBytes  int64
}

func normalizeLimits(limits Limits) (Limits, error) {
	if limits.MaxParts < 0 || limits.MaxHeaderBytes < 0 || limits.MaxPartBytes < 0 || limits.MaxTotalBytes < 0 {
		return Limits{}, ErrInvalidConfig
	}
	if limits.MaxParts == 0 {
		limits.MaxParts = defaultMaxParts
	}
	if limits.MaxHeaderBytes == 0 {
		limits.MaxHeaderBytes = defaultMaxHeaderBytes
	}
	if limits.MaxPartBytes == 0 {
		limits.MaxPartBytes = defaultMaxPartBytes
	}
	if limits.MaxTotalBytes == 0 {
		limits.MaxTotalBytes = defaultMaxTotalBytes
	}
	return limits, nil
}

// PartInfo 描述一个 multipart part 的头部和 form-data 名称。
type PartInfo struct {
	Headers  textproto.MIMEHeader
	Name     string
	FileName string
}

// Part 表示已完整载入内存的 multipart part。
type Part struct {
	PartInfo
	Data []byte
}

// IsFile 返回该 part 是否表示上传文件。
func (p PartInfo) IsFile() bool {
	return p.FileName != ""
}

// Header 返回首个匹配的 MIME 头部值。
func (p PartInfo) Header(name string) string {
	return p.Headers.Get(name)
}

// PartHandler 以流式方式消费 multipart part。
type PartHandler interface {
	HandlePart(info PartInfo, body io.Reader) error
}

// PartHandlerFunc 将函数适配为 PartHandler。
type PartHandlerFunc func(info PartInfo, body io.Reader) error

func (f PartHandlerFunc) HandlePart(info PartInfo, body io.Reader) error {
	return f(info, body)
}
