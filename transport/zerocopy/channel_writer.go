package zerocopy

import (
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

// ChannelWriter 把平台零拷贝发送器适配为 channel.FileRegionWriter。
type ChannelWriter struct {
	sender Sender
}

func NewChannelWriter(chunkSize int) (ChannelWriter, error) {
	sender, err := NewSender(chunkSize)
	if err != nil {
		return ChannelWriter{}, err
	}
	return ChannelWriter{sender: sender}, nil
}

func (w ChannelWriter) WriteFileRegion(fd transport.FDRef, region channel.FileRegion) (int64, bool, error) {
	result, again, err := w.sender.SendFile(fd, region)
	return result.Bytes, again, err
}
