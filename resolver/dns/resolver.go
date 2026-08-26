package dns

import (
	"context"
	"errors"
	"net"
	"strings"
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
	// Servers 是递归 DNS 服务器地址列表。
	Servers []string
	// Timeout 是单次 DNS 交换超时。
	Timeout time.Duration
	// Network 控制默认 UDP exchanger 使用的网络类型。
	Network string
	// Exchanger 执行首选 DNS 交换，nil 时使用 UDPExchanger。
	Exchanger Exchanger
	// TCPFallback 表示 UDP 响应设置 TC 位后使用 TCP 重新查询。
	TCPFallback bool
	// TCPExchanger 执行截断响应的 TCP 兜底查询。
	TCPExchanger Exchanger
	// Cache 缓存解析结果；nil 表示关闭缓存。
	Cache Cache
	// MinTTL 是正向缓存最小 TTL。
	MinTTL time.Duration
	// MaxTTL 是正向缓存最大 TTL。
	MaxTTL time.Duration
	// NegativeTTL 是 ErrNoAnswer 等负向结果 TTL。
	NegativeTTL time.Duration
	// Hosts 提供 hosts 文件等本地静态解析结果。
	Hosts HostsResolver
	// SearchDomains 是相对域名查询的搜索域。
	SearchDomains []string
	// Ndots 控制先查原始名称还是先走搜索域，0 表示 1。
	Ndots int
	// MaxCNAMEHops 限制 CNAME 递归深度，0 表示 8。
	MaxCNAMEHops int
}

func DefaultConfig() Config {
	return Config{Timeout: defaultTimeout, Network: "udp"}
}

