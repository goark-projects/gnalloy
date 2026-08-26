package ipfilter

import (
	"net"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/raw"
	"goark.dev/gnalloy/transport/udp"
)

// RemoteIPProvider 允许自定义消息暴露远端 IP。
type RemoteIPProvider interface {
	RemoteIP() net.IP
}

// Config 描述 IP 过滤器行为。
type Config struct {
	// Rules 按顺序匹配，首个命中规则决定动作。
	Rules []Rule
	// DefaultAccept 表示没有规则命中时是否放行。
	DefaultAccept bool
	// CloseOnReject 表示拒绝消息后关闭 Channel。
	CloseOnReject bool
}

// Handler 对齐 Netty ipfilter，按远端 IP 拒绝入站消息。
type Handler struct {
	rules         []Rule
	defaultAccept bool
	closeOnReject bool
}

// NewHandler 创建 IP 过滤器。
func NewHandler(cfg Config) *Handler {
	rules := make([]Rule, 0, len(cfg.Rules))
	for _, rule := range cfg.Rules {
		if rule != nil {
			rules = append(rules, rule)
		}
	}
	return &Handler{rules: rules, defaultAccept: cfg.DefaultAccept, closeOnReject: cfg.CloseOnReject}
}

// ChannelRead 检查消息远端 IP；无法提取 IP 的消息按默认策略处理。
func (h *Handler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	ip, ok := remoteIP(msg)
	if !ok || h.accept(ip) {
		ctx.FireChannelRead(msg)
		return
	}
	release(msg)
	if h.closeOnReject {
		_ = ctx.Close()
	}
}

func (h *Handler) accept(ip net.IP) bool {
	for _, rule := range h.rules {
		action, ok := rule.Match(ip)
		if !ok {
			continue
		}
		return action == Accept
	}
	return h.defaultAccept
}

func remoteIP(msg any) (net.IP, bool) {
	switch v := msg.(type) {
	case udp.Datagram:
		return v.Addr.IP, v.Addr.IP != nil
	case udp.Addressed:
		return v.Addr.IP, v.Addr.IP != nil
	case raw.Packet:
		return v.Addr.IP, v.Addr.IP != nil
	case raw.Addressed:
		return v.Addr.IP, v.Addr.IP != nil
	case RemoteIPProvider:
		ip := v.RemoteIP()
		return ip, ip != nil
	default:
		return nil, false
	}
}

func release(msg any) {
	if releasable, ok := msg.(interface{ Release() }); ok {
		releasable.Release()
	}
}
