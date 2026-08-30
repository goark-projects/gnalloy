// Package tls 提供基于 crypto/tls 的 Channel Handler。
//
// Handler 位于业务 Handler 与底层传输之间：入站密文 ByteBuf 会被解密成明文
// ByteBuf 继续向后传播，出站明文 ByteBuf 会被加密后写给底层 Channel。
//
// 包内同时提供密码套件目录、IANA/Java/OpenSSL 名称解析和 TLS 1.0-1.2
// 配置应用工具。TLS 1.3 密码套件由 Go 运行时管理，目录可查询但不能写入
// crypto/tls.Config.CipherSuites。
package tls
