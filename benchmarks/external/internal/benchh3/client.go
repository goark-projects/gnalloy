package benchh3

import (
	"context"
	"errors"
	"io"
	"sync/atomic"

	codechttp3 "goark.dev/gnalloy/codec/http3"
	h3transport "goark.dev/gnalloy/transport/http3"
	"goark.dev/gnalloy/transport/quic/rfc9000"
)

type client struct {
	conn     rfc9000.Connection
	session  *h3transport.Session
	headers  codechttp3.HeadersBlock
	expected []byte
	reply    []byte
	alpn     string
}

func runClientMessages(ctx context.Context, c *client, messageCount int, latencySampleRate int, startCh <-chan struct{}, successes *atomic.Int64, latencySamples *[]int64) error {
	select {
	case <-startCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	latencyRecorder := newLatencyWindowRecorder(latencySampleRate, latencySamples)
	for i := 0; i < messageCount; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		latencyRecorder.begin()
		if err := runRequest(ctx, c); err != nil {
			return err
		}
		latencyRecorder.finish(i == messageCount-1)
		if successes != nil {
			successes.Add(1)
		}
	}
	return nil
}

func runRequest(ctx context.Context, c *client) error {
	streamCh, err := c.session.OpenRequestStream(ctx)
	if err != nil {
		return err
	}
	defer streamCh.Close()
	capture := &responseCapture{expected: c.expected, reply: c.reply[:0]}
	if err := streamCh.Channel().Pipeline().AddLast("capture", capture); err != nil {
		return err
	}
	if err := streamCh.Channel().WriteAndFlush(c.headers); err != nil {
		return err
	}
	if err := readResponse(ctx, streamCh, capture); err != nil {
		return err
	}
	return nil
}

func readResponse(ctx context.Context, streamCh *h3transport.StreamChannel, capture *responseCapture) error {
	for {
		if capture.complete() {
			return nil
		}
		_, err := streamCh.ReadOnce(ctx)
		if capture.err != nil {
			return capture.err
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) && capture.complete() {
			return nil
		}
		return err
	}
}
