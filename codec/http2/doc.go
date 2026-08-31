// Package http2 提供 HTTP/2 基础帧编解码、连接生命周期和 Stream 状态契约。
//
// 本包固定 RFC 9113 的二进制 frame 边界，HPACK、h2c 和 HTTP/1 对象桥接保持显式组合。
package http2
