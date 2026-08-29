//go:build !darwin

package macos

import (
	"context"

	resolverdns "goark.dev/gnalloy/resolver/dns"
)

// DNSConfig 在非 macOS 平台显式返回 unsupported。
func (p Provider) DNSConfig(context.Context) (resolverdns.Config, error) {
	return resolverdns.Config{}, ErrUnsupportedPlatform
}
