package tls

import (
	"fmt"
	"strings"
)

// LookupCipherSuite 按 IANA/Java 名称、OpenSSL 名称或十六进制 ID 查找密码套件。
func LookupCipherSuite(name string, options CipherSuiteOptions) (CipherSuiteInfo, error) {
	token := strings.TrimSpace(name)
	if token == "" {
		return CipherSuiteInfo{}, fmt.Errorf("%w: empty name", ErrUnknownCipherSuite)
	}
	if id, parsed := parseCipherSuiteID(token); parsed {
		return LookupCipherSuiteID(id, options)
	}
	info, ok := getCipherSuiteRegistry().byKey[cipherSuiteKey(token)]
	if !ok {
		return CipherSuiteInfo{}, fmt.Errorf("%w: %s", ErrUnknownCipherSuite, token)
	}
	return checkedCipherSuiteInfo(info, token, options)
}

// LookupCipherSuiteID 按 uint16 ID 查找当前 Go 运行时可识别的密码套件。
func LookupCipherSuiteID(id uint16, options CipherSuiteOptions) (CipherSuiteInfo, error) {
	info, ok := getCipherSuiteRegistry().byID[id]
	if !ok {
		return CipherSuiteInfo{}, fmt.Errorf("%w: 0x%04X", ErrUnknownCipherSuite, id)
	}
	return checkedCipherSuiteInfo(info, fmt.Sprintf("0x%04X", id), options)
}

func checkedCipherSuiteInfo(info CipherSuiteInfo, token string, options CipherSuiteOptions) (CipherSuiteInfo, error) {
	if info.Insecure && !options.IncludeInsecure {
		return CipherSuiteInfo{}, fmt.Errorf("%w: %s", ErrInsecureCipherSuite, token)
	}
	info.SupportedVersions = cloneUint16s(info.SupportedVersions)
	return info, nil
}
