package http3

import "sync/atomic"

// SessionStats 是 HTTP/3 transport binding 的 stream 和字节计数快照。
type SessionStats struct {
	OpenedStreams   uint64
	AcceptedStreams uint64
	ActiveStreams   uint64
	ClosedStreams   uint64
	BytesRead       uint64
	BytesWritten    uint64
	StreamsByKind   map[StreamKind]uint64
}

type sessionStats struct {
	opened     atomic.Uint64
	accepted   atomic.Uint64
	active     atomic.Uint64
	closed     atomic.Uint64
	readBytes  atomic.Uint64
	writeBytes atomic.Uint64
	byKind     [streamKindMetricSlots]atomic.Uint64
}

const streamKindMetricSlots = int(StreamKindRemoteQPACKDecoder) + 1

func newSessionStats() *sessionStats {
	return &sessionStats{}
}

func (s *sessionStats) recordStream(kind StreamKind, opened bool) {
	if s == nil {
		return
	}
	if opened {
		s.opened.Add(1)
	} else {
		s.accepted.Add(1)
	}
	s.active.Add(1)
	if idx := streamKindIndex(kind); idx >= 0 {
		s.byKind[idx].Add(1)
	}
}

func (s *sessionStats) recordClosed() {
	if s == nil {
		return
	}
	s.closed.Add(1)
	for {
		current := s.active.Load()
		if current == 0 || s.active.CompareAndSwap(current, current-1) {
			return
		}
	}
}

func (s *sessionStats) recordReadBytes(n int) {
	if s != nil && n > 0 {
		s.readBytes.Add(uint64(n))
	}
}

func (s *sessionStats) recordWriteBytes(n int) {
	if s != nil && n > 0 {
		s.writeBytes.Add(uint64(n))
	}
}

func (s *sessionStats) snapshot() SessionStats {
	if s == nil {
		return SessionStats{StreamsByKind: map[StreamKind]uint64{}}
	}
	return SessionStats{
		OpenedStreams:   s.opened.Load(),
		AcceptedStreams: s.accepted.Load(),
		ActiveStreams:   s.active.Load(),
		ClosedStreams:   s.closed.Load(),
		BytesRead:       s.readBytes.Load(),
		BytesWritten:    s.writeBytes.Load(),
		StreamsByKind:   s.kindSnapshot(),
	}
}

func (s *sessionStats) kindSnapshot() map[StreamKind]uint64 {
	out := make(map[StreamKind]uint64, streamKindMetricSlots-1)
	for i := 1; i < streamKindMetricSlots; i++ {
		value := s.byKind[i].Load()
		if value > 0 {
			out[StreamKind(i)] = value
		}
	}
	return out
}

func streamKindIndex(kind StreamKind) int {
	idx := int(kind)
	if idx <= 0 || idx >= streamKindMetricSlots {
		return -1
	}
	return idx
}
