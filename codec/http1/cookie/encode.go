package cookie

import (
	"net/http"
	"strconv"
)

// EncodeCookieHeader 编码请求 Cookie 头部值。
func EncodeCookieHeader(cookies []Cookie) (string, error) {
	out, err := AppendCookieHeader(nil, cookies)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// AppendCookieHeader 向 dst 追加请求 Cookie 头部值。
func AppendCookieHeader(dst []byte, cookies []Cookie) ([]byte, error) {
	for i, c := range cookies {
		if err := validateCookie(c); err != nil {
			return dst, err
		}
		if i > 0 {
			dst = append(dst, "; "...)
		}
		dst = append(dst, c.Name...)
		dst = append(dst, '=')
		dst = append(dst, c.Value...)
	}
	return dst, nil
}

// EncodeSetCookie 编码响应 Set-Cookie 头部值。
func EncodeSetCookie(c Cookie) (string, error) {
	out, err := AppendSetCookie(nil, c)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// AppendSetCookie 向 dst 追加响应 Set-Cookie 头部值。
func AppendSetCookie(dst []byte, c Cookie) ([]byte, error) {
	if err := validateCookie(c); err != nil {
		return dst, err
	}
	dst = append(dst, c.Name...)
	dst = append(dst, '=')
	dst = append(dst, c.Value...)
	if c.Path != "" {
		if !validAttributeValue(c.Path) {
			return dst, ErrInvalidCookie
		}
		dst = append(dst, "; Path="...)
		dst = append(dst, c.Path...)
	}
	if c.Domain != "" {
		if !validAttributeValue(c.Domain) {
			return dst, ErrInvalidCookie
		}
		dst = append(dst, "; Domain="...)
		dst = append(dst, c.Domain...)
	}
	if c.HasMaxAge {
		dst = append(dst, "; Max-Age="...)
		dst = strconv.AppendInt(dst, int64(c.MaxAge), 10)
	}
	if !c.Expires.IsZero() {
		dst = append(dst, "; Expires="...)
		dst = c.Expires.UTC().AppendFormat(dst, http.TimeFormat)
	}
	if c.Secure {
		dst = append(dst, "; Secure"...)
	}
	if c.HTTPOnly {
		dst = append(dst, "; HttpOnly"...)
	}
	if c.SameSite != SameSiteDefault {
		text, ok := SameSiteString(c.SameSite)
		if !ok {
			return dst, ErrInvalidSameSite
		}
		dst = append(dst, "; SameSite="...)
		dst = append(dst, text...)
	}
	if c.Partitioned {
		dst = append(dst, "; Partitioned"...)
	}
	return dst, nil
}

func validateCookie(c Cookie) error {
	if !validName(c.Name) || !validValue(c.Value) {
		return ErrInvalidCookie
	}
	if _, ok := SameSiteString(c.SameSite); !ok {
		return ErrInvalidSameSite
	}
	return nil
}
