//go:build darwin

package macos

import (
	"context"
	"os/exec"

	resolverdns "goark.dev/gnalloy/resolver/dns"
)

// DNSConfig 调用 macOS scutil 读取系统 DNS 快照。
func (p Provider) DNSConfig(ctx context.Context) (resolverdns.Config, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg := normalizeProviderConfig(p.cfg)
	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); !ok && cfg.CommandTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, cfg.CommandTimeout)
		defer cancel()
	}
	out, err := exec.CommandContext(ctx, cfg.Command, "--dns").Output()
	if err != nil {
		return resolverdns.Config{}, err
	}
	return ParseScutilDNSConfig(out, cfg)
}
