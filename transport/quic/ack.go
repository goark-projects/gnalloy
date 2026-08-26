package quic

import "sort"

// PacketNumberRange 描述闭区间 packet number 范围。
type PacketNumberRange struct {
	Smallest uint64
	Largest  uint64
}

// ACKTracker 维护每个 packet number space 的已接收 packet 范围。
type ACKTracker struct {
	spaces map[PacketNumberSpace][]PacketNumberRange
}

// NewACKTracker 创建 ACK 跟踪器。
func NewACKTracker() *ACKTracker {
	return &ACKTracker{spaces: make(map[PacketNumberSpace][]PacketNumberRange, 3)}
}

// Receive 记录 packet number；重复包返回 false。
func (t *ACKTracker) Receive(space PacketNumberSpace, packetNumber uint64) bool {
	if t == nil {
		return false
	}
	ranges := t.spaces[space]
	for i := range ranges {
		r := ranges[i]
		if packetNumber >= r.Smallest && packetNumber <= r.Largest {
			return false
		}
	}
	ranges = append(ranges, PacketNumberRange{Smallest: packetNumber, Largest: packetNumber})
	sort.Slice(ranges, func(i int, j int) bool {
		return ranges[i].Smallest < ranges[j].Smallest
	})
	t.spaces[space] = mergePacketNumberRanges(ranges)
	return true
}

// ACKFrame 构造当前 space 的 ACK frame。
func (t *ACKTracker) ACKFrame(space PacketNumberSpace, delay uint64) (ACKFrame, bool) {
	if t == nil {
		return ACKFrame{}, false
	}
	ranges := t.spaces[space]
	if len(ranges) == 0 {
		return ACKFrame{}, false
	}
	largest := ranges[len(ranges)-1]
	frame := ACKFrame{
		LargestAcked:  largest.Largest,
		Delay:         delay,
		FirstAckRange: largest.Largest - largest.Smallest,
	}
	previousSmallest := largest.Smallest
	for i := len(ranges) - 2; i >= 0 && len(frame.AdditionalRanges) < maxACKRanges; i-- {
		r := ranges[i]
		if previousSmallest <= r.Largest+1 {
			continue
		}
		gapPackets := previousSmallest - r.Largest - 1
		frame.AdditionalRanges = append(frame.AdditionalRanges, ACKRange{
			Gap:    gapPackets - 1,
			Length: r.Largest - r.Smallest,
		})
		previousSmallest = r.Smallest
	}
	return frame, true
}

// Ranges 返回当前 space 的升序 packet number 范围快照。
func (t *ACKTracker) Ranges(space PacketNumberSpace) []PacketNumberRange {
	if t == nil || len(t.spaces[space]) == 0 {
		return nil
	}
	out := make([]PacketNumberRange, len(t.spaces[space]))
	copy(out, t.spaces[space])
	return out
}

// ACKFrameRanges 将 ACK frame 展开为升序 packet number 范围。
func ACKFrameRanges(frame ACKFrame) ([]PacketNumberRange, error) {
	if frame.FirstAckRange > frame.LargestAcked {
		return nil, ErrInvalidFrame
	}
	ranges := make([]PacketNumberRange, 0, 1+len(frame.AdditionalRanges))
	smallest := frame.LargestAcked - frame.FirstAckRange
	ranges = append(ranges, PacketNumberRange{Smallest: smallest, Largest: frame.LargestAcked})
	previousSmallest := smallest
	for _, r := range frame.AdditionalRanges {
		gap := r.Gap + 1
		if previousSmallest <= gap+1 {
			return nil, ErrInvalidFrame
		}
		largest := previousSmallest - gap - 1
		if r.Length > largest {
			return nil, ErrInvalidFrame
		}
		smallest = largest - r.Length
		ranges = append(ranges, PacketNumberRange{Smallest: smallest, Largest: largest})
		previousSmallest = smallest
	}
	for i, j := 0, len(ranges)-1; i < j; i, j = i+1, j-1 {
		ranges[i], ranges[j] = ranges[j], ranges[i]
	}
	return ranges, nil
}

// ACKFrameContains 判断 ACK frame 是否覆盖指定 packet number。
func ACKFrameContains(frame ACKFrame, packetNumber uint64) bool {
	ranges, err := ACKFrameRanges(frame)
	if err != nil {
		return false
	}
	for _, r := range ranges {
		if packetNumber >= r.Smallest && packetNumber <= r.Largest {
			return true
		}
	}
	return false
}

func mergePacketNumberRanges(ranges []PacketNumberRange) []PacketNumberRange {
	if len(ranges) <= 1 {
		return ranges
	}
	out := ranges[:1]
	for _, r := range ranges[1:] {
		last := &out[len(out)-1]
		if r.Smallest <= last.Largest+1 {
			if r.Largest > last.Largest {
				last.Largest = r.Largest
			}
			continue
		}
		out = append(out, r)
	}
	return out
}
