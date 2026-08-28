package zerocopy

import (
	"errors"
	"io"
	"os"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

const defaultChunkSize = 1 << 20

// Result 描述一次文件区域传输结果。
type Result struct {
	Bytes    int64
	ZeroCopy bool
}

// Sender 持有单次 sendfile/copy 的最大传输块大小。
type Sender struct {
	chunkSize int
}

// NewSender 创建零拷贝传输器，chunkSize 为 0 时使用默认 1MiB。
func NewSender(chunkSize int) (Sender, error) {
	if chunkSize < 0 {
		return Sender{}, ErrInvalidConfig
	}
	if chunkSize == 0 {
		chunkSize = defaultChunkSize
	}
	return Sender{chunkSize: chunkSize}, nil
}

// SendFile 使用默认 Sender 传输文件区域。
func SendFile(dst transport.FDRef, region channel.FileRegion) (Result, bool, error) {
	sender, err := NewSender(0)
	if err != nil {
		return Result{}, false, err
	}
	return sender.SendFile(dst, region)
}

// SendFile 尝试使用平台原生 sendfile，again 表示非阻塞 fd 暂不可写。
func (s Sender) SendFile(dst transport.FDRef, region channel.FileRegion) (Result, bool, error) {
	if !dst.Valid() {
		return Result{}, false, transport.ErrInvalidFD
	}
	if err := validateRegion(region); err != nil {
		return Result{}, false, err
	}
	if region.Transferred() >= region.Count() {
		return Result{}, false, nil
	}
	return sendFile(dst, region, s.chunkSize)
}

// Copy 使用普通 reader/write fallback 传输 FileRegion，调用方仍持有 region 生命周期。
func Copy(writer io.Writer, region channel.FileRegion, chunkSize int) (Result, error) {
	if writer == nil {
		return Result{}, ErrInvalidConfig
	}
	sender, err := NewSender(chunkSize)
	if err != nil {
		return Result{}, err
	}
	if err := validateRegion(region); err != nil {
		return Result{}, err
	}
	buf := make([]byte, sender.chunkSize)
	var written int64
	for region.Transferred() < region.Count() {
		remaining := region.Count() - region.Transferred()
		dst := buf
		if int64(len(dst)) > remaining {
			dst = dst[:remaining]
		}
		n, readErr := region.Read(dst)
		if n > 0 {
			if err := writeAll(writer, dst[:n]); err != nil {
				return Result{Bytes: written, ZeroCopy: false}, err
			}
			written += int64(n)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) && region.Transferred() >= region.Count() {
				break
			}
			return Result{Bytes: written, ZeroCopy: false}, readErr
		}
	}
	return Result{Bytes: written, ZeroCopy: false}, nil
}

func validateRegion(region channel.FileRegion) error {
	if region == nil || region.Count() < 0 || region.Transferred() < 0 || region.Transferred() > region.Count() {
		return channel.ErrInvalidFileRegion
	}
	return nil
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if n > 0 {
			data = data[n:]
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

type nativeRegion interface {
	channel.FileRegion
	ReaderAt() io.ReaderAt
	Offset() int64
	Advance(int64) error
}

func nativeFile(region channel.FileRegion) (*os.File, int64, func(int64) error, bool) {
	native, ok := region.(nativeRegion)
	if !ok {
		return nil, 0, nil, false
	}
	file, ok := native.ReaderAt().(*os.File)
	if !ok {
		return nil, 0, nil, false
	}
	return file, native.Offset(), native.Advance, true
}
