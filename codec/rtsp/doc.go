// Package rtsp 提供 RTSP/1.0 请求与响应编解码。
//
// Decoder 支持 Content-Length 定界和零拷贝 body；Encoder 自动补默认
// RTSP/1.0 版本与 Content-Length。
package rtsp
