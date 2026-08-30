package tls

import cryptotls "crypto/tls"

// CipherSuiteOptions 控制密码套件目录和名称解析策略。
type CipherSuiteOptions struct {
	// IncludeInsecure 显式纳入 Go 运行时标记为不安全的兼容性套件。
	IncludeInsecure bool
}

// CipherSuiteInfo 描述当前 Go 运行时可识别的 TLS 密码套件。
type CipherSuiteInfo struct {
	ID                uint16
	Name              string
	OpenSSLName       string
	SupportedVersions []uint16
	Insecure          bool
	Configurable      bool
	CertificateAuth   CipherSuiteCertificateAuth
}

// CipherSuiteCertificateAuth 描述密码套件绑定的服务端证书认证族。
type CipherSuiteCertificateAuth uint8

const (
	// CipherSuiteCertificateAny 表示密码套件不绑定证书算法，典型场景是 TLS 1.3。
	CipherSuiteCertificateAny CipherSuiteCertificateAuth = iota
	// CipherSuiteCertificateRSA 表示套件需要 RSA 证书参与握手认证。
	CipherSuiteCertificateRSA
	// CipherSuiteCertificateECDSA 表示套件需要 ECDSA 证书参与握手认证。
	CipherSuiteCertificateECDSA
	// CipherSuiteCertificateUnknown 表示当前运行时返回了 Gnalloy 未分类的新套件。
	CipherSuiteCertificateUnknown
)

func (a CipherSuiteCertificateAuth) String() string {
	switch a {
	case CipherSuiteCertificateAny:
		return "any"
	case CipherSuiteCertificateRSA:
		return "rsa"
	case CipherSuiteCertificateECDSA:
		return "ecdsa"
	default:
		return "unknown"
	}
}

// CipherSuiteName 返回运行时标准 IANA/Java 密码套件名称。
func CipherSuiteName(id uint16) string {
	return cryptotls.CipherSuiteName(id)
}

// CipherSuiteNames 返回运行时标准 IANA/Java 密码套件名称列表。
func CipherSuiteNames(ids []uint16) []string {
	if len(ids) == 0 {
		return nil
	}
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = CipherSuiteName(id)
	}
	return names
}

// OpenSSLCipherSuiteName 返回 Netty/OpenSSL 生态常用的密码套件名称。
func OpenSSLCipherSuiteName(id uint16) (string, bool) {
	info, ok := getCipherSuiteRegistry().byID[id]
	if !ok || info.OpenSSLName == "" {
		return "", false
	}
	return info.OpenSSLName, true
}
