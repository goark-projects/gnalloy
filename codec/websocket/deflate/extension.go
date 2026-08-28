package deflate

import (
	"fmt"
	"strings"
)

const ExtensionName = "permessage-deflate"

// Parameters 描述 permessage-deflate 协商参数。
type Parameters struct {
	ServerNoContextTakeover bool
	ClientNoContextTakeover bool
	ServerMaxWindowBits     string
	ClientMaxWindowBits     string
}

// Offer 返回可写入 Sec-WebSocket-Extensions 的扩展声明。
func Offer(params Parameters) string {
	parts := []string{ExtensionName}
	if params.ServerNoContextTakeover {
		parts = append(parts, "server_no_context_takeover")
	}
	if params.ClientNoContextTakeover {
		parts = append(parts, "client_no_context_takeover")
	}
	if params.ServerMaxWindowBits != "" {
		parts = append(parts, "server_max_window_bits="+params.ServerMaxWindowBits)
	}
	if params.ClientMaxWindowBits != "" {
		parts = append(parts, "client_max_window_bits="+params.ClientMaxWindowBits)
	}
	return strings.Join(parts, "; ")
}

// Parse 在 Sec-WebSocket-Extensions 头部中查找 permessage-deflate。
func Parse(header string) (Parameters, bool, error) {
	for _, ext := range strings.Split(header, ",") {
		parts := strings.Split(ext, ";")
		if len(parts) == 0 || !strings.EqualFold(strings.TrimSpace(parts[0]), ExtensionName) {
			continue
		}
		params, err := parseParameters(parts[1:])
		return params, true, err
	}
	return Parameters{}, false, nil
}

func parseParameters(parts []string) (Parameters, error) {
	var params Parameters
	seen := map[string]struct{}{}
	for _, raw := range parts {
		key, value, _ := strings.Cut(strings.TrimSpace(raw), "=")
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.Trim(strings.TrimSpace(value), `"`)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			return Parameters{}, fmt.Errorf("%w: duplicate %s", ErrInvalidExtension, key)
		}
		seen[key] = struct{}{}
		switch key {
		case "server_no_context_takeover":
			params.ServerNoContextTakeover = true
		case "client_no_context_takeover":
			params.ClientNoContextTakeover = true
		case "server_max_window_bits":
			params.ServerMaxWindowBits = value
		case "client_max_window_bits":
			params.ClientMaxWindowBits = value
		default:
			return Parameters{}, fmt.Errorf("%w: %s", ErrInvalidExtension, key)
		}
	}
	return params, nil
}
