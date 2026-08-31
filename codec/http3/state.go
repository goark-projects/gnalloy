package http3

import (
	"sync"

	"goark.dev/gnalloy/channel"
)

// StateManagerConfig 描述 HTTP/3 连接级状态管理参数。
type StateManagerConfig struct {
	// Server 表示本端是服务端；用于校验 push 方向。
	Server bool
	// InitialMaxPushID 是本端允许对端使用的最大 push ID。
	InitialMaxPushID uint64
}

// StateManager 维护 HTTP/3 SETTINGS、GOAWAY 和 server push 状态。
type StateManager struct {
	mu                 sync.Mutex
	cfg                StateManagerConfig
	seenLocalSettings  bool
	seenRemoteSettings bool
	localSettings      []Setting
	remoteSettings     []Setting
	localMaxPushID     uint64
	remoteMaxPushID    uint64
	remoteMaxPushIDSet bool
	inboundGoAwaySet   bool
	inboundGoAwayID    uint64
	outboundGoAwaySet  bool
	outboundGoAwayID   uint64
	pushes             map[uint64]PushState
}

// NewStateManager 创建 HTTP/3 连接级状态 handler。
func NewStateManager(cfg StateManagerConfig) (*StateManager, error) {
	if cfg.InitialMaxPushID > maxVarInt {
		return nil, ErrInvalidVarInt
	}
	return &StateManager{cfg: cfg, localMaxPushID: cfg.InitialMaxPushID}, nil
}

// LocalSettings 返回本端已发送 SETTINGS 快照。
func (m *StateManager) LocalSettings() []Setting {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneSettings(m.localSettings)
}

// RemoteSettings 返回对端已接收 SETTINGS 快照。
func (m *StateManager) RemoteSettings() []Setting {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneSettings(m.remoteSettings)
}

// LocalMaxPushID 返回本端允许对端使用的最大 push ID。
func (m *StateManager) LocalMaxPushID() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.localMaxPushID
}

// RemoteMaxPushID 返回对端允许本端使用的最大 push ID。
func (m *StateManager) RemoteMaxPushID() (uint64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.remoteMaxPushID, m.remoteMaxPushIDSet
}

// PushState 返回指定 push ID 的当前状态。
func (m *StateManager) PushState(pushID uint64) PushState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pushes == nil {
		return PushStateIdle
	}
	return m.pushes[pushID]
}

// ChannelRead 应用入站 HTTP/3 连接级状态后继续传播 frame。
func (m *StateManager) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if err := m.readState(msg); err != nil {
		releaseMessage(msg)
		ctx.FireExceptionCaught(err)
		return
	}
	ctx.FireChannelRead(msg)
}

// Write 应用出站 HTTP/3 连接级状态。
func (m *StateManager) Write(ctx *channel.HandlerContext, msg any) error {
	if err := m.writeState(msg); err != nil {
		releaseMessage(msg)
		return err
	}
	return ctx.Write(msg)
}
