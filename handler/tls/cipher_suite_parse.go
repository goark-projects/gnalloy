package tls

import (
	"strconv"
	"strings"
)

// ParseCipherSuites 解析逗号、冒号、分号或空白分隔的密码套件列表。
func ParseCipherSuites(value string, options CipherSuiteOptions) ([]uint16, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	tokens := splitCipherSuiteTokens(value)
	if len(tokens) == 0 {
		return nil, ErrUnknownCipherSuite
	}
	seen := make(map[uint16]struct{}, len(tokens))
	ids := make([]uint16, 0, len(tokens))
	for _, token := range tokens {
		info, err := LookupCipherSuite(token, options)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[info.ID]; ok {
			continue
		}
		seen[info.ID] = struct{}{}
		ids = append(ids, info.ID)
	}
	return ids, nil
}

func splitCipherSuiteTokens(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ':' || r == ';' || r == ' ' || r == '\t' || r == '\r' || r == '\n'
	})
}

func parseCipherSuiteID(token string) (uint16, bool) {
	value := strings.TrimSpace(token)
	if value == "" {
		return 0, false
	}
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") {
		parsed, err := strconv.ParseUint(value[2:], 16, 16)
		return uint16(parsed), err == nil
	}
	if isHexCipherSuiteID(value) {
		parsed, err := strconv.ParseUint(value, 16, 16)
		return uint16(parsed), err == nil
	}
	if isDecimalCipherSuiteID(value) {
		parsed, err := strconv.ParseUint(value, 10, 16)
		return uint16(parsed), err == nil
	}
	return 0, false
}

func isHexCipherSuiteID(value string) bool {
	if len(value) == 0 || len(value) > 4 {
		return false
	}
	hasHexLetter := false
	for _, r := range value {
		if r >= '0' && r <= '9' {
			continue
		}
		if r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' {
			hasHexLetter = true
			continue
		}
		return false
	}
	return hasHexLetter
}

func isDecimalCipherSuiteID(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func cipherSuiteKey(name string) string {
	key := strings.ToUpper(strings.TrimSpace(name))
	key = strings.ReplaceAll(key, "-", "_")
	key = strings.ReplaceAll(key, " ", "")
	return key
}
