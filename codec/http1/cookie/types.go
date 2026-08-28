package cookie

import "time"

// SameSite 描述 Set-Cookie 的 SameSite 属性。
type SameSite uint8

const (
	// SameSiteDefault 表示不输出 SameSite 属性。
	SameSiteDefault SameSite = iota
	SameSiteLax
	SameSiteStrict
	SameSiteNone
)

// Cookie 描述请求 Cookie 或响应 Set-Cookie。
type Cookie struct {
	Name        string
	Value       string
	Path        string
	Domain      string
	Expires     time.Time
	MaxAge      int
	HasMaxAge   bool
	Secure      bool
	HTTPOnly    bool
	SameSite    SameSite
	Partitioned bool
}

// SameSiteString 返回线协议使用的 SameSite 文本。
func SameSiteString(mode SameSite) (string, bool) {
	switch mode {
	case SameSiteDefault:
		return "", true
	case SameSiteLax:
		return "Lax", true
	case SameSiteStrict:
		return "Strict", true
	case SameSiteNone:
		return "None", true
	default:
		return "", false
	}
}
