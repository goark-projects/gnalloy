// Package socks 提供 SOCKS4/SOCKS5 握手与命令帧编解码。
//
// 该包面向 pipeline 使用；客户端代理流程可按阶段挂载 Greeting、Method、
// Command 和 Reply 对应的 decoder/encoder。
package socks
