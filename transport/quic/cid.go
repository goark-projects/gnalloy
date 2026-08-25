package quic

import "encoding/hex"

const MaxConnectionIDLength = 20

// ConnectionID 使用定长数组保存，避免热路径为短 CID 额外分配。
type ConnectionID struct {
	data [MaxConnectionIDLength]byte
	n    uint8
}

func NewConnectionID(src []byte) (ConnectionID, error) {
	if len(src) > MaxConnectionIDLength {
		return ConnectionID{}, ErrInvalidConnectionID
	}
	var cid ConnectionID
	copy(cid.data[:], src)
	cid.n = uint8(len(src))
	return cid, nil
}

func MustConnectionID(src []byte) ConnectionID {
	cid, err := NewConnectionID(src)
	if err != nil {
		panic(err)
	}
	return cid
}

func (c ConnectionID) Len() int {
	return int(c.n)
}

func (c ConnectionID) Empty() bool {
	return c.n == 0
}

func (c ConnectionID) AppendTo(dst []byte) []byte {
	return append(dst, c.data[:c.n]...)
}

func (c ConnectionID) Equal(other ConnectionID) bool {
	if c.n != other.n {
		return false
	}
	for i := uint8(0); i < c.n; i++ {
		if c.data[i] != other.data[i] {
			return false
		}
	}
	return true
}

func (c ConnectionID) String() string {
	return hex.EncodeToString(c.data[:c.n])
}
