package unix

import "strings"

const maxSocketPathLength = 103

// Address 描述 Unix domain socket 地址。
type Address struct {
	Path     string
	Abstract bool
}

// ParseAddress 解析 Unix domain socket 地址。
//
// 支持普通文件路径和 unix:// 前缀；以 @ 开头的地址表示 Linux abstract socket。
func ParseAddress(address string) (Address, error) {
	value := strings.TrimSpace(address)
	value = strings.TrimPrefix(value, "unix://")
	if value == "" {
		return Address{}, ErrInvalidAddress
	}
	abstract := false
	if strings.HasPrefix(value, "@") {
		abstract = true
		value = strings.TrimPrefix(value, "@")
		if value == "" {
			return Address{}, ErrInvalidAddress
		}
	}
	if len(value) > maxSocketPathLength {
		return Address{}, ErrPathTooLong
	}
	return Address{Path: value, Abstract: abstract}, nil
}

func (a Address) String() string {
	if a.Abstract {
		return "unix://@" + a.Path
	}
	return "unix://" + a.Path
}

func (a Address) sockaddrName() string {
	if a.Abstract {
		return "\x00" + a.Path
	}
	return a.Path
}