// Resolver 提供 Go 化 DNS 解析入口。
type Resolver struct {
	servers      []string
	timeout      time.Duration
	network      string
	exchanger    Exchanger
	tcpExchanger Exchanger
	cache        Cache
	minTTL       time.Duration
	maxTTL       time.Duration
	negativeTTL  time.Duration
	hosts        HostsResolver
	search       []string
	ndots        int
	maxCNAMEHops int
	nextID       atomic.Uint32
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
	var tcpExchanger Exchanger
	if cfg.TCPFallback {
		tcpExchanger = cfg.TCPExchanger
		if tcpExchanger == nil {
			tcpExchanger = TCPExchanger{Timeout: timeout}
		}
	}
	ndots := cfg.Ndots
	if ndots <= 0 {
		ndots = 1
	}
	maxCNAMEHops := cfg.MaxCNAMEHops
	if maxCNAMEHops <= 0 {
		maxCNAMEHops = 8
	}
	return &Resolver{
		servers:      servers,
		timeout:      timeout,
		network:      network,
		exchanger:    exchanger,
		tcpExchanger: tcpExchanger,
		cache:        cfg.Cache,
		minTTL:       cfg.MinTTL,
		maxTTL:       cfg.MaxTTL,
		negativeTTL:  cfg.NegativeTTL,
		hosts:        cfg.Hosts,
		search:       normalizeSearchDomains(cfg.SearchDomains),
		ndots:        ndots,
		maxCNAMEHops: maxCNAMEHops,
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
	if r == nil {
		return lookupSystem(ctx, host)
	}
	candidates := r.candidateNames(host)
	var firstErr error
	for _, candidate := range candidates {
		if r.hosts != nil {
			if ips, ok := r.hosts.LookupHost(candidate); ok {
				return ips, nil
			}
		}
		if r.cache != nil {
			if ips, err, ok := r.cache.Lookup(candidate, time.Now()); ok {
				return ips, err
			}
		}
		ips, err := r.lookupIPCandidate(ctx, candidate)
		if err == nil {
			return ips, nil
		}
		if firstErr == nil && !errors.Is(err, ErrNoAnswer) {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, ErrNoAnswer
}

func (r *Resolver) lookupIPCandidate(ctx context.Context, host string) ([]net.IP, error) {
	if r == nil || r.exchanger == nil {
		return lookupSystem(ctx, host)
	}
	if len(r.servers) == 0 {
		return nil, ErrNoNameServer
	}
	ips := make([]net.IP, 0, 2)
	var ttl time.Duration
	var firstErr error
	for _, qtype := range []uint16{dnscodec.TypeA, dnscodec.TypeAAAA} {
		found, foundTTL, err := r.lookupType(ctx, host, qtype)
		if err != nil {
			if firstErr == nil && !errors.Is(err, ErrNoAnswer) {
				firstErr = err
			}
			continue
		}
		ips = appendUniqueIP(ips, found...)
		ttl = minPositiveTTL(ttl, foundTTL)
	}
	if len(ips) > 0 {
		r.storeCache(host, ips, nil, r.positiveTTL(ttl))
		return ips, nil
	}
	if firstErr != nil {
		r.storeCache(host, nil, firstErr, r.negativeTTL)
		return nil, firstErr
	}
	r.storeCache(host, nil, ErrNoAnswer, r.negativeTTL)
	return nil, ErrNoAnswer
}

func (r *Resolver) lookupType(ctx context.Context, host string, qtype uint16) ([]net.IP, time.Duration, error) {
	return r.lookupTypeDepth(ctx, host, qtype, 0)
}

func (r *Resolver) lookupTypeDepth(ctx context.Context, host string, qtype uint16, depth int) ([]net.IP, time.Duration, error) {
	if depth > r.maxCNAMEHops {
		return nil, 0, ErrCNAMETooDeep
	}
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
		if reply.Truncated && r.tcpExchanger != nil {
			reply, err = r.tcpExchanger.Exchange(ctx, server, query)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
		}
		ips, ttl, cname, cnameTTL, err := collectIPs(query, reply, qtype)
		if err != nil {
			if errors.Is(err, ErrNoAnswer) && cname != "" {
				found, foundTTL, err := r.lookupTypeDepth(ctx, cname, qtype, depth+1)
				if err == nil {
					return found, minPositiveTTL(cnameTTL, foundTTL), nil
				}
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		return ips, ttl, nil
	}
	if firstErr != nil {
		return nil, 0, firstErr
	}
	return nil, 0, ErrNoAnswer
}

func collectIPs(query dnscodec.Message, reply dnscodec.Message, qtype uint16) ([]net.IP, time.Duration, string, time.Duration, error) {
	if !reply.Response || reply.ID != query.ID {
		return nil, 0, "", 0, ErrInvalidReply
	}
	if reply.ResponseCode != dnscodec.RCodeNoError {
		return nil, 0, "", 0, ErrServerFailure
	}
	ips := make([]net.IP, 0, len(reply.Answers))
	var ttl time.Duration
	cname := ""
	var cnameTTL time.Duration
	questionName := ""
	if len(query.Questions) > 0 {
		questionName = normalizeDNSName(query.Questions[0].Name)
	}
	for _, answer := range reply.Answers {
		if answer.Class != dnscodec.ClassIN {
			continue
		}
		answerName := normalizeDNSName(answer.Name)
		if answer.Type == dnscodec.TypeCNAME && (answerName == questionName || answerName == normalizeDNSName(cname)) {
			if target, ok := answer.Target(); ok {
				cname = normalizeDNSName(target)
				cnameTTL = minPositiveTTL(cnameTTL, time.Duration(answer.TTL)*time.Second)
			}
			continue
		}
	}
	targetName := questionName
	if cname != "" {
		targetName = cname
	}
	for _, answer := range reply.Answers {
		if answer.Class != dnscodec.ClassIN {
			continue
		}
		answerName := normalizeDNSName(answer.Name)
		if answer.Type != qtype {
			continue
		}
		if answerName != targetName {
			continue
		}
		if ip := answer.IP(); ip != nil {
			ips = append(ips, ip)
			ttl = minPositiveTTL(ttl, time.Duration(answer.TTL)*time.Second)
		}
	}
	if len(ips) == 0 {
		return nil, 0, cname, cnameTTL, ErrNoAnswer
	}
	return ips, ttl, cname, cnameTTL, nil
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

func (r *Resolver) storeCache(host string, ips []net.IP, err error, ttl time.Duration) {
	if r == nil || r.cache == nil || ttl <= 0 {
		return
	}
	r.cache.Store(host, ips, err, ttl, time.Now())
}

func (r *Resolver) positiveTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		ttl = r.minTTL
	}
	if r.minTTL > 0 && ttl < r.minTTL {
		ttl = r.minTTL
	}
	if r.maxTTL > 0 && ttl > r.maxTTL {
		ttl = r.maxTTL
	}
	return ttl
}

func (r *Resolver) candidateNames(host string) []string {
	name := normalizeDNSName(host)
	if name == "" {
		return nil
	}
	absolute := strings.HasSuffix(strings.TrimSpace(host), ".")
	if absolute || len(r.search) == 0 {
		return []string{name}
	}
	dots := strings.Count(name, ".")
	out := make([]string, 0, len(r.search)+1)
	add := func(candidate string) {
		candidate = normalizeDNSName(candidate)
		if candidate == "" {
			return
		}
		for _, existing := range out {
			if existing == candidate {
				return
			}
		}
		out = append(out, candidate)
	}
	if dots >= r.ndots {
		add(name)
	}
	for _, domain := range r.search {
		add(name + "." + domain)
	}
	add(name)
	return out
}

func minPositiveTTL(current time.Duration, next time.Duration) time.Duration {
	if next <= 0 {
		return current
	}
	if current <= 0 || next < current {
		return next
	}
	return current
}

func normalizeSearchDomains(domains []string) []string {
	out := make([]string, 0, len(domains))
	for _, domain := range domains {
		normalized := normalizeDNSName(domain)
		if normalized == "" {
			continue
		}
		duplicate := false
		for _, existing := range out {
			if existing == normalized {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, normalized)
		}
	}
	return out
}

func normalizeDNSName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	return strings.TrimSuffix(name, ".")
}
