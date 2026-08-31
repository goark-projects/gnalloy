package http1

import (
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/codec/compression"
)

// ContentEncodingInputConfig 描述 HTTP/1 大 body 流式压缩参数。
type ContentEncodingInputConfig struct {
	Level     int
	ChunkSize int
}

// NewContentEncodingInput 创建可直接交给 codec.ChunkedWriteHandler 的压缩输入。
func NewContentEncodingInput(input codec.ChunkedInput, coding ContentCoding, cfg ContentEncodingInputConfig) (*compression.CompressingChunkedInput, error) {
	format, ok := contentCodingFormat(coding)
	if !ok {
		if input != nil {
			_ = input.Close()
		}
		return nil, codec.ErrInvalidFrameLength
	}
	return compression.NewCompressingChunkedInput(input, compression.ChunkedEncoderConfig{
		Format:    format,
		Level:     cfg.Level,
		ChunkSize: cfg.ChunkSize,
	})
}

func contentCodingFormat(coding ContentCoding) (compression.Format, bool) {
	switch normalizeContentCoding(string(coding)) {
	case ContentCodingGzip:
		return compression.FormatGzip, true
	case ContentCodingDeflate:
		return compression.FormatZlib, true
	default:
		return 0, false
	}
}
