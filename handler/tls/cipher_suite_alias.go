package tls

import (
	cryptotls "crypto/tls"
	"strings"
)

var cipherSuiteOpenSSLNames = map[string]string{
	"TLS_AES_128_GCM_SHA256":                        "TLS_AES_128_GCM_SHA256",
	"TLS_AES_256_GCM_SHA384":                        "TLS_AES_256_GCM_SHA384",
	"TLS_CHACHA20_POLY1305_SHA256":                  "TLS_CHACHA20_POLY1305_SHA256",
	"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA":          "ECDHE-ECDSA-AES128-SHA",
	"TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA":          "ECDHE-ECDSA-AES256-SHA",
	"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA":            "ECDHE-RSA-AES128-SHA",
	"TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA":            "ECDHE-RSA-AES256-SHA",
	"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256":       "ECDHE-ECDSA-AES128-GCM-SHA256",
	"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384":       "ECDHE-ECDSA-AES256-GCM-SHA384",
	"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":         "ECDHE-RSA-AES128-GCM-SHA256",
	"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":         "ECDHE-RSA-AES256-GCM-SHA384",
	"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256":   "ECDHE-RSA-CHACHA20-POLY1305",
	"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256": "ECDHE-ECDSA-CHACHA20-POLY1305",
	"TLS_RSA_WITH_RC4_128_SHA":                      "RC4-SHA",
	"TLS_RSA_WITH_3DES_EDE_CBC_SHA":                 "DES-CBC3-SHA",
	"TLS_RSA_WITH_AES_128_CBC_SHA":                  "AES128-SHA",
	"TLS_RSA_WITH_AES_256_CBC_SHA":                  "AES256-SHA",
	"TLS_RSA_WITH_AES_128_CBC_SHA256":               "AES128-SHA256",
	"TLS_RSA_WITH_AES_128_GCM_SHA256":               "AES128-GCM-SHA256",
	"TLS_RSA_WITH_AES_256_GCM_SHA384":               "AES256-GCM-SHA384",
	"TLS_ECDHE_ECDSA_WITH_RC4_128_SHA":              "ECDHE-ECDSA-RC4-SHA",
	"TLS_ECDHE_RSA_WITH_RC4_128_SHA":                "ECDHE-RSA-RC4-SHA",
	"TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA":           "ECDHE-RSA-DES-CBC3-SHA",
	"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256":       "ECDHE-ECDSA-AES128-SHA256",
	"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256":         "ECDHE-RSA-AES128-SHA256",
}

var cipherSuiteAdditionalAliases = map[string][]string{
	"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256": {
		"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305",
		"ECDHE-RSA-CHACHA20-POLY1305-SHA256",
	},
	"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256": {
		"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305",
		"ECDHE-ECDSA-CHACHA20-POLY1305-SHA256",
	},
}

func cipherSuiteOpenSSLName(name string) string {
	return cipherSuiteOpenSSLNames[name]
}

func cipherSuiteExtraAliases(name string) []string {
	return cipherSuiteAdditionalAliases[name]
}

func cipherSuiteCertificateAuth(name string, versions []uint16) CipherSuiteCertificateAuth {
	if cipherSuiteTLS13Only(versions) {
		return CipherSuiteCertificateAny
	}
	switch {
	case strings.HasPrefix(name, "TLS_RSA_"), strings.Contains(name, "_RSA_"):
		return CipherSuiteCertificateRSA
	case strings.Contains(name, "_ECDSA_"):
		return CipherSuiteCertificateECDSA
	default:
		return CipherSuiteCertificateUnknown
	}
}

func cipherSuiteTLS13Only(versions []uint16) bool {
	if len(versions) == 0 {
		return false
	}
	for _, version := range versions {
		if version != cryptotls.VersionTLS13 {
			return false
		}
	}
	return true
}
