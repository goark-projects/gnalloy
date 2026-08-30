package tls

import (
	"goark.dev/gnalloy/buffer"
)

func ownedBufferFromChunk(chunk *byteChunk) buffer.ByteBuf {
	if chunk == nil || len(chunk.data) == 0 {
		return nil
	}
	data := chunk.data
	owner := chunk.owner
	release := chunk.release
	*chunk = byteChunk{}
	return buffer.NewOwnedBuffer(data, func([]byte) {
		if release != nil && owner != nil {
			release(owner)
		}
	})
}
