package traffic

import "testing"

func TestRateLimiterReservesSmoothDelay(t *testing.T) {
	limiter := newRateLimiter(1000, 0)
	if got := limiter.Reserve(0, 500); got != 0 {
		t.Fatalf("first delay=%d", got)
	}
	if got := limiter.Reserve(0, 500); got != 500 {
		t.Fatalf("second delay=%d", got)
	}
	if got := limiter.Reserve(250, 500); got != 750 {
		t.Fatalf("third delay=%d", got)
	}
}

func TestRateLimiterCapsMaxDelay(t *testing.T) {
	limiter := newRateLimiter(1000, 100)
	_ = limiter.Reserve(0, 1000)
	if got := limiter.Reserve(0, 1000); got != 100 {
		t.Fatalf("delay=%d, want capped 100", got)
	}
}
