// Package stomp 提供 STOMP 1.2 文本帧编解码。
//
// Decoder 支持 LF/CRLF 行尾、心跳帧、content-length 定界和零拷贝 body。
// Encoder 只构造协议头尾，业务 body 直接交给下游写出。
package stomp
