package ipfilter

import "net"

// Action 描述 IP 过滤规则命中后的处理动作。
type Action uint8

const (
	Accept Action = iota + 1
	Reject
)

// Rule 是有序 IP 过滤规则。
type Rule interface {
	Match(ip net.IP) (Action, bool)
}

type ipRule struct {
	ip     net.IP
	action Action
}

type cidrRule struct {
	network *net.IPNet
	action  Action
}

// AllowIP 创建单 IP 放行规则。
func AllowIP(ip net.IP) Rule {
	return ipRule{ip: cloneIP(ip), action: Accept}
}

// DenyIP 创建单 IP 拒绝规则。
func DenyIP(ip net.IP) Rule {
	return ipRule{ip: cloneIP(ip), action: Reject}
}

// AllowCIDR 创建 CIDR 放行规则。
func AllowCIDR(cidr string) (Rule, error) {
	return parseCIDRRule(cidr, Accept)
}

// DenyCIDR 创建 CIDR 拒绝规则。
func DenyCIDR(cidr string) (Rule, error) {
	return parseCIDRRule(cidr, Reject)
}

func (r ipRule) Match(ip net.IP) (Action, bool) {
	if r.ip == nil || ip == nil || !r.ip.Equal(ip) {
		return 0, false
	}
	return r.action, true
}

func (r cidrRule) Match(ip net.IP) (Action, bool) {
	if r.network == nil || ip == nil || !r.network.Contains(ip) {
		return 0, false
	}
	return r.action, true
}

func parseCIDRRule(cidr string, action Action) (Rule, error) {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	return cidrRule{network: network, action: action}, nil
}

func cloneIP(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	return append(net.IP(nil), ip...)
}
