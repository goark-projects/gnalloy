package channel

import "sync"

type attributeKeyID string

// AttributeKey 是 Channel 级别的类型安全属性键。
type AttributeKey[T any] struct {
	name attributeKeyID
}

func NewAttributeKey[T any](name string) AttributeKey[T] {
	return AttributeKey[T]{name: attributeKeyID(name)}
}

func (k AttributeKey[T]) Name() string {
	return string(k.name)
}

func (k AttributeKey[T]) Get(attrs *AttributeMap) (T, bool) {
	var zero T
	if attrs == nil {
		return zero, false
	}
	value, ok := attrs.get(k.name)
	if !ok {
		return zero, false
	}
	typed, ok := value.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}

func (k AttributeKey[T]) Set(attrs *AttributeMap, value T) {
	if attrs == nil {
		return
	}
	attrs.set(k.name, value)
}

func (k AttributeKey[T]) GetOrSet(attrs *AttributeMap, value T) T {
	if got, ok := k.Get(attrs); ok {
		return got
	}
	k.Set(attrs, value)
	return value
}

func (k AttributeKey[T]) Remove(attrs *AttributeMap) (T, bool) {
	var zero T
	if attrs == nil {
		return zero, false
	}
	value, ok := attrs.remove(k.name)
	if !ok {
		return zero, false
	}
	typed, ok := value.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}

// AttributeMap 存储 Handler 间共享的 Channel 局部状态。
type AttributeMap struct {
	mu     sync.RWMutex
	values map[attributeKeyID]any
}

func NewAttributeMap() *AttributeMap {
	return &AttributeMap{}
}

func (m *AttributeMap) get(key attributeKeyID) (any, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.values[key]
	return value, ok
}

func (m *AttributeMap) set(key attributeKeyID, value any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.values == nil {
		m.values = make(map[attributeKeyID]any, 4)
	}
	m.values[key] = value
}

func (m *AttributeMap) remove(key attributeKeyID) (any, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.values[key]
	if ok {
		delete(m.values, key)
	}
	return value, ok
}
