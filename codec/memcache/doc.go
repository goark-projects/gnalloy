// Package memcache 提供 Memcached binary protocol 帧、对象和 client/server codec。
//
// FrameDecoder 解析 24 字节固定头并将 extras/key/value 作为零拷贝切片输出；
// FrameEncoder 构造固定头，body 三段直接下传给后续 OutboundSink。
package memcache
