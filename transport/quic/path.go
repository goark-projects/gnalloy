package quic

import (
	"crypto/rand"
	"io"

	"goark.dev/gnalloy/transport/udp"
)

// PathState 描述 QUIC 连接路径状态。
type PathState uint8

const (
	PathStateActive PathState = iota + 1
	PathStateValidating
	PathStateValidated
)

// Path 描述一个可用于连接迁移的 UDP 路径。
type Path struct {
	Remote    udp.Address
	State     PathState
	Challenge [8]byte
}

// PathManager 管理 QUIC 当前路径和候选迁移路径。
type PathManager struct {
	active udp.Address
	paths  map[string]*Path
	rand   io.Reader
}

// NewPathManager 创建路径管理器。
func NewPathManager(active udp.Address) *PathManager {
	return &PathManager{
		active: active,
		paths: map[string]*Path{
			active.String(): {Remote: active, State: PathStateActive},
		},
		rand: rand.Reader,
	}
}

// Active 返回当前活动路径。
func (m *PathManager) Active() udp.Address {
	if m == nil {
		return udp.Address{}
	}
	return m.active
}

// Challenge 为目标路径生成 PATH_CHALLENGE。
func (m *PathManager) Challenge(remote udp.Address) (PathChallengeFrame, error) {
	if m == nil {
		return PathChallengeFrame{}, ErrInvalidConfig
	}
	path := m.path(remote)
	path.State = PathStateValidating
	if _, err := io.ReadFull(m.rand, path.Challenge[:]); err != nil {
		return PathChallengeFrame{}, err
	}
	return PathChallengeFrame{Data: path.Challenge}, nil
}

// Validate 使用 PATH_RESPONSE 验证候选路径。
func (m *PathManager) Validate(remote udp.Address, frame PathResponseFrame) bool {
	if m == nil {
		return false
	}
	path := m.paths[remote.String()]
	if path == nil || path.State != PathStateValidating || path.Challenge != frame.Data {
		return false
	}
	path.State = PathStateValidated
	return true
}

// Migrate 将活动路径切换到已验证路径。
func (m *PathManager) Migrate(remote udp.Address) error {
	if m == nil {
		return ErrInvalidConfig
	}
	path := m.paths[remote.String()]
	if path == nil || path.State != PathStateValidated {
		return ErrInvalidPacket
	}
	if active := m.paths[m.active.String()]; active != nil {
		active.State = PathStateValidated
	}
	path.State = PathStateActive
	m.active = remote
	return nil
}

func (m *PathManager) path(remote udp.Address) *Path {
	key := remote.String()
	if path := m.paths[key]; path != nil {
		return path
	}
	path := &Path{Remote: remote, State: PathStateValidating}
	m.paths[key] = path
	return path
}
