package dns

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"time"

	dnscodec "goark.dev/gnalloy/codec/dns"
)

const maxTCPMessageSize = 1 << 16

// TCPExchanger 使用 RFC 1035 的两字节长度前缀执行 DNS over TCP 交换。
type TCPExchanger struct {
	// Network 控制 Dialer 使用的网络类型，空值表示 tcp。
	Network string
	// Timeout 是单次 TCP 交换超时。
	Timeout time.Duration
	// Dialer 控制 TCP 连接建立策略。
	Dialer net.Dialer
}

// Exchange 执行一次 DNS over TCP 查询。
func (e TCPExchanger) Exchange(ctx context.Context, server string, query dnscodec.Message) (dnscodec.Message, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := e.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); !ok && timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	network := e.Network
	if network == "" {
		network = "tcp"
	}
	conn, err := e.Dialer.DialContext(ctx, network, server)
	if err != nil {
		return dnscodec.Message{}, err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	payload, err := dnscodec.AppendMessage(nil, query)
	if err != nil {
		return dnscodec.Message{}, err
	}
	if len(payload) >= maxTCPMessageSize {
		return dnscodec.Message{}, ErrInvalidReply
	}
	frame := make([]byte, 2+len(payload))
	binary.BigEndian.PutUint16(frame[:2], uint16(len(payload)))
	copy(frame[2:], payload)
	if _, err := conn.Write(frame); err != nil {
		return dnscodec.Message{}, err
	}
	var header [2]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return dnscodec.Message{}, err
	}
	size := int(binary.BigEndian.Uint16(header[:]))
	if size <= 0 {
		return dnscodec.Message{}, ErrInvalidReply
	}
	reply := make([]byte, size)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return dnscodec.Message{}, err
	}
	return dnscodec.ParseMessage(reply)
}
