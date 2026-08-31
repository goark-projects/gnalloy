package http3

import "sync"

type qpackDynamicEntry struct {
	field HeaderField
	size  uint64
	index uint64
}

// QPACKDynamicTable 维护 QPACK 动态表的容量、插入计数和驱逐顺序。
type QPACKDynamicTable struct {
	capacity    uint64
	size        uint64
	insertCount uint64
	entries     []qpackDynamicEntry
}

// NewQPACKDynamicTable 创建动态表，容量为字节数。
func NewQPACKDynamicTable(capacity uint64) *QPACKDynamicTable {
	return &QPACKDynamicTable{capacity: capacity}
}

// Capacity 返回当前动态表容量。
func (t *QPACKDynamicTable) Capacity() uint64 {
	if t == nil {
		return 0
	}
	return t.capacity
}

// Size 返回当前动态表占用字节数。
func (t *QPACKDynamicTable) Size() uint64 {
	if t == nil {
		return 0
	}
	return t.size
}

// InsertCount 返回 RFC 语义上的已插入条目数。
func (t *QPACKDynamicTable) InsertCount() uint64 {
	if t == nil {
		return 0
	}
	return t.insertCount
}

// SetCapacity 更新动态表容量，并立即按新容量驱逐旧条目。
func (t *QPACKDynamicTable) SetCapacity(capacity uint64) error {
	if t == nil || capacity > maxVarInt {
		return ErrInvalidVarInt
	}
	t.capacity = capacity
	t.evict()
	return nil
}

// Insert 写入一个新字段，返回该条目的 absolute index。
func (t *QPACKDynamicTable) Insert(field HeaderField) (uint64, error) {
	if t == nil {
		return 0, ErrQPACKInvalidInstruction
	}
	size := qpackEntrySize(field)
	if size > t.capacity {
		return 0, ErrQPACKEntryTooLarge
	}
	index := t.insertCount
	t.insertCount++
	t.entries = append([]qpackDynamicEntry{{field: field, size: size, index: index}}, t.entries...)
	t.size += size
	t.evict()
	return index, nil
}

// Duplicate 复制 relative index 指向的条目，并把副本插入表头。
func (t *QPACKDynamicTable) Duplicate(relativeIndex uint64) (uint64, error) {
	field, ok := t.GetRelative(relativeIndex)
	if !ok {
		return 0, ErrQPACKInvalidIndex
	}
	return t.Insert(field)
}

// GetRelative 按 QPACK relative index 获取字段，0 表示最新条目。
func (t *QPACKDynamicTable) GetRelative(index uint64) (HeaderField, bool) {
	if t == nil || index >= uint64(len(t.entries)) {
		return HeaderField{}, false
	}
	return t.entries[index].field, true
}

// GetAbsolute 按 absolute index 获取字段。
func (t *QPACKDynamicTable) GetAbsolute(index uint64) (HeaderField, bool) {
	if t == nil {
		return HeaderField{}, false
	}
	for i := range t.entries {
		entry := t.entries[i]
		if entry.index == index {
			return entry.field, true
		}
	}
	return HeaderField{}, false
}

func (t *QPACKDynamicTable) evict() {
	for t.size > t.capacity && len(t.entries) > 0 {
		last := len(t.entries) - 1
		t.size -= t.entries[last].size
		t.entries[last] = qpackDynamicEntry{}
		t.entries = t.entries[:last]
	}
}

func qpackEntrySize(field HeaderField) uint64 {
	return uint64(len(field.Name) + len(field.Value) + 32)
}

// QPACKDynamicStateConfig 描述 QPACK 动态表和阻塞流限制。
type QPACKDynamicStateConfig struct {
	MaxTableCapacity  uint64
	MaxBlockedStreams uint64
}

// QPACKDynamicState 串联 encoder/decoder stream 指令和动态表状态。
type QPACKDynamicState struct {
	mu                  sync.Mutex
	table               QPACKDynamicTable
	maxTableCapacity    uint64
	maxBlockedStreams   uint64
	blockedSections     map[uint64]uint64
	trackedSections     map[uint64][]uint64
	knownReceivedCount  uint64
	acknowledgedStreams map[uint64]struct{}
}

// NewQPACKDynamicState 创建 QPACK 动态状态机。
func NewQPACKDynamicState(cfg QPACKDynamicStateConfig) *QPACKDynamicState {
	return &QPACKDynamicState{
		table:             *NewQPACKDynamicTable(cfg.MaxTableCapacity),
		maxTableCapacity:  cfg.MaxTableCapacity,
		maxBlockedStreams: cfg.MaxBlockedStreams,
	}
}

// DynamicTable 返回动态表快照。
func (s *QPACKDynamicState) DynamicTable() QPACKDynamicTable {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.table
}

