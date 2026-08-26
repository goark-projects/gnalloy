package quic

const defaultPacketThreshold uint64 = 3

// SentPacket 描述已发出但尚未确认的 QUIC packet。
type SentPacket struct {
	Space        PacketNumberSpace
	Number       uint64
	Bytes        int
	AckEliciting bool
	SentAtMillis int64
}

// LossRecoveryConfig 描述丢包判定策略。
type LossRecoveryConfig struct {
	// PacketThreshold 是 RFC 9002 推荐的 packet-threshold 丢包阈值，0 表示 3。
	PacketThreshold uint64
}

// LossRecovery 维护 packet number space 维度的 in-flight packet。
type LossRecovery struct {
	threshold uint64
	spaces    map[PacketNumberSpace]map[uint64]SentPacket
}

// NewLossRecovery 创建丢包恢复状态机。
func NewLossRecovery(cfg LossRecoveryConfig) *LossRecovery {
	threshold := cfg.PacketThreshold
	if threshold == 0 {
		threshold = defaultPacketThreshold
	}
	return &LossRecovery{
		threshold: threshold,
		spaces:    make(map[PacketNumberSpace]map[uint64]SentPacket, 3),
	}
}

// OnPacketSent 记录一个已发出 packet。
func (r *LossRecovery) OnPacketSent(packet SentPacket) {
	if r == nil || packet.Bytes < 0 {
		return
	}
	space := r.spaces[packet.Space]
	if space == nil {
		space = make(map[uint64]SentPacket, 64)
		r.spaces[packet.Space] = space
	}
	space[packet.Number] = packet
}

// OnACK 根据 ACK frame 返回确认包和 packet-threshold 判定的丢包。
func (r *LossRecovery) OnACK(space PacketNumberSpace, frame ACKFrame) ([]SentPacket, []SentPacket, error) {
	if r == nil {
		return nil, nil, nil
	}
	ranges, err := ACKFrameRanges(frame)
	if err != nil {
		return nil, nil, err
	}
	sent := r.spaces[space]
	if len(sent) == 0 {
		return nil, nil, nil
	}
	acked := make([]SentPacket, 0, len(ranges))
	for _, ackRange := range ranges {
		for pn, packet := range sent {
			if pn >= ackRange.Smallest && pn <= ackRange.Largest {
				acked = append(acked, packet)
				delete(sent, pn)
			}
		}
	}
	lost := make([]SentPacket, 0)
	for pn, packet := range sent {
		if !packet.AckEliciting {
			continue
		}
		if frame.LargestAcked > pn && frame.LargestAcked-pn >= r.threshold {
			lost = append(lost, packet)
			delete(sent, pn)
		}
	}
	return acked, lost, nil
}

// InFlight 返回当前 space 中仍未完成确认或丢包判定的 packet 数。
func (r *LossRecovery) InFlight(space PacketNumberSpace) int {
	if r == nil {
		return 0
	}
	return len(r.spaces[space])
}
