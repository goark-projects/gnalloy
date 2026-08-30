package benchh3

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"strconv"
	"testing"
	"time"

	"goark.dev/gnalloy/benchmarks/external/internal/benchtls"
	"goark.dev/gnalloy/channel"
	codechttp3 "goark.dev/gnalloy/codec/http3"
	h3transport "goark.dev/gnalloy/transport/http3"
	"goark.dev/gnalloy/transport/quic/rfc9000"
)

func TestRunLoadHTTP3QUIC(t *testing.T) {
	cert, err := benchtls.SelfSignedCertificate(benchtls.DefaultServerName)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := rfc9000.ListenAddr("127.0.0.1:0", rfc9000.Config{
		TLS: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS13,
			NextProtos:   []string{alpnHTTP3},
		},
		NextProtos: []string{alpnHTTP3},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	clientDone := make(chan struct{})
	done := serveH3(t, ctx, listener, 16, 3, clientDone)

	result, err := RunLoad(context.Background(), Config{
		Addr:              listener.Addr().String(),
		ServerName:        benchtls.DefaultServerName,
		Payload:           16,
		Connections:       1,
		Messages:          2,
		WarmupMessages:    1,
		LatencySampleRate: 1,
		Timeout:           5 * time.Second,
		TLS: &tls.Config{
			ServerName:         benchtls.DefaultServerName,
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS13,
			NextProtos:         []string{alpnHTTP3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests != 2 || result.Errors != 0 || result.Latency.Samples != 2 || result.NegotiatedProtocol != alpnHTTP3 {
		t.Fatalf("result=%+v", result)
	}
	close(clientDone)
	<-done
}

func serveH3(t *testing.T, ctx context.Context, listener rfc9000.Listener, payload int, requests int, clientDone <-chan struct{}) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept(ctx)
		if err != nil {
			t.Errorf("accept h3: %v", err)
			return
		}
		defer closeH3TestConnection(ctx, conn, clientDone)
		session, err := h3transport.NewSession(conn, h3transport.Config{})
		if err != nil {
			t.Errorf("new h3 session: %v", err)
			return
		}
		body := ResponseBody(payload)
		for i := 0; i < requests; i++ {
			streamCh, err := session.AcceptRequestStream(ctx)
			if err != nil {
				t.Errorf("accept h3 stream: %v", err)
				return
			}
			if err := serveH3Stream(ctx, streamCh, body); err != nil {
				t.Errorf("serve h3 stream: %v", err)
				return
			}
		}
	}()
	return done
}

func closeH3TestConnection(ctx context.Context, conn rfc9000.Connection, clientDone <-chan struct{}) {
	select {
	case <-clientDone:
	case <-ctx.Done():
	}
	_ = conn.CloseWithError(0, "test done")
}

func serveH3Stream(ctx context.Context, streamCh *h3transport.StreamChannel, body []byte) error {
	handler := &h3TestResponseHandler{body: body}
	if err := streamCh.Channel().Pipeline().AddLast("handler", handler); err != nil {
		return err
	}
	for !handler.responded {
		_, err := streamCh.ReadOnce(ctx)
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) && handler.responded {
			break
		}
		return err
	}
	return streamCh.Close()
}

type h3TestResponseHandler struct {
	body      []byte
	responded bool
}

func (h *h3TestResponseHandler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if _, ok := msg.(codechttp3.HeadersBlock); !ok {
		ctx.FireChannelRead(msg)
		return
	}
	h.responded = true
	body, err := ctx.Channel().Allocator().Acquire(len(h.body))
	if err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	if _, err := body.WriteBytes(h.body); err != nil {
		body.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	if err := ctx.Write(codechttp3.HeadersBlock{Fields: []codechttp3.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-length", Value: strconv.Itoa(len(h.body))},
	}}); err != nil {
		body.Release()
		ctx.FireExceptionCaught(err)
		return
	}
	if err := ctx.Write(codechttp3.DataFrame{Data: body}); err != nil {
		ctx.FireExceptionCaught(err)
		return
	}
	if err := ctx.Flush(); err != nil {
		ctx.FireExceptionCaught(err)
	}
}