// KnownReceivedCount 返回 decoder stream 已确认的插入计数。
func (s *QPACKDynamicState) KnownReceivedCount() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.knownReceivedCount
}

// BlockedStreams 返回当前被动态表依赖阻塞的 stream 数量。
func (s *QPACKDynamicState) BlockedStreams() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return uint64(len(s.blockedSections))
}

// StartFieldSection 注册一个将要解码的 header section，并返回它是否需要等待动态表。
func (s *QPACKDynamicState) StartFieldSection(streamID uint64, requiredInsertCount uint64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if requiredInsertCount == 0 || requiredInsertCount <= s.table.InsertCount() {
		return false, nil
	}
	if s.blockedSections == nil {
		s.blockedSections = make(map[uint64]uint64, 4)
	}
	if _, exists := s.blockedSections[streamID]; !exists && s.maxBlockedStreams > 0 && uint64(len(s.blockedSections)) >= s.maxBlockedStreams {
		return false, ErrQPACKBlockedStreamsExceeded
	}
	s.blockedSections[streamID] = requiredInsertCount
	return true, nil
}

// TrackFieldSection 记录本端已发送的 header section，用于处理 Section Acknowledgment。
func (s *QPACKDynamicState) TrackFieldSection(streamID uint64, requiredInsertCount uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if requiredInsertCount > s.table.InsertCount() {
		return ErrQPACKInvalidInstruction
	}
	if requiredInsertCount == 0 {
		return nil
	}
	if s.trackedSections == nil {
		s.trackedSections = make(map[uint64][]uint64, 4)
	}
	s.trackedSections[streamID] = append(s.trackedSections[streamID], requiredInsertCount)
	return nil
}

// ApplyEncoderInstruction 应用对端 encoder stream 指令。
func (s *QPACKDynamicState) ApplyEncoderInstruction(inst any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch v := inst.(type) {
	case QPACKSetDynamicTableCapacity:
		if v.Capacity > s.maxTableCapacity {
			return ErrQPACKCapacityExceeded
		}
		return s.table.SetCapacity(v.Capacity)
	case QPACKInsertWithNameRef:
		field, err := s.resolveName(v.Static, v.NameIndex)
		if err != nil {
			return err
		}
		field.Value = v.Value
		if _, err := s.table.Insert(field); err != nil {
			return err
		}
		s.releaseUnblocked()
		return nil
	case QPACKInsertWithoutNameRef:
		if _, err := s.table.Insert(v.Field); err != nil {
			return err
		}
		s.releaseUnblocked()
		return nil
	case QPACKDuplicate:
		if _, err := s.table.Duplicate(v.Index); err != nil {
			return err
		}
		s.releaseUnblocked()
		return nil
	default:
		return ErrQPACKInvalidInstruction
	}
}

// ApplyDecoderInstruction 应用对端 decoder stream 反馈指令。
func (s *QPACKDynamicState) ApplyDecoderInstruction(inst any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch v := inst.(type) {
	case QPACKSectionAcknowledgment:
		return s.ackSection(v.StreamID)
	case QPACKStreamCancellation:
		delete(s.trackedSections, v.StreamID)
		return nil
	case QPACKInsertCountIncrement:
		if v.Increment == 0 || s.knownReceivedCount+v.Increment > s.table.InsertCount() {
			return ErrQPACKInvalidInstruction
		}
		s.knownReceivedCount += v.Increment
		return nil
	default:
		return ErrQPACKInvalidInstruction
	}
}

func (s *QPACKDynamicState) resolveName(static bool, index uint64) (HeaderField, error) {
	if static {
		field, ok := qpackStaticField(index)
		if !ok {
			return HeaderField{}, ErrQPACKInvalidIndex
		}
		return HeaderField{Name: field.Name}, nil
	}
	field, ok := s.table.GetRelative(index)
	if !ok {
		return HeaderField{}, ErrQPACKInvalidIndex
	}
	return HeaderField{Name: field.Name}, nil
}

func (s *QPACKDynamicState) releaseUnblocked() {
	count := s.table.InsertCount()
	for streamID, required := range s.blockedSections {
		if required <= count {
			delete(s.blockedSections, streamID)
		}
	}
}

func (s *QPACKDynamicState) ackSection(streamID uint64) error {
	sections := s.trackedSections[streamID]
	if len(sections) == 0 {
		if s.acknowledgedStreams == nil {
			s.acknowledgedStreams = make(map[uint64]struct{}, 4)
		}
		s.acknowledgedStreams[streamID] = struct{}{}
		return nil
	}
	required := sections[0]
	if len(sections) == 1 {
		delete(s.trackedSections, streamID)
	} else {
		copy(sections, sections[1:])
		sections[len(sections)-1] = 0
		s.trackedSections[streamID] = sections[:len(sections)-1]
	}
	if required > s.knownReceivedCount {
		s.knownReceivedCount = required
	}
	return nil
}
