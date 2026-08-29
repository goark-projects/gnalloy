package tcp

import (
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/zerocopy"
)

func newFileRegionWriter() channel.FileRegionWriter {
	writer, err := zerocopy.NewChannelWriter(0)
	if err != nil {
		return nil
	}
	return writer
}
