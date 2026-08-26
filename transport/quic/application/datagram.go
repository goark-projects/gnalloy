package application

import (
	"context"
	"time"

	"goark.dev/gnalloy/transport/quic/rfc9000"
)

// DatagramMatcher 判断收到的 datagram 是否属于当前请求。
type DatagramMatcher func(request []byte, response []byte) bool

// DatagramExchanger 在 QUIC datagram 能力上执行一次 request-response 交换。
type DatagramExchanger struct {
	Dialer  rfc9000.Dialer
	Config  rfc9000.Config
	Timeout time.Duration
	Match   DatagramMatcher
}

// Exchange 发送一个 datagram，并持续接收直到匹配响应或上下文结束。
func (e DatagramExchanger) Exchange(ctx context.Context, address string, payload []byte) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if e.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.Timeout)
		defer cancel()
	}
	dialer := e.Dialer
	if dialer == nil {
		dialer = rfc9000.DefaultDialer{}
	}
	conn, err := dialer.DialAddr(ctx, address, e.Config)
	if err != nil {
		return nil, err
	}
	defer conn.CloseWithError(0, "datagram exchange complete")
	if err := conn.SendDatagram(payload); err != nil {
		return nil, err
	}
	match := e.Match
	if match == nil {
		match = func(_, _ []byte) bool { return true }
	}
	for {
		response, err := conn.ReceiveDatagram(ctx)
		if err != nil {
			return nil, err
		}
		if match(payload, response) {
			return response, nil
		}
	}
}
