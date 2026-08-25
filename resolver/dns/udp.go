package dns

import (
	"context"
	"net"
	"time"

	dnscodec "goark.dev/gnalloy/codec/dns"
)

// UDPExchanger 使用标准库 UDP 连接执行 DNS 报文交换。
type UDPExchanger struct {
	Network string
	Timeout time.Duration
	Dialer  net.Dialer
}

func (e UDPExchanger) Exchange(ctx context.Context, server string, query dnscodec.Message) (dnscodec.Message, error) {
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
		network = "udp"
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
	if _, err := conn.Write(payload); err != nil {
		return dnscodec.Message{}, err
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return dnscodec.Message{}, err
	}
	return dnscodec.ParseMessage(buf[:n])
}
