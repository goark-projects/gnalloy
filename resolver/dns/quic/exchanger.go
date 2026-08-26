package quic

import (
	"context"
	"net"
	"strings"
	"time"

	dnscodec "goark.dev/gnalloy/codec/dns"
	resolverdns "goark.dev/gnalloy/resolver/dns"
	"goark.dev/gnalloy/transport/quic/application"
	"goark.dev/gnalloy/transport/quic/rfc9000"
)

const (
	// DefaultALPN 是 RFC 9250 定义的 DNS-over-QUIC ALPN。
	DefaultALPN = "doq"
	// DefaultPort 是 DNS-over-QUIC 的默认端口。
	DefaultPort = "853"
)

// Exchanger 使用 DNS-over-QUIC 执行单次 DNS 查询。
type Exchanger struct {
	Dialer         rfc9000.Dialer
	Config         rfc9000.Config
	Timeout        time.Duration
	MaxMessageSize int
}

// Exchange 执行一次 DNS-over-QUIC 查询/响应交换。
func (e Exchanger) Exchange(ctx context.Context, server string, query dnscodec.Message) (dnscodec.Message, error) {
	payload, err := dnscodec.AppendMessage(nil, query)
	if err != nil {
		return dnscodec.Message{}, err
	}
	response, err := application.StreamExchanger{
		Dialer:  e.Dialer,
		Config:  e.config(),
		Timeout: e.Timeout,
		Codec: application.LengthPrefixedCodec{
			MaxFrameSize: e.maxMessageSize(),
		},
	}.Exchange(ctx, serverAddress(server), payload)
	if err != nil {
		return dnscodec.Message{}, err
	}
	return dnscodec.ParseMessage(response)
}

func (e Exchanger) config() rfc9000.Config {
	cfg := e.Config
	if len(cfg.NextProtos) == 0 {
		cfg.NextProtos = []string{DefaultALPN}
	}
	return cfg
}

func (e Exchanger) maxMessageSize() int {
	if e.MaxMessageSize <= 0 {
		return application.DefaultMaxFrameSize
	}
	return e.MaxMessageSize
}

func serverAddress(server string) string {
	server = strings.TrimSpace(server)
	if server == "" {
		return server
	}
	if _, _, err := net.SplitHostPort(server); err == nil {
		return server
	}
	if strings.HasPrefix(server, "[") && strings.HasSuffix(server, "]") {
		server = strings.TrimPrefix(strings.TrimSuffix(server, "]"), "[")
	}
	return net.JoinHostPort(server, DefaultPort)
}

var _ resolverdns.Exchanger = Exchanger{}
