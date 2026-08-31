package quic

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"time"
)

// StreamID 是 QUIC stream 的稳定公开标识。
type StreamID int64

// ApplicationErrorCode 是 QUIC 应用层连接关闭错误码。
type ApplicationErrorCode uint64

// StreamErrorCode 是 QUIC stream reset 或 stop-sending 错误码。
type StreamErrorCode uint64

// FeatureSupport 描述本端和对端是否都支持某项 QUIC 扩展能力。
type FeatureSupport struct {
	// Local 表示本端配置已启用该扩展能力。
	Local bool
	// Remote 表示对端传输参数声明支持该扩展能力。
	Remote bool
}

// State 是不暴露底层实现类型的 QUIC 连接状态快照。
type State struct {
	// TLS 是握手完成后的 TLS 1.3 状态，包含证书链和 ALPN。
	TLS tls.ConnectionState
	// Version 是已协商的 QUIC 版本。
	Version Version
	// SupportsDatagrams 表示 RFC 9221 datagram 扩展的协商结果。
	SupportsDatagrams FeatureSupport
	// SupportsStreamResetPartialDelivery 表示部分交付 reset 扩展的协商结果。
	SupportsStreamResetPartialDelivery FeatureSupport
	// Used0RTT 表示连接是否使用了 0-RTT。
	Used0RTT bool
	// GSO 表示底层发送路径是否使用 generic segmentation offload。
	GSO bool
}

// Listener 是 RFC9000 QUIC 服务端监听器接口。
type Listener interface {
	// Addr 返回监听器绑定的本地地址。
	Addr() net.Addr
	// Accept 接受一个已完成握手的 QUIC 连接。
	Accept(ctx context.Context) (Connection, error)
	// Close 关闭监听器；已经接受的连接由调用方自行关闭。
	Close() error
}

// EarlyListener 是可在握手完成前接受 0-RTT 连接的监听器接口。
type EarlyListener interface {
	// Addr 返回监听器绑定的本地地址。
	Addr() net.Addr
	// Accept 接受 QUIC 连接；返回时握手可能尚未完成，调用方可通过 HandshakeComplete 等待。
	Accept(ctx context.Context) (Connection, error)
	// Close 关闭监听器；已经接受的连接由调用方自行关闭。
	Close() error
}

// Connection 是 RFC9000 QUIC 连接接口。
type Connection interface {
	// LocalAddr 返回连接本地 UDP 地址。
	LocalAddr() net.Addr
	// RemoteAddr 返回连接对端 UDP 地址。
	RemoteAddr() net.Addr
	// HandshakeComplete 返回握手完成信号；常规 Dial/Accept 返回时通常已经关闭。
	HandshakeComplete() <-chan struct{}
	// ConnectionState 返回握手、版本和扩展能力状态快照。
	ConnectionState() State
	// Stats 返回连接级 RTT、包和字节计数快照。
	Stats() ConnectionStats
	// OpenStreamSync 打开双向 stream；无可用额度时阻塞到 ctx 结束或额度恢复。
	OpenStreamSync(ctx context.Context) (Stream, error)
	// AcceptStream 接受对端打开的双向 stream。
	AcceptStream(ctx context.Context) (Stream, error)
	// OpenUniStreamSync 打开本端发送的单向 stream，适合 HTTP/3 control/QPACK encoder。
	OpenUniStreamSync(ctx context.Context) (SendStream, error)
	// AcceptUniStream 接受对端发送的单向 stream，适合 HTTP/3 control/QPACK decoder。
	AcceptUniStream(ctx context.Context) (ReceiveStream, error)
	// SendDatagram 发送 RFC 9221 QUIC datagram。
	SendDatagram(payload []byte) error
	// ReceiveDatagram 接收 RFC 9221 QUIC datagram。
	ReceiveDatagram(ctx context.Context) ([]byte, error)
	// CloseWithError 使用应用层错误码关闭连接。
	CloseWithError(code ApplicationErrorCode, reason string) error
}

// Stream 是 QUIC 双向 stream；Close 只关闭发送方向并发送 FIN。
type Stream interface {
	io.Reader
	io.Writer
	io.Closer
	// ID 返回 stream 标识。
	ID() StreamID
	// SetDeadline 设置读写截止时间。
	SetDeadline(t time.Time) error
	// SetReadDeadline 设置读截止时间。
	SetReadDeadline(t time.Time) error
	// SetWriteDeadline 设置写截止时间。
	SetWriteDeadline(t time.Time) error
	// CancelRead 停止接收方向并向对端发送 STOP_SENDING。
	CancelRead(code StreamErrorCode)
	// CancelWrite 重置发送方向并向对端发送 RESET_STREAM。
	CancelWrite(code StreamErrorCode)
}

// SendStream 是 QUIC 单向发送 stream；Close 发送 FIN。
type SendStream interface {
	io.Writer
	io.Closer
	// ID 返回 stream 标识。
	ID() StreamID
	// SetWriteDeadline 设置写截止时间。
	SetWriteDeadline(t time.Time) error
	// CancelWrite 重置发送方向并向对端发送 RESET_STREAM。
	CancelWrite(code StreamErrorCode)
}

// ReceiveStream 是 QUIC 单向接收 stream。
type ReceiveStream interface {
	io.Reader
	// ID 返回 stream 标识。
	ID() StreamID
	// SetReadDeadline 设置读截止时间。
	SetReadDeadline(t time.Time) error
	// CancelRead 停止接收方向并向对端发送 STOP_SENDING。
	CancelRead(code StreamErrorCode)
}

// Dialer 抽象 RFC9000 QUIC 客户端拨号能力，便于连接池和测试替换。
type Dialer interface {
	// DialAddr 连接远端 QUIC 服务端。
	DialAddr(ctx context.Context, addr string, cfg Config) (Connection, error)
}

// EarlyDialer 抽象 RFC9000 QUIC 0-RTT 客户端拨号能力。
type EarlyDialer interface {
	// DialAddrEarly 使用 0-RTT 路径连接远端 QUIC 服务端。
	DialAddrEarly(ctx context.Context, addr string, cfg Config) (Connection, error)
}

// DialerFunc 允许普通函数作为 Dialer 使用。
type DialerFunc func(ctx context.Context, addr string, cfg Config) (Connection, error)

// DialAddr 实现 Dialer。
func (f DialerFunc) DialAddr(ctx context.Context, addr string, cfg Config) (Connection, error) {
	return f(ctx, addr, cfg)
}

// DefaultDialer 是使用系统 UDP socket 的默认 QUIC 拨号器。
type DefaultDialer struct{}

// DialAddr 连接远端 QUIC 服务端。
func (DefaultDialer) DialAddr(ctx context.Context, addr string, cfg Config) (Connection, error) {
	return DialAddr(ctx, addr, cfg)
}

// DialAddrEarly 使用 0-RTT 路径连接远端 QUIC 服务端。
func (DefaultDialer) DialAddrEarly(ctx context.Context, addr string, cfg Config) (Connection, error) {
	return DialAddrEarly(ctx, addr, cfg)
}
