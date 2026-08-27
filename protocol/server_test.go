package protocol

import (
	"context"
	"errors"
	"net"
	"testing"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/channel/embedded"
	transportcore "goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/l2"
	"goark.dev/gnalloy/transport/raw"
	"goark.dev/gnalloy/transport/udp"
)

func TestServerAdaptersRespondWithTransportMetadata(t *testing.T) {
	tests := []struct {
		name     string
		adapter  ServerAdapter
		inbound  func(channel.Channel) (any, error)
		validate func(*testing.T, any)
	}{
		{
			name:    "stream",
			adapter: StreamAdapter{},
			inbound: func(ch channel.Channel) (any, error) {
				return testBuffer(ch, "ping")
			},
			validate: func(t *testing.T, msg any) {
				if got := payloadFromMessage(t, msg); got != "pong" {
					t.Fatalf("payload=%q, want pong", got)
				}
			},
		},
		{
			name:    "datagram",
			adapter: DatagramAdapter{},
			inbound: func(ch channel.Channel) (any, error) {
				payload, err := testBuffer(ch, "ping")
				if err != nil {
					return nil, err
				}
				return udp.Datagram{
					Payload: payload,
					Addr:    udp.Address{IP: net.IPv4(127, 0, 0, 1), Port: 5353},
				}, nil
			},
			validate: func(t *testing.T, msg any) {
				datagram, ok := msg.(udp.Datagram)
				if !ok {
					t.Fatalf("response type=%T, want udp.Datagram", msg)
				}
				defer datagram.Release()
				if got := string(datagram.Payload.Bytes()); got != "pong" {
					t.Fatalf("payload=%q, want pong", got)
				}
				if datagram.Addr.String() != "127.0.0.1:5353" {
					t.Fatalf("addr=%q, want 127.0.0.1:5353", datagram.Addr.String())
				}
			},
		},
		{
			name:    "packet",
			adapter: PacketAdapter{},
			inbound: func(ch channel.Channel) (any, error) {
				payload, err := testBuffer(ch, "ping")
				if err != nil {
					return nil, err
				}
				return raw.Packet{
					Payload:  payload,
					Addr:     raw.Address{IP: net.IPv4(127, 0, 0, 1)},
					Protocol: 253,
				}, nil
			},
			validate: func(t *testing.T, msg any) {
				packet, ok := msg.(raw.Packet)
				if !ok {
					t.Fatalf("response type=%T, want raw.Packet", msg)
				}
				defer packet.Release()
				if got := string(packet.Payload.Bytes()); got != "pong" {
					t.Fatalf("payload=%q, want pong", got)
				}
				if packet.Addr.String() != "127.0.0.1" || packet.Protocol != 253 {
					t.Fatalf("packet target=%s/%d, want 127.0.0.1/253", packet.Addr.String(), packet.Protocol)
				}
			},
		},
		{
			name:    "frame",
			adapter: FrameAdapter{},
			inbound: func(ch channel.Channel) (any, error) {
				payload, err := testBuffer(ch, "ping")
				if err != nil {
					return nil, err
				}
				return l2.Frame{
					Meta:    l2.FrameMeta{InterfaceName: "eth-test", EtherType: 0x88b5},
					Payload: payload,
				}, nil
			},
			validate: func(t *testing.T, msg any) {
				frame, ok := msg.(l2.Frame)
				if !ok {
					t.Fatalf("response type=%T, want l2.Frame", msg)
				}
				defer frame.Release()
				if got := string(frame.Payload.Bytes()); got != "pong" {
					t.Fatalf("payload=%q, want pong", got)
				}
				if frame.Meta.InterfaceName != "eth-test" || frame.Meta.EtherType != 0x88b5 {
					t.Fatalf("frame meta=%+v, want eth-test/0x88b5", frame.Meta)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ec, err := embedded.New(NewServerHandler(tt.adapter, HandlerFunc(func(req Request, responder Responder) error {
				if string(req.Payload) != "ping" {
					t.Fatalf("payload=%q, want ping", string(req.Payload))
				}
				if req.Channel == nil || req.Message == nil {
					t.Fatalf("request channel/message must be retained")
				}
				return responder.Respond([]byte("pong"))
			})))
			if err != nil {
				t.Fatal(err)
			}
			defer ec.FinishAndReleaseAll()

			inbound, err := tt.inbound(ec.Channel())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ec.WriteInbound(inbound); err != nil {
				t.Fatal(err)
			}
			outbound, ok := ec.ReadOutbound()
			if !ok {
				t.Fatal("missing outbound response")
			}
			tt.validate(t, outbound)
		})
	}
}

func TestServerBindInstallsProtocolHandler(t *testing.T) {
	boss := newProtocolTestGroup(t)
	worker := newProtocolTestGroup(t)
	var initialized bool
	var response []byte

	server := Server{
		BossGroup:   boss,
		WorkerGroup: worker,
		Transport: fakeServerTransport{bind: func(_ context.Context, cfg bootstrap.ServerConfig) (bootstrap.Server, error) {
			sink := &captureSink{}
			ch := channel.NewLocalChannel(transportcore.ChannelID(9), buffer.NewHeapAllocator(), sink)
			if err := cfg.ChildInitializer(ch); err != nil {
				return nil, err
			}
			inbound, err := testBuffer(ch, "ping")
			if err != nil {
				return nil, err
			}
			ch.Pipeline().FireChannelRead(inbound)
			if sink.msg == nil {
				return nil, errors.New("missing response")
			}
			response = []byte(payloadFromMessage(t, sink.msg))
			return fakeBoundServer{addr: "protocol-test"}, nil
		}},
		Adapter: StreamAdapter{},
		Initializer: func(channel.Channel) error {
			initialized = true
			return nil
		},
		Handler: HandlerFunc(func(req Request, responder Responder) error {
			return responder.Respond(append(req.Payload, "-pong"...))
		}),
	}

	bound, err := server.BindContext(context.Background(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if bound.Addr() != "protocol-test" {
		t.Fatalf("addr=%q, want protocol-test", bound.Addr())
	}
	if !initialized {
		t.Fatal("initializer was not called")
	}
	if string(response) != "ping-pong" {
		t.Fatalf("response=%q, want ping-pong", string(response))
	}
}

func TestServerRejectsInvalidConfig(t *testing.T) {
	_, err := Server{}.BindContext(context.Background(), "127.0.0.1:0")
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidConfig)
	}
}

type fakeServerTransport struct {
	bind func(context.Context, bootstrap.ServerConfig) (bootstrap.Server, error)
}

func (t fakeServerTransport) Bind(ctx context.Context, cfg bootstrap.ServerConfig) (bootstrap.Server, error) {
	return t.bind(ctx, cfg)
}

type fakeBoundServer struct {
	addr string
}

func (s fakeBoundServer) Addr() string {
	return s.addr
}

func (s fakeBoundServer) Close() error {
	return nil
}

type captureSink struct {
	msg any
}

func (s *captureSink) Write(msg any) error {
	s.msg = msg
	return nil
}

func (s *captureSink) Flush() error {
	return nil
}

func (s *captureSink) Close() error {
	return nil
}
