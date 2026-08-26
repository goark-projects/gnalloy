package dns

import (
	"context"
	"net"
	"sync"
	"testing"
)

func TestResolverGroupReusesResolverPerKey(t *testing.T) {
	group := NewResolverGroup[string](Config{})

	first := group.Resolver("loop-a")
	second := group.Resolver("loop-a")
	third := group.Resolver("loop-b")

	if first != second {
		t.Fatal("same key should reuse resolver")
	}
	if first == third {
		t.Fatal("different keys should use isolated resolvers")
	}
	if group.Size() != 2 {
		t.Fatalf("size=%d, want 2", group.Size())
	}
	if !group.Delete("loop-a") || group.Size() != 1 {
		t.Fatalf("delete failed, size=%d", group.Size())
	}
	group.Clear()
	if group.Size() != 0 {
		t.Fatalf("size=%d, want 0", group.Size())
	}
}

func TestResolverGroupConcurrentResolverCreation(t *testing.T) {
	group := NewResolverGroup[int](Config{})
	const workers = 32

	var wg sync.WaitGroup
	results := make(chan *Resolver, workers)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			results <- group.Resolver(7)
		}()
	}
	wg.Wait()
	close(results)

	var first *Resolver
	for resolver := range results {
		if first == nil {
			first = resolver
			continue
		}
		if first != resolver {
			t.Fatal("concurrent calls should return one resolver instance")
		}
	}
}

func TestRoundRobinResolverRotatesIPOrder(t *testing.T) {
	hosts := NewStaticHosts(map[string][]net.IP{
		"svc.local": {
			net.ParseIP("10.0.0.1"),
			net.ParseIP("10.0.0.2"),
			net.ParseIP("10.0.0.3"),
		},
	})
	resolver := NewRoundRobinResolver(NewResolver(Config{Hosts: hosts}))

	first, err := resolver.LookupHost(context.Background(), "svc.local")
	if err != nil {
		t.Fatal(err)
	}
	second, err := resolver.LookupHost(context.Background(), "svc.local")
	if err != nil {
		t.Fatal(err)
	}
	third, err := resolver.LookupHost(context.Background(), "svc.local")
	if err != nil {
		t.Fatal(err)
	}

	if first[0] != "10.0.0.1" || second[0] != "10.0.0.2" || third[0] != "10.0.0.3" {
		t.Fatalf("round-robin heads=%s,%s,%s", first[0], second[0], third[0])
	}
}

func TestRoundRobinResolverGroupIsolatesKeys(t *testing.T) {
	hosts := NewStaticHosts(map[string][]net.IP{
		"svc.local": {
			net.ParseIP("10.0.0.1"),
			net.ParseIP("10.0.0.2"),
		},
	})
	group := NewRoundRobinResolverGroup[string](Config{Hosts: hosts})

	a := group.Resolver("loop-a")
	b := group.Resolver("loop-b")
	aFirst, err := a.LookupHost(context.Background(), "svc.local")
	if err != nil {
		t.Fatal(err)
	}
	bFirst, err := b.LookupHost(context.Background(), "svc.local")
	if err != nil {
		t.Fatal(err)
	}

	if aFirst[0] != "10.0.0.1" || bFirst[0] != "10.0.0.1" {
		t.Fatalf("resolver keys should keep independent rotation: a=%v b=%v", aFirst, bFirst)
	}
}
