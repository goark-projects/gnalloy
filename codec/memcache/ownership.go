package memcache

import "goark.dev/gnalloy/buffer"

func retainPart(part buffer.ByteBuf) buffer.ByteBuf {
	if part == nil {
		return nil
	}
	return part.Retain()
}

func releasePart(part buffer.ByteBuf) {
	if part != nil {
		part.Release()
	}
}
