package tls

import (
	"fmt"

	cryptotls "crypto/tls"
)

// ConfigureCipherSuites 将 TLS 1.0-1.2 密码套件复制到 crypto/tls 配置。
func ConfigureCipherSuites(cfg *cryptotls.Config, suites []uint16) error {
	if len(suites) == 0 {
		return nil
	}
	if cfg == nil {
		return fmt.Errorf("%w: nil tls config", ErrInvalidConfig)
	}
	if err := ValidateConfigurableCipherSuites(suites); err != nil {
		return err
	}
	cfg.CipherSuites = cloneUint16s(suites)
	return nil
}

// ConfigureCipherSuiteNames 解析并应用密码套件名称，适合配置文件和命令行入口使用。
func ConfigureCipherSuiteNames(cfg *cryptotls.Config, value string, options CipherSuiteOptions) error {
	suites, err := ParseCipherSuites(value, options)
	if err != nil {
		return err
	}
	return ConfigureCipherSuites(cfg, suites)
}

// ValidateConfigurableCipherSuites 校验给定 ID 是否能写入 crypto/tls.Config.CipherSuites。
func ValidateConfigurableCipherSuites(suites []uint16) error {
	registry := getCipherSuiteRegistry()
	for _, id := range suites {
		info, ok := registry.byID[id]
		if !ok {
			return fmt.Errorf("%w: 0x%04X", ErrUnknownCipherSuite, id)
		}
		if !info.Configurable {
			return fmt.Errorf("%w: %s", ErrTLS13CipherSuiteNotConfigurable, info.Name)
		}
	}
	return nil
}
