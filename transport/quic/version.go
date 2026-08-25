package quic

import "fmt"

// Version 是 QUIC 版本号，当前先固定 RFC 9000 的 v1。
type Version uint32

const Version1 Version = 0x00000001

func (v Version) Valid() bool {
	return v == Version1
}

func (v Version) String() string {
	if v == Version1 {
		return "v1"
	}
	return fmt.Sprintf("0x%08x", uint32(v))
}
