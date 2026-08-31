package scheduler

import "goark.dev/gnalloy/codec/http2"

const (
	defaultWeight  = 16
	minWeight      = 1
	maxWeight      = 256
	defaultQuantum = 16 * 1024
)

// Config 描述 HTTP/2 权重公平字节分配器参数。
type Config struct {
	// Quantum 是权重 1 每轮可获得的基础字节额度，0 使用 16KiB。
	Quantum int
}

// StreamState 是参与出站 DATA 分配的 stream 快照。
type StreamState struct {
	ID           http2.StreamID
	Weight       uint16
	PendingBytes int
}

// WriteFunc 写出指定 stream 在本轮最多可发送的字节数。
type WriteFunc func(id http2.StreamID, maxBytes int) (int, error)

// WeightedFairQueueByteDistributor 提供 HTTP/2 DATA 的权重公平分配。
type WeightedFairQueueByteDistributor struct {
	quantum int
	streams map[http2.StreamID]*streamState
	order   []http2.StreamID
	cursor  int
}

type streamState struct {
	id      http2.StreamID
	weight  int
	pending int
	deficit int
}

// NewWeightedFairQueueByteDistributor 创建权重公平字节分配器。
func NewWeightedFairQueueByteDistributor(cfg Config) *WeightedFairQueueByteDistributor {
	quantum := cfg.Quantum
	if quantum <= 0 {
		quantum = defaultQuantum
	}
	return &WeightedFairQueueByteDistributor{quantum: quantum, streams: make(map[http2.StreamID]*streamState, 8)}
}

// UpdateStream 更新一个 stream 的权重和待发送字节数。
func (d *WeightedFairQueueByteDistributor) UpdateStream(state StreamState) error {
	if d == nil || !state.ID.Valid() || state.PendingBytes < 0 {
		return http2.ErrInvalidStreamID
	}
	stream := d.streams[state.ID]
	if stream == nil {
		stream = &streamState{id: state.ID}
		d.streams[state.ID] = stream
		d.order = append(d.order, state.ID)
	}
	stream.weight = normalizeWeight(state.Weight)
	stream.pending = state.PendingBytes
	if stream.pending == 0 {
		stream.deficit = 0
	}
	return nil
}

// RemoveStream 删除一个 stream 的调度状态。
func (d *WeightedFairQueueByteDistributor) RemoveStream(id http2.StreamID) {
	if d == nil {
		return
	}
	delete(d.streams, id)
	for i, candidate := range d.order {
		if candidate != id {
			continue
		}
		copy(d.order[i:], d.order[i+1:])
		d.order[len(d.order)-1] = 0
		d.order = d.order[:len(d.order)-1]
		if d.cursor > i && d.cursor > 0 {
			d.cursor--
		}
		if d.cursor >= len(d.order) {
			d.cursor = 0
		}
		return
	}
}

// PendingBytes 返回指定 stream 当前待发送字节数。
func (d *WeightedFairQueueByteDistributor) PendingBytes(id http2.StreamID) int {
	if d == nil {
		return 0
	}
	stream := d.streams[id]
	if stream == nil {
		return 0
	}
	return stream.pending
}

// ActiveStreams 返回仍有待发送字节的 stream 数。
func (d *WeightedFairQueueByteDistributor) ActiveStreams() int {
	if d == nil {
		return 0
	}
	count := 0
	for _, stream := range d.streams {
		if stream.pending > 0 {
			count++
		}
	}
	return count
}

// Distribute 按权重公平策略分配最多 maxBytes 字节。
func (d *WeightedFairQueueByteDistributor) Distribute(maxBytes int, write WriteFunc) (int, error) {
	if d == nil || maxBytes <= 0 || write == nil {
		return 0, nil
	}
	total := 0
	for total < maxBytes && d.ActiveStreams() > 0 {
		progressed := false
		rounds := len(d.order)
		for i := 0; i < rounds && total < maxBytes; i++ {
			stream := d.nextStream()
			if stream == nil || stream.pending == 0 {
				continue
			}
			stream.deficit += stream.weight * d.quantum
			allowed := minInt(stream.deficit, stream.pending, maxBytes-total)
			if allowed <= 0 {
				continue
			}
			written, err := write(stream.id, allowed)
			if err != nil {
				return total, err
			}
			if written < 0 || written > allowed {
				return total, ErrInvalidWrite
			}
			if written == 0 {
				continue
			}
			stream.pending -= written
			stream.deficit -= written
			total += written
			progressed = true
			if stream.pending == 0 {
				stream.deficit = 0
			}
		}
		if !progressed {
			return total, nil
		}
	}
	return total, nil
}

func (d *WeightedFairQueueByteDistributor) nextStream() *streamState {
	if len(d.order) == 0 {
		return nil
	}
	if d.cursor >= len(d.order) {
		d.cursor = 0
	}
	id := d.order[d.cursor]
	d.cursor++
	return d.streams[id]
}

func normalizeWeight(weight uint16) int {
	switch {
	case weight == 0:
		return defaultWeight
	case weight < minWeight:
		return minWeight
	case weight > maxWeight:
		return maxWeight
	default:
		return int(weight)
	}
}

func minInt(first int, values ...int) int {
	out := first
	for _, value := range values {
		if value < out {
			out = value
		}
	}
	return out
}
