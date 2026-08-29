//go:build !darwin

package macos

import (
	"context"
	"errors"
	"testing"
)

func TestProviderReturnsUnsupportedOffDarwin(t *testing.T) {
	_, err := NewProvider(ResolverConfig{}).DNSConfig(context.Background())
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("err=%v, want ErrUnsupportedPlatform", err)
	}
}
