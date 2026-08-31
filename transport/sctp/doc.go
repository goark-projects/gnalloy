// Package sctp 提供 SCTP one-to-one stream transport 的 Go-native 边界。
//
// SCTP 内核支持由操作系统决定；当前原生 socket 入口仅在 Linux 编译，
// 其他平台返回明确 unsupported 错误。
package sctp
