package tls

import (
	"sort"
	"strings"
	"sync"

	cryptotls "crypto/tls"
)

type cipherSuiteRegistry struct {
	secure []CipherSuiteInfo
	all    []CipherSuiteInfo
	byID   map[uint16]CipherSuiteInfo
	byKey  map[string]CipherSuiteInfo
}

var (
	cipherSuiteRegistryOnce sync.Once
	cipherSuiteRegistryData cipherSuiteRegistry
)

// CipherSuiteCatalog 返回当前运行时支持的密码套件目录。
func CipherSuiteCatalog(options CipherSuiteOptions) []CipherSuiteInfo {
	registry := getCipherSuiteRegistry()
	source := registry.secure
	if options.IncludeInsecure {
		source = registry.all
	}
	out := make([]CipherSuiteInfo, len(source))
	copy(out, source)
	for i := range out {
		out[i].SupportedVersions = cloneUint16s(out[i].SupportedVersions)
	}
	return out
}

func getCipherSuiteRegistry() cipherSuiteRegistry {
	cipherSuiteRegistryOnce.Do(func() {
		cipherSuiteRegistryData = buildCipherSuiteRegistry()
	})
	return cipherSuiteRegistryData
}

func buildCipherSuiteRegistry() cipherSuiteRegistry {
	secureRuntime := cryptotls.CipherSuites()
	insecureRuntime := cryptotls.InsecureCipherSuites()
	registry := cipherSuiteRegistry{
		secure: make([]CipherSuiteInfo, 0, len(secureRuntime)),
		all:    make([]CipherSuiteInfo, 0, len(secureRuntime)+len(insecureRuntime)),
		byID:   make(map[uint16]CipherSuiteInfo, len(secureRuntime)+len(insecureRuntime)),
		byKey:  make(map[string]CipherSuiteInfo, (len(secureRuntime)+len(insecureRuntime))*4),
	}
	for _, suite := range secureRuntime {
		registry.add(suite)
	}
	for _, suite := range insecureRuntime {
		registry.add(suite)
	}
	sortCipherSuites(registry.secure)
	sortCipherSuites(registry.all)
	return registry
}

func (r *cipherSuiteRegistry) add(suite *cryptotls.CipherSuite) {
	if suite == nil {
		return
	}
	info := CipherSuiteInfo{
		ID:                suite.ID,
		Name:              suite.Name,
		OpenSSLName:       cipherSuiteOpenSSLName(suite.Name),
		SupportedVersions: cloneUint16s(suite.SupportedVersions),
		Insecure:          suite.Insecure,
		Configurable:      cipherSuiteConfigurable(suite.SupportedVersions),
		CertificateAuth:   cipherSuiteCertificateAuth(suite.Name, suite.SupportedVersions),
	}
	r.all = append(r.all, info)
	if !info.Insecure {
		r.secure = append(r.secure, info)
	}
	r.byID[info.ID] = info
	r.addAlias(info, info.Name)
	r.addAlias(info, strings.TrimPrefix(info.Name, "TLS_"))
	if info.OpenSSLName != "" {
		r.addAlias(info, info.OpenSSLName)
	}
	if info.Configurable {
		r.addAlias(info, "SSL_"+strings.TrimPrefix(info.Name, "TLS_"))
	}
	for _, alias := range cipherSuiteExtraAliases(info.Name) {
		r.addAlias(info, alias)
	}
}

func (r *cipherSuiteRegistry) addAlias(info CipherSuiteInfo, alias string) {
	key := cipherSuiteKey(alias)
	if key == "" {
		return
	}
	r.byKey[key] = info
}

func cipherSuiteConfigurable(versions []uint16) bool {
	for _, version := range versions {
		if version < cryptotls.VersionTLS13 {
			return true
		}
	}
	return false
}

func sortCipherSuites(suites []CipherSuiteInfo) {
	sort.Slice(suites, func(i, j int) bool {
		return suites[i].ID < suites[j].ID
	})
}

func cloneUint16s(values []uint16) []uint16 {
	if len(values) == 0 {
		return nil
	}
	out := make([]uint16, len(values))
	copy(out, values)
	return out
}
