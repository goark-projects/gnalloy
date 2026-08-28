package cookie

import (
	"net/http"
	"strconv"
	"strings"
)

// DecodeCookieHeader 解析请求 Cookie 头部的多个 name=value 对。
func DecodeCookieHeader(header string) ([]Cookie, error) {
	header = stripHeaderPrefix(header, "Cookie")
	if strings.TrimSpace(header) == "" {
		return nil, nil
	}
	cookies := make([]Cookie, 0, 4)
	for len(header) > 0 {
		part := header
		if before, after, ok := strings.Cut(header, ";"); ok {
			part = before
			header = after
		} else {
			header = ""
		}
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return nil, ErrInvalidCookie
		}
		c, err := decodePair(name, value)
		if err != nil {
			return nil, err
		}
		cookies = append(cookies, c)
	}
	return cookies, nil
}

// DecodeSetCookie 解析响应 Set-Cookie 头部。
func DecodeSetCookie(header string) (Cookie, error) {
	header = stripHeaderPrefix(header, "Set-Cookie")
	pair, rest, _ := strings.Cut(header, ";")
	name, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
	if !ok {
		return Cookie{}, ErrInvalidCookie
	}
	c, err := decodePair(name, value)
	if err != nil {
		return Cookie{}, err
	}
	for len(rest) > 0 {
		part := rest
		if before, after, ok := strings.Cut(rest, ";"); ok {
			part = before
			rest = after
		} else {
			rest = ""
		}
		if err := decodeAttribute(&c, strings.TrimSpace(part)); err != nil {
			return Cookie{}, err
		}
	}
	return c, nil
}

func decodePair(name string, value string) (Cookie, error) {
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	if !validName(name) {
		return Cookie{}, ErrInvalidCookie
	}
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return Cookie{}, ErrInvalidCookie
		}
		value = unquoted
	}
	if !validValue(value) {
		return Cookie{}, ErrInvalidCookie
	}
	return Cookie{Name: name, Value: value}, nil
}

func decodeAttribute(c *Cookie, attr string) error {
	if attr == "" {
		return nil
	}
	name, value, hasValue := strings.Cut(attr, "=")
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	switch strings.ToLower(name) {
	case "path":
		if !hasValue || !validAttributeValue(value) {
			return ErrInvalidCookie
		}
		c.Path = value
	case "domain":
		if !hasValue || !validAttributeValue(value) {
			return ErrInvalidCookie
		}
		c.Domain = value
	case "expires":
		if !hasValue {
			return ErrInvalidCookie
		}
		expires, err := http.ParseTime(value)
		if err != nil {
			return ErrInvalidCookie
		}
		c.Expires = expires
	case "max-age":
		if !hasValue {
			return ErrInvalidCookie
		}
		maxAge, err := strconv.Atoi(value)
		if err != nil {
			return ErrInvalidCookie
		}
		c.MaxAge = maxAge
		c.HasMaxAge = true
	case "secure":
		c.Secure = true
	case "httponly":
		c.HTTPOnly = true
	case "samesite":
		if !hasValue {
			return ErrInvalidSameSite
		}
		mode, err := parseSameSite(value)
		if err != nil {
			return err
		}
		c.SameSite = mode
	case "partitioned":
		c.Partitioned = true
	default:
		// 未识别属性按浏览器行为忽略，避免阻断扩展属性。
	}
	return nil
}

func parseSameSite(value string) (SameSite, error) {
	switch strings.ToLower(value) {
	case "lax":
		return SameSiteLax, nil
	case "strict":
		return SameSiteStrict, nil
	case "none":
		return SameSiteNone, nil
	default:
		return SameSiteDefault, ErrInvalidSameSite
	}
}

func stripHeaderPrefix(header string, name string) string {
	header = strings.TrimSpace(header)
	if len(header) <= len(name) || header[len(name)] != ':' {
		return header
	}
	if strings.EqualFold(header[:len(name)], name) {
		return strings.TrimSpace(header[len(name)+1:])
	}
	return header
}

func validName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !isTokenByte(name[i]) {
			return false
		}
	}
	return true
}

func validValue(value string) bool {
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c < 0x21 || c == '"' || c == ',' || c == ';' || c == '\\' || c == 0x7f {
			return false
		}
	}
	return true
}

func validAttributeValue(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c < 0x20 || c == ';' || c == 0x7f {
			return false
		}
	}
	return true
}

func isTokenByte(c byte) bool {
	switch {
	case c >= '0' && c <= '9':
		return true
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	}
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	default:
		return false
	}
}
