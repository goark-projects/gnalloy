package proxy

import "goark.dev/gnalloy/channel"

func writeProxyPayload(ctx *channel.HandlerContext, payload []byte) error {
	out, err := ctx.Channel().Allocator().Acquire(len(payload))
	if err != nil {
		return err
	}
	if _, err := out.WriteBytes(payload); err != nil {
		out.Release()
		return err
	}
	if err := ctx.Channel().WriteAndFlush(out); err != nil {
		out.Release()
		return err
	}
	return nil
}
