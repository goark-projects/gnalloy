package dns

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	dnscodec "goark.dev/gnalloy/codec/dns"
)

func TestLookupIPUsesConfiguredExchanger(t *testing.T) {
	fake := &fakeExchanger{}
	resolver := NewResolver(Config{Servers: []string{"127.0.0.1:53"}, Exchanger: fake})

	ips, err := resolver.LookupIP(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 1 || !ips[0].Equal(net.IPv4(1, 2, 3, 4)) {
		t.Fatalf("ips=%v", ips)
	}
	if len(fake.queries) != 2 {
		t.Fatalf("queries=%d, want A and AAAA", len(fake.queries))
	}
	if fake.queries[0].Questions[0].Type != dnscodec.TypeA || fake.queries[1].Questions[0].Type != dnscodec.TypeAAAA {
		t.Fatalf("queries=%+v", fake.queries)
	}
}

func TestLookupIPReturnsLiteralWithoutExchange(t *testing.T) {
	fake := &fakeExchanger{}
	resolver := NewResolver(Config{Servers: []string{"127.0.0.1:53"}, Exchanger: fake})

	ips, err := resolver.LookupIP(context.Background(), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 1 || !ips[0].Equal(net.IPv4(127, 0, 0, 1)) {
		t.Fatalf("ips=%v", ips)
	}
	if len(fake.queries) != 0 {
		t.Fatalf("queries=%d, want 0", len(fake.queries))
	}
}

func TestLookupIPReturnsNoAnswer(t *testing.T) {
	resolver := NewResolver(Config{
		Servers: []string{"127.0.0.1:53"},
		Exchanger: ExchangerFunc(func(_ context.Context, _ string, query dnscodec.Message) (dnscodec.Message, error) {
			return dnscodec.Message{
				ID:                 query.ID,
				Response:           true,
				RecursionDesired:   query.RecursionDesired,
				RecursionAvailable: true,
				Questions:          query.Questions,
			}, nil
		}),
	})

	_, err := resolver.LookupIP(context.Background(), "missing.example")
	if !errors.Is(err, ErrNoAnswer) {
		t.Fatalf("err=%v, want %v", err, ErrNoAnswer)
	}
}

func TestLookupIPUsesPositiveCache(t *testing.T) {
	fake := &fakeExchanger{}
	resolver := NewResolver(Config{
		Servers:   []string{"127.0.0.1:53"},
		Cache:     NewMemoryCache(),
		Exchanger: fake,
	})

	ips, err := resolver.LookupIP(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	ips[0][0] = 9
	cached, err := resolver.LookupIP(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(cached) != 1 || !cached[0].Equal(net.IPv4(1, 2, 3, 4)) {
		t.Fatalf("cached ips=%v", cached)
	}
	if len(fake.queries) != 2 {
		t.Fatalf("queries=%d, want one A/AAAA lookup pair", len(fake.queries))
	}
}

func TestLookupIPUsesNegativeCache(t *testing.T) {
	var queries int
	resolver := NewResolver(Config{
		Servers:     []string{"127.0.0.1:53"},
		Cache:       NewMemoryCache(),
		NegativeTTL: time.Minute,
		Exchanger: ExchangerFunc(func(_ context.Context, _ string, query dnscodec.Message) (dnscodec.Message, error) {
			queries++
			return dnscodec.Message{
				ID:        query.ID,
				Response:  true,
				Questions: query.Questions,
			}, nil
		}),
	})

	for i := 0; i < 2; i++ {
		_, err := resolver.LookupIP(context.Background(), "missing.example")
		if !errors.Is(err, ErrNoAnswer) {
			t.Fatalf("err=%v, want %v", err, ErrNoAnswer)
		}
	}
	if queries != 2 {
		t.Fatalf("queries=%d, want one cached A/AAAA miss", queries)
	}
}

func TestLookupIPFallsBackToTCPOnTruncatedUDPReply(t *testing.T) {
	var udpQueries int
	var tcpQueries int
	resolver := NewResolver(Config{
		Servers:     []string{"127.0.0.1:53"},
		TCPFallback: true,
		Exchanger: ExchangerFunc(func(_ context.Context, _ string, query dnscodec.Message) (dnscodec.Message, error) {
			udpQueries++
			return dnscodec.Message{
				ID:        query.ID,
				Response:  true,
				Truncated: true,
				Questions: query.Questions,
			}, nil
		}),
		TCPExchanger: ExchangerFunc(func(_ context.Context, _ string, query dnscodec.Message) (dnscodec.Message, error) {
			tcpQueries++
			return replyWithA(query, net.IPv4(5, 6, 7, 8), 60), nil
		}),
	})

	ips, err := resolver.LookupIP(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 1 || !ips[0].Equal(net.IPv4(5, 6, 7, 8)) {
		t.Fatalf("ips=%v", ips)
	}
	if udpQueries != 2 || tcpQueries != 2 {
		t.Fatalf("udp=%d tcp=%d, want fallback for A and AAAA", udpQueries, tcpQueries)
	}
}

func TestLookupIPUsesStaticHostsBeforeNetwork(t *testing.T) {
	fake := &fakeExchanger{}
	resolver := NewResolver(Config{
		Servers:   []string{"127.0.0.1:53"},
		Exchanger: fake,
		Hosts: NewStaticHosts(map[string][]net.IP{
			"Example.COM.": {net.IPv4(10, 0, 0, 1)},
		}),
	})

	ips, err := resolver.LookupIP(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 1 || !ips[0].Equal(net.IPv4(10, 0, 0, 1)) {
		t.Fatalf("ips=%v", ips)
	}
	if len(fake.queries) != 0 {
		t.Fatalf("queries=%d, want hosts hit without network", len(fake.queries))
	}
	ips[0][0] = 99
	again, ok := resolver.hosts.LookupHost("example.com")
	if !ok || !again[0].Equal(net.IPv4(10, 0, 0, 1)) {
		t.Fatalf("hosts returned mutable storage: %v", again)
	}
}

func TestLookupIPAppliesSearchDomainsAndNdots(t *testing.T) {
	var names []string
	resolver := NewResolver(Config{
		Servers:       []string{"127.0.0.1:53"},
		SearchDomains: []string{"svc.local", "local"},
		Ndots:         2,
		Exchanger: ExchangerFunc(func(_ context.Context, _ string, query dnscodec.Message) (dnscodec.Message, error) {
			names = append(names, query.Questions[0].Name)
			if query.Questions[0].Name == "api.svc.local" {
				return replyWithA(query, net.IPv4(10, 0, 0, 2), 30), nil
			}
			return emptyReply(query), nil
		}),
	})

	ips, err := resolver.LookupIP(context.Background(), "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 1 || !ips[0].Equal(net.IPv4(10, 0, 0, 2)) {
		t.Fatalf("ips=%v", ips)
	}
	if len(names) == 0 || names[0] != "api.svc.local" {
		t.Fatalf("query order=%v, want search domain first", names)
	}
}

func TestLookupIPFollowsCNAMEChain(t *testing.T) {
	var names []string
	resolver := NewResolver(Config{
		Servers: []string{"127.0.0.1:53"},
		Exchanger: ExchangerFunc(func(_ context.Context, _ string, query dnscodec.Message) (dnscodec.Message, error) {
			names = append(names, query.Questions[0].Name)
			switch query.Questions[0].Name {
			case "alias.example":
				cname, err := dnscodec.NewNameResource("alias.example", dnscodec.TypeCNAME, 10, "target.example")
				if err != nil {
					return dnscodec.Message{}, err
				}
				reply := emptyReply(query)
				reply.Answers = []dnscodec.Resource{cname}
				return reply, nil
			case "target.example":
				return replyWithA(query, net.IPv4(10, 0, 0, 3), 20), nil
			default:
				return emptyReply(query), nil
			}
		}),
	})

	ips, err := resolver.LookupIP(context.Background(), "alias.example")
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 1 || !ips[0].Equal(net.IPv4(10, 0, 0, 3)) {
		t.Fatalf("ips=%v", ips)
	}
	if len(names) < 2 || names[0] != "alias.example" || names[1] != "target.example" {
		t.Fatalf("query names=%v, want alias then target", names)
	}
}

func TestMemoryCacheClearRemovesAllEntries(t *testing.T) {
	cache := NewMemoryCache()
	cache.Store("a.example", []net.IP{net.IPv4(1, 1, 1, 1)}, nil, time.Minute, time.Now())
	cache.Store("b.example", []net.IP{net.IPv4(2, 2, 2, 2)}, nil, time.Minute, time.Now())

	cache.Clear()

	if _, _, ok := cache.Lookup("a.example", time.Now()); ok {
		t.Fatal("a.example should be removed")
	}
	if _, _, ok := cache.Lookup("b.example", time.Now()); ok {
		t.Fatal("b.example should be removed")
	}
}

type fakeExchanger struct {
	queries []dnscodec.Message
}

func (e *fakeExchanger) Exchange(_ context.Context, _ string, query dnscodec.Message) (dnscodec.Message, error) {
	e.queries = append(e.queries, query)
	reply := dnscodec.Message{
		ID:                 query.ID,
		Response:           true,
		RecursionDesired:   query.RecursionDesired,
		RecursionAvailable: true,
		Questions:          query.Questions,
	}
	if len(query.Questions) == 0 || query.Questions[0].Type != dnscodec.TypeA {
		return reply, nil
	}
	return replyWithA(query, net.IPv4(1, 2, 3, 4), 60), nil
}

func replyWithA(query dnscodec.Message, ip net.IP, ttl uint32) dnscodec.Message {
	reply := dnscodec.Message{
		ID:                 query.ID,
		Response:           true,
		RecursionDesired:   query.RecursionDesired,
		RecursionAvailable: true,
		Questions:          query.Questions,
	}
	if len(query.Questions) == 0 || query.Questions[0].Type != dnscodec.TypeA {
		return reply
	}
	reply.Answers = []dnscodec.Resource{{
		Name:  query.Questions[0].Name,
		Type:  dnscodec.TypeA,
		Class: dnscodec.ClassIN,
		TTL:   ttl,
		Data:  []byte(ip.To4()),
	}}
	return reply
}

func emptyReply(query dnscodec.Message) dnscodec.Message {
	return dnscodec.Message{
		ID:                 query.ID,
		Response:           true,
		RecursionDesired:   query.RecursionDesired,
		RecursionAvailable: true,
		Questions:          query.Questions,
	}
}
