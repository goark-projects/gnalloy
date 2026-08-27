package protocol

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	transportcore "goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/l2"
	"goark.dev/gnalloy/transport/raw"
	"goark.dev/gnalloy/transport/udp"
)

func TestChannelExchangerAdapters(t *testing.T) {
	tests := []struct {
		name    string
		adapter Adapter
		reply   func(channel.Channel, []byte) (any, error)
	}{
		{
			name:    "stream",
			adapter: StreamAdapter{},
			reply: func(ch channel.Channel, _ []byte) (any, error) {
				return testBuffer(ch, "pong")
			},
		},
		{
			name:    "datagram",
			adapter: DatagramAdapter{},
			reply: func(ch channel.Channel, _ []byte) (any, error) {
				payload, err := testBuffer(ch, "pong")
				if err != nil {
					return nil, err
				}
				return udp.Datagram{
					Payload: payload,
					Addr:    udp.Address{IP: net.IPv4(127, 0, 0, 1), Port: 5353},
				}, nil
			},
		},
		{
			name:    "packet",
			adapter: PacketAdapter{},
			reply: func(ch channel.Channel, _ []byte) (any, error) {
				payload, err := testBuffer(ch, "pong")
				if err != nil {
					return nil, err
				}
				return raw.Packet{
					Payload:  payload,
					Addr:     raw.Address{IP: net.IPv4(127, 0, 0, 1)},
					Protocol: 253,
				}, nil
			},
		},
		{
			name:    "frame",
			adapter: FrameAdapter{},
			reply: func(ch channel.Channel, _ []byte) (any, error) {
				payload, err := testBuffer(ch, "pong")
				if err != nil {
					return nil, err
				}
				return l2.Frame{
					Meta:    l2.FrameMeta{InterfaceName: "eth-test", EtherType: 0x88b5},
					Payload: payload,
				}, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := newProtocolTestGroup(t)
			transport := fakeClientTransport{
				write: func(ch channel.Channel, msg any) error {
					request := payloadFromMessage(t, msg)
					if request != "ping" {
						t.Fatalf("request=%q, want ping", request)
					}
					response, err := tt.reply(ch, []byte(request))
					if err != nil {
						return err
					}
					ch.Pipeline().FireChannelRead(response)
					ch.Pipeline().FireChannelReadComplete()
					return nil
				},
			}

			got, err := (ChannelExchanger{
				Group:     group,
				Transport: transport,
				Adapter:   tt.adapter,
				Timeout:   time.Second,
			}).Exchange(context.Background(), "example", []byte("ping"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "pong" {
				t.Fatalf("response=%q, want pong", string(got))
			}
		})
	}
}

func TestChannelExchangerRejectsInvalidConfig(t *testing.T) {
	_, err := ChannelExchanger{}.Exchange(context.Background(), "example", []byte("ping"))
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidConfig)
	}
}

type fakeClientTransport struct {
	write func(channel.Channel, any) error
}

func (t fakeClientTransport) Dial(_ context.Context, cfg bootstrap.ClientConfig) (channel.Channel, error) {
	sink := &fakeSink{write: t.write}
	ch := channel.NewLocalChannel(1, buffer.NewHeapAllocator(), sink)
	sink.ch = ch
	if cfg.Initializer != nil {
		if err := cfg.Initializer(ch); err != nil {
			return nil, err
		}
	}
	return ch, nil
}

type fakeSink struct {
	write func(channel.Channel, any) error
	ch    channel.Channel
}

func (s fakeSink) Write(msg any) error {
	if s.write == nil {
		return nil
	}
	return s.write(s.ch, msg)
}

func (s fakeSink) Flush() error {
	return nil
}

func (s fakeSink) Close() error {
	return nil
}

func testBuffer(ch channel.Channel, value string) (buffer.ByteBuf, error) {
	buf, err := ch.Allocator().Acquire(len(value))
	if err != nil {
		return nil, err
	}
	if _, err := buf.WriteBytes([]byte(value)); err != nil {
		buf.Release()
		return nil, err
	}
	return buf, nil
}

func payloadFromMessage(t *testing.T, msg any) string {
	t.Helper()
	buf, ok := msg.(buffer.ByteBuf)
	if !ok {
		t.Fatalf("request type=%T, want ByteBuf", msg)
	}
	defer buf.Release()
	return string(buf.Bytes())
}

func newProtocolTestGroup(t *testing.T) *transportcore.EventLoopGroup {
	t.Helper()
	group, err := transportcore.NewEventLoopGroup(transportcore.EventLoopGroupConfig{
		Size:         1,
		PollerConfig: transportcore.Config{Backend: transportcore.BackendMemory},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = group.Shutdown(ctx)
	})
	return group
}
