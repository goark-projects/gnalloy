// Package tls 提供基于 crypto/tls 的 Channel Handler。
//
// Handler 位于业务 Handler 与底层传输之间：入站密文 ByteBuf 会被解密成明文
// ByteBuf 继续向后传播，出站明文 ByteBuf 会被加密后写给底层 Channel。
package tls
