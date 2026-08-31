package h2c

import (
	"encoding/base64"

	"goark.dev/gnalloy/codec"
	"goark.dev/gnalloy/codec/http1"
	"goark.dev/gnalloy/codec/http2"
)

const (
	// ProtocolName 是 HTTP/1.1 Upgrade 里的明文 HTTP/2 协议标识。
	ProtocolName = "h2c"
	// HTTP2SettingsHeader 是 h2c Upgrade 必须携带的 SETTINGS payload header。
	HTTP2SettingsHeader = "HTTP2-Settings"
)

// EncodeHTTP2Settings 将 SETTINGS payload 编码为 RFC 7540 要求的 base64url 无填充 header。
func EncodeHTTP2Settings(settings []http2.Setting) string {
	payload := make([]byte, len(settings)*6)
	for i, setting := range settings {
		offset := i * 6
		payload[offset] = byte(setting.ID >> 8)
		payload[offset+1] = byte(setting.ID)
		payload[offset+2] = byte(setting.Value >> 24)
		payload[offset+3] = byte(setting.Value >> 16)
		payload[offset+4] = byte(setting.Value >> 8)
		payload[offset+5] = byte(setting.Value)
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

// DecodeHTTP2Settings 解析 h2c HTTP2-Settings header。
func DecodeHTTP2Settings(value string) ([]http2.Setting, error) {
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(value)
	}
	if err != nil || len(payload)%6 != 0 {
		return nil, codec.ErrInvalidFrameLength
	}
	settings := make([]http2.Setting, len(payload)/6)
	for i := range settings {
		offset := i * 6
		settings[i] = http2.Setting{
			ID: uint16(payload[offset])<<8 | uint16(payload[offset+1]),
			Value: uint32(payload[offset+2])<<24 |
				uint32(payload[offset+3])<<16 |
				uint32(payload[offset+4])<<8 |
				uint32(payload[offset+5]),
		}
	}
	return settings, nil
}

// ApplyUpgradeHeaders 返回带 h2c Upgrade 和 HTTP2-Settings 的 HTTP/1.1 请求副本。
func ApplyUpgradeHeaders(req http1.Request, settings []http2.Setting) (http1.Request, error) {
	headers := cloneHeaders(req.Headers)
	headers = setHeaderToken(headers, "Connection", "Upgrade")
	headers = setHeaderToken(headers, "Connection", HTTP2SettingsHeader)
	headers.Set("Upgrade", ProtocolName)
	headers.Set(HTTP2SettingsHeader, EncodeHTTP2Settings(settings))
	req.Headers = headers
	if req.Method == "" {
		req.Method = "GET"
	}
	if req.URI == "" {
		req.URI = "/"
	}
	if req.Version == "" {
		req.Version = "HTTP/1.1"
	}
	return req, nil
}

func cloneHeaders(headers http1.Headers) http1.Headers {
	out := make(http1.Headers, len(headers)+3)
	for name, value := range headers {
		out[name] = value
	}
	return out
}

func setHeaderToken(headers http1.Headers, name string, token string) http1.Headers {
	if headers == nil {
		headers = http1.Headers{}
	}
	if headers.ContainsToken(name, token) {
		return headers
	}
	current := headers.Get(name)
	if current == "" {
		headers.Set(name, token)
		return headers
	}
	headers.Set(name, current+", "+token)
	return headers
}
