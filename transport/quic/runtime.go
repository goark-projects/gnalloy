package quic

import "goark.dev/gnalloy/transport/udp"

// RuntimeConfig 描述 QUIC 连接语义层的初始参数。
type RuntimeConfig struct {
	Loss              LossRecoveryConfig
	Congestion        CongestionConfig
	InitialSendWindow uint64
	InitialRecvWindow uint64
}

// Runtime 聚合 QUIC 连接所需的 ACK、丢包、拥塞、stream 和 path 状态。
type Runtime struct {
	conn       *Connection
	ACK        *ACKTracker
	Loss       *LossRecovery
	Congestion *CongestionController
	Streams    *StreamManager
	Paths      *PathManager
}

// NewRuntime 创建连接语义层。
func NewRuntime(conn *Connection, cfg RuntimeConfig) (*Runtime, error) {
	if conn == nil {
		return nil, ErrInvalidConfig
	}
	congestion, err := NewCongestionController(cfg.Congestion)
	if err != nil {
		return nil, err
	}
	return &Runtime{
		conn:       conn,
		ACK:        NewACKTracker(),
		Loss:       NewLossRecovery(cfg.Loss),
		Congestion: congestion,
		Streams:    NewStreamManager(cfg.InitialSendWindow, cfg.InitialRecvWindow),
		Paths:      NewPathManager(conn.Remote),
	}, nil
}

// Runtime 返回连接语义层；首次调用时使用默认参数初始化。
func (c *Connection) Runtime() (*Runtime, error) {
	return c.RuntimeWithConfig(RuntimeConfig{})
}

// RuntimeWithConfig 返回连接语义层；首次调用时使用给定参数初始化。
func (c *Connection) RuntimeWithConfig(cfg RuntimeConfig) (*Runtime, error) {
	if c == nil {
		return nil, ErrInvalidConfig
	}
	if c.runtime != nil {
		return c.runtime, nil
	}
	runtime, err := NewRuntime(c, cfg)
	if err != nil {
		return nil, err
	}
	c.runtime = runtime
	return runtime, nil
}

// ObservePacket 记录收到的 ack-eliciting packet number；重复包返回 false。
func (r *Runtime) ObservePacket(space PacketNumberSpace, packetNumber uint64) bool {
	if r == nil || r.ACK == nil {
		return false
	}
	return r.ACK.Receive(space, packetNumber)
}

// ACKFrame 构造指定 packet number space 的 ACK frame。
func (r *Runtime) ACKFrame(space PacketNumberSpace, delay uint64) (ACKFrame, bool) {
	if r == nil || r.ACK == nil {
		return ACKFrame{}, false
	}
	return r.ACK.ACKFrame(space, delay)
}

// RecordSentPacket 同步更新拥塞窗口和丢包恢复状态。
func (r *Runtime) RecordSentPacket(packet SentPacket) error {
	if r == nil || r.Congestion == nil || r.Loss == nil {
		return ErrInvalidConfig
	}
	if packet.AckEliciting {
		if err := r.Congestion.OnPacketSent(packet.Bytes); err != nil {
			return err
		}
	}
	r.Loss.OnPacketSent(packet)
	return nil
}

// ApplyFrame 将 frame 应用到连接语义层。
func (r *Runtime) ApplyFrame(space PacketNumberSpace, frame any) error {
	if r == nil {
		return ErrInvalidConfig
	}
	remote := udp.Address{}
	if r.Paths != nil {
		remote = r.Paths.Active()
	}
	return r.ApplyFrameFrom(remote, space, frame)
}

// ApplyFrameFrom 将来自指定路径的 frame 应用到连接语义层。
func (r *Runtime) ApplyFrameFrom(remote udp.Address, space PacketNumberSpace, frame any) error {
	if r == nil {
		return ErrInvalidConfig
	}
	switch f := frame.(type) {
	case ACKFrame:
		acked, lost, err := r.Loss.OnACK(space, f)
		if err != nil {
			return err
		}
		for _, packet := range acked {
			if packet.AckEliciting {
				r.Congestion.OnPacketAcked(packet.Bytes)
			}
		}
		for _, packet := range lost {
			r.Congestion.OnPacketLost(packet.Bytes)
		}
	case StreamFrame:
		_, err := r.Streams.Receive(f)
		return err
	case PathResponseFrame:
		if !r.Paths.Validate(remote, f) {
			return ErrInvalidPacket
		}
	case ConnectionCloseFrame:
		if r.conn != nil && r.conn.State != ConnectionStateClosed {
			r.conn.State = ConnectionStateClosing
		}
	}
	return nil
}
