package content

import (
	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/codec/compression"
	"goark.dev/gnalloy/codec/http2"
	h2chunked "goark.dev/gnalloy/codec/http2/chunked"
)

// DataCompressingInputConfig 描述 HTTP/2 DATA 流式压缩参数。
type DataCompressingInputConfig struct {
	Level     int
	ChunkSize int
}

// NewDataCompressingInput 创建压缩后的 HTTP/2 DATA frame 分片输入。
func NewDataCompressingInput(streamID http2.StreamID, input codec.ChunkedInput, coding Coding, cfg DataCompressingInputConfig, endStream bool) (*h2chunked.DataChunkedInput, error) {
	format, ok := codingFormat(coding)
	if !ok {
		if input != nil {
			_ = input.Close()
		}
		return nil, codec.ErrInvalidFrameLength
	}
	compressed, err := compression.NewCompressingChunkedInput(input, compression.ChunkedEncoderConfig{
		Format:    format,
		Level:     cfg.Level,
		ChunkSize: cfg.ChunkSize,
	})
	if err != nil {
		return nil, err
	}
	return h2chunked.NewDataChunkedInput(streamID, compressed, endStream)
}

func codingFormat(coding Coding) (compression.Format, bool) {
	switch normalizeCoding(string(coding)) {
	case CodingGzip:
		return compression.FormatGzip, true
	case CodingDeflate:
		return compression.FormatZlib, true
	default:
		return 0, false
	}
}
