package dns

import (
	"context"
	"errors"
	"net"
	"testing"

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
	reply.Answers = []dnscodec.Resource{{
		Name:  query.Questions[0].Name,
		Type:  dnscodec.TypeA,
		Class: dnscodec.ClassIN,
		TTL:   60,
		Data:  []byte{1, 2, 3, 4},
	}}
	return reply, nil
}
