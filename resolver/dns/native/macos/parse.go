package macos

import (
	"bufio"
	"bytes"
	"net"
	"strings"

	resolverdns "goark.dev/gnalloy/resolver/dns"
)

const maxScutilLineSize = 64 * 1024

// ParseScutilDNSConfig 把 scutil --dns 输出解析为 gnalloy DNS 配置。
func ParseScutilDNSConfig(data []byte, cfg ResolverConfig) (resolverdns.Config, error) {
	var servers []string
	var search []string

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024), maxScutilLineSize)
	for scanner.Scan() {
		key, value, ok := splitScutilLine(scanner.Text())
		if !ok {
			continue
		}
		switch {
		case strings.HasPrefix(key, "nameserver["):
			if server, ok := normalizeServer(value); ok {
				servers = appendUniqueString(servers, server)
			}
		case strings.HasPrefix(key, "search domain[") || key == "domain":
			if domain, ok := normalizeDomain(value); ok {
				search = appendUniqueString(search, domain)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return resolverdns.Config{}, err
	}
	if len(servers) == 0 {
		return resolverdns.Config{}, ErrInvalidConfig
	}
	return applyProviderConfig(resolverdns.Config{
		Servers:       servers,
		SearchDomains: search,
	}, cfg), nil
}

func splitScutilLine(line string) (string, string, bool) {
	key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
	if !ok {
		return "", "", false
	}
	key = strings.ToLower(strings.TrimSpace(key))
	value = strings.TrimSpace(value)
	return key, value, key != "" && value != ""
}

func normalizeServer(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if host, port, err := net.SplitHostPort(raw); err == nil && host != "" && port != "" {
		return net.JoinHostPort(strings.Trim(host, "[]"), port), true
	}
	if ip := net.ParseIP(raw); ip != nil {
		return net.JoinHostPort(ip.String(), "53"), true
	}
	return net.JoinHostPort(raw, "53"), true
}

func normalizeDomain(raw string) (string, bool) {
	raw = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if raw == "" || raw == "." {
		return "", false
	}
	return raw, true
}

func appendUniqueString(dst []string, value string) []string {
	for _, existing := range dst {
		if existing == value {
			return dst
		}
	}
	return append(dst, value)
}
