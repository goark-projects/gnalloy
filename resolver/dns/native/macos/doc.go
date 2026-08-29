// Package macos 提供 macOS 系统 DNS 配置到 gnalloy resolver 的适配。
//
// 该包对齐 Netty resolver-dns-native-macos 的核心边界：从 macOS 系统 resolver
// 快照读取 nameserver 与搜索域，再交给 Go-native resolver 执行查询、缓存和回退。
package macos
