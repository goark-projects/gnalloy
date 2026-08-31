package quic

import "fmt"

// Version 是 RFC9000 适配层公开的 QUIC 版本号类型。
type Version uint32

// Version1 是 RFC 9000 定义的 QUIC v1。
const Version1 Version = 0x00000001

// Valid 判断当前版本是否由生产适配层支持。
func (v Version) Valid() bool {
	return v == Version1
}

// String 返回稳定可读的 QUIC 版本文本。
func (v Version) String() string {
	if v == Version1 {
		return "v1"
	}
	return fmt.Sprintf("0x%08x", uint32(v))
}
