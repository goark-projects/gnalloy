// Package ipfilter 提供按远端 IP 过滤入站消息的 Pipeline Handler。
//
// 规则按配置顺序匹配，首个命中规则决定放行或拒绝；无法提取远端地址的消息
// 由 DefaultAccept 决定处理方式。该包不依赖具体 TCP Channel 实现，可直接处理
// UDP/raw typed message，也支持业务消息实现 RemoteIPProvider。
package ipfilter
