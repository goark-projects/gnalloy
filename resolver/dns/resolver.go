package dns

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"time"

	dnscodec "goark.dev/gnalloy/codec/dns"
)

const defaultTimeout = 5 * time.Second

// Exchanger 执行一次 DNS 查询/响应交换。
type Exchanger interface {
	Exchange(ctx context.Context, server string, query dnscodec.Message) (dnscodec.Message, error)
}

type ExchangerFunc func(ctx context.Context, server string, query dnscodec.Message) (dnscodec.Message, error)

func (f ExchangerFunc) Exchange(ctx context.Context, server string, query dnscodec.Message) (dnscodec.Message, error) {
	return f(ctx, server, query)
}

type Config struct {
	Servers   []string
	Timeout   time.Duration
	Network   string
	Exchanger Exchanger
}

func DefaultConfig() Config {
	return Config{Timeout: defaultTimeout, Network: "udp"}
}

// Resolver 提供 Go 化 DNS 解析入口。
type Resolver struct {
	servers   []string
	timeout   time.Duration
	network   string
	exchanger Exchanger
	nextID    atomic.Uint32
}

func NewResolver(cfg Config) *Resolver {
	def := DefaultConfig()
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = def.Timeout
	}
	network := cfg.Network
	if network == "" {
		network = def.Network
	}
	servers := append([]string(nil), cfg.Servers...)
	exchanger := cfg.Exchanger
	if exchanger == nil && len(servers) > 0 {
		exchanger = UDPExchanger{Network: network, Timeout: timeout}
	}
	return &Resolver{
		servers:   servers,
		timeout:   timeout,
		network:   network,
		exchanger: exchanger,
	}
}

func (r *Resolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	ips, err := r.LookupIP(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.String())
	}
	return out, nil
}

func (r *Resolver) LookupIP(ctx context.Context, host string) ([]net.IP, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	if r == nil || r.exchanger == nil {
		return lookupSystem(ctx, host)
	}
	if len(r.servers) == 0 {
		return nil, ErrNoNameServer
	}
	ips := make([]net.IP, 0, 2)
	var firstErr error
	for _, qtype := range []uint16{dnscodec.TypeA, dnscodec.TypeAAAA} {
		found, err := r.lookupType(ctx, host, qtype)
		if err != nil {
			if firstErr == nil && !errors.Is(err, ErrNoAnswer) {
				firstErr = err
			}
			continue
		}
		ips = appendUniqueIP(ips, found...)
	}
	if len(ips) > 0 {
		return ips, nil
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, ErrNoAnswer
}

func (r *Resolver) lookupType(ctx context.Context, host string, qtype uint16) ([]net.IP, error) {
	query := dnscodec.NewQuery(uint16(r.nextID.Add(1)), host, qtype)
	var firstErr error
	for _, server := range r.servers {
		reply, err := r.exchanger.Exchange(ctx, server, query)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		ips, err := collectIPs(query, reply, qtype)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		return ips, nil
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, ErrNoAnswer
}

func collectIPs(query dnscodec.Message, reply dnscodec.Message, qtype uint16) ([]net.IP, error) {
	if !reply.Response || reply.ID != query.ID {
		return nil, ErrInvalidReply
	}
	if reply.ResponseCode != dnscodec.RCodeNoError {
		return nil, ErrServerFailure
	}
	ips := make([]net.IP, 0, len(reply.Answers))
	for _, answer := range reply.Answers {
		if answer.Class != dnscodec.ClassIN || answer.Type != qtype {
			continue
		}
		if ip := answer.IP(); ip != nil {
			ips = append(ips, ip)
		}
	}
	if len(ips) == 0 {
		return nil, ErrNoAnswer
	}
	return ips, nil
}

func lookupSystem(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		ips = append(ips, addr.IP)
	}
	if len(ips) == 0 {
		return nil, ErrNoAnswer
	}
	return ips, nil
}

func appendUniqueIP(dst []net.IP, src ...net.IP) []net.IP {
	for _, ip := range src {
		duplicate := false
		for _, existing := range dst {
			if existing.Equal(ip) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			dst = append(dst, ip)
		}
	}
	return dst
}
