package tls

import (
	"fmt"

	cryptotls "crypto/tls"
)

// OCSPConfig 描述 TLS handler 对 stapled OCSP 响应的处理策略。
type OCSPConfig struct {
	// RequireStaple 要求对端必须在握手中返回 stapled OCSP 响应。
	RequireStaple bool
	// EmitEvent 控制握手后是否向 pipeline 发出 OCSPEvent。
	EmitEvent bool
	// Validator 校验 stapled OCSP 响应；nil 表示只暴露响应，不做加密学判定。
	Validator OCSPValidator
}

// OCSPValidator 校验对端随证书携带的 stapled OCSP 响应。
type OCSPValidator interface {
	ValidateOCSP(state cryptotls.ConnectionState, response []byte) error
}

// OCSPValidatorFunc 把函数适配为 OCSPValidator。
type OCSPValidatorFunc func(state cryptotls.ConnectionState, response []byte) error

// ValidateOCSP 执行函数式 OCSP 校验器。
func (f OCSPValidatorFunc) ValidateOCSP(state cryptotls.ConnectionState, response []byte) error {
	if f == nil {
		return nil
	}
	return f(state, response)
}

// OCSPEvent 是 TLS 握手后发给业务 pipeline 的 OCSP 状态快照。
type OCSPEvent struct {
	Response  []byte
	Stapled   bool
	Validated bool
}

func (cfg OCSPConfig) evaluate(state cryptotls.ConnectionState) (OCSPEvent, bool, error) {
	response := state.OCSPResponse
	stapled := len(response) > 0
	if cfg.RequireStaple && !stapled {
		return OCSPEvent{Stapled: false}, false, ErrOCSPStapleRequired
	}

	validated := false
	if stapled && cfg.Validator != nil {
		if err := cfg.Validator.ValidateOCSP(state, response); err != nil {
			return OCSPEvent{}, false, fmt.Errorf("%w: %w", ErrOCSPValidationFailed, err)
		}
		validated = true
	}

	event := OCSPEvent{
		Response:  cloneOCSPBytes(response),
		Stapled:   stapled,
		Validated: validated,
	}
	return event, cfg.EmitEvent, nil
}

// CertificateWithOCSPStaple 返回携带 stapled OCSP 响应的证书副本。
func CertificateWithOCSPStaple(cert cryptotls.Certificate, response []byte) (cryptotls.Certificate, error) {
	if len(response) == 0 {
		return cryptotls.Certificate{}, ErrInvalidConfig
	}
	out := cert
	out.Certificate = cloneOCSPByteSlices(cert.Certificate)
	out.SignedCertificateTimestamps = cloneOCSPByteSlices(cert.SignedCertificateTimestamps)
	out.OCSPStaple = cloneOCSPBytes(response)
	return out, nil
}

func cloneOCSPBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	return append([]byte(nil), src...)
}

func cloneOCSPByteSlices(src [][]byte) [][]byte {
	if len(src) == 0 {
		return nil
	}
	out := make([][]byte, len(src))
	for i := range src {
		out[i] = cloneOCSPBytes(src[i])
	}
	return out
}
