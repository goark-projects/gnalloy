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
