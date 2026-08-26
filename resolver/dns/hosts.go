package dns

import (
	"net"
	"strings"
)

// HostsResolver 提供本地 hosts 文件或静态主机表解析能力。
type HostsResolver interface {
	LookupHost(host string) ([]net.IP, bool)
}

// StaticHosts 是进程内静态 hosts 表。
type StaticHosts struct {
	entries map[string][]net.IP
}

// NewStaticHosts 创建静态 hosts 表，输入切片会被复制。
func NewStaticHosts(entries map[string][]net.IP) *StaticHosts {
	hosts := &StaticHosts{entries: make(map[string][]net.IP, len(entries))}
	for name, ips := range entries {
		normalized := normalizeDNSName(name)
		if normalized == "" {
			continue
		}
		hosts.entries[normalized] = cloneIPs(ips)
	}
	return hosts
}

// LookupHost 查询静态 hosts 表。
func (h *StaticHosts) LookupHost(host string) ([]net.IP, bool) {
	if h == nil {
		return nil, false
	}
	ips := h.entries[normalizeDNSName(host)]
	if len(ips) == 0 {
		return nil, false
	}
	return cloneIPs(ips), true
}

// ParseHosts 解析 hosts 文件内容。
func ParseHosts(content string) *StaticHosts {
	entries := make(map[string][]net.IP)
	for _, line := range strings.Split(content, "\n") {
		if idx := strings.IndexByte(line, '#'); idx >= 0 {
			line = line[:idx]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ip := net.ParseIP(fields[0])
		if ip == nil {
			continue
		}
		for _, name := range fields[1:] {
			normalized := normalizeDNSName(name)
			if normalized == "" {
				continue
			}
			entries[normalized] = append(entries[normalized], append(net.IP(nil), ip...))
		}
	}
	return NewStaticHosts(entries)
}
