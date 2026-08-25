// Package http2 提供 HTTP/2 基础帧编解码和 Stream 状态契约。
//
// 本包先固定 RFC 9113 的二进制 frame 边界，不在核心层绑定 HPACK 或业务路由。
package http2
