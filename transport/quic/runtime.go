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
		ACK:        NewACKTracker(),
		Loss:       NewLossRecovery(cfg.Loss),
		Congestion: congestion,
		Streams:    NewStreamManager(cfg.InitialSendWindow, cfg.InitialRecvWindow),
		Paths:      NewPathManager(conn.Remote),
	}, nil
}

// Runtime 返回连接语义层；首次调用时使用默认参数初始化。
func (c *Connection) Runtime() (*Runtime, error) {
	if c == nil {
		return nil, ErrInvalidConfig
	}
	if c.runtime != nil {
		return c.runtime, nil
	}
	runtime, err := NewRuntime(c, RuntimeConfig{})
	if err != nil {
		return nil, err
	}
	c.runtime = runtime
	return runtime, nil
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
			r.Congestion.OnPacketAcked(packet.Bytes)
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
	}
	return nil
}
