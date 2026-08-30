package benchtls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"
)

const DefaultServerName = "gnalloy.local"

// CertificateKeyAlgorithm 表示 benchmark 临时证书的公钥算法。
type CertificateKeyAlgorithm uint8

const (
	// CertificateKeyECDSA 使用 P-256 ECDSA 证书，适配 ECDHE_ECDSA 与 TLS 1.3。
	CertificateKeyECDSA CertificateKeyAlgorithm = iota + 1
	// CertificateKeyRSA 使用 2048 位 RSA 证书，适配 RSA 认证类 TLS 1.1/1.2 套件。
	CertificateKeyRSA
)

// SelfSignedCertificate 生成仅供 benchmark 进程内使用的临时 ECDSA 证书。
func SelfSignedCertificate(serverName string) (tls.Certificate, error) {
	return SelfSignedCertificateWithAlgorithm(serverName, CertificateKeyECDSA)
}

// SelfSignedCertificateWithAlgorithm 按指定公钥算法生成 benchmark 临时证书。
func SelfSignedCertificateWithAlgorithm(serverName string, algorithm CertificateKeyAlgorithm) (tls.Certificate, error) {
	name := normalizeServerName(serverName)
	key, err := generatePrivateKey(algorithm)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("benchtls: generate key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := cryptorand.Int(cryptorand.Reader, serialLimit)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("benchtls: generate serial: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     certificateKeyUsage(algorithm),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(name); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{name}
	}
	publicKey, err := certificatePublicKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	certDER, err := x509.CreateCertificate(cryptorand.Reader, template, template, publicKey, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("benchtls: create certificate: %w", err)
	}
	keyPEM, err := encodePrivateKeyPEM(key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("benchtls: marshal private key: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("benchtls: load key pair: %w", err)
	}
	return cert, nil
}

// SelfSignedCertificates 按顺序生成多张 benchmark 临时证书。
func SelfSignedCertificates(serverName string, algorithms ...CertificateKeyAlgorithm) ([]tls.Certificate, error) {
	if len(algorithms) == 0 {
		algorithms = []CertificateKeyAlgorithm{CertificateKeyECDSA}
	}
	certs := make([]tls.Certificate, 0, len(algorithms))
	seen := make(map[CertificateKeyAlgorithm]struct{}, len(algorithms))
	for _, algorithm := range algorithms {
		if _, ok := seen[algorithm]; ok {
			continue
		}
		seen[algorithm] = struct{}{}
		cert, err := SelfSignedCertificateWithAlgorithm(serverName, algorithm)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}
	return certs, nil
}

func generatePrivateKey(algorithm CertificateKeyAlgorithm) (any, error) {
	switch algorithm {
	case CertificateKeyECDSA:
		return ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
	case CertificateKeyRSA:
		return rsa.GenerateKey(cryptorand.Reader, 2048)
	default:
		return nil, fmt.Errorf("unsupported certificate key algorithm %d", algorithm)
	}
}

func certificateKeyUsage(algorithm CertificateKeyAlgorithm) x509.KeyUsage {
	if algorithm == CertificateKeyRSA {
		return x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
	}
	return x509.KeyUsageDigitalSignature
}

func certificatePublicKey(key any) (any, error) {
	switch typed := key.(type) {
	case *ecdsa.PrivateKey:
		return typed.Public(), nil
	case *rsa.PrivateKey:
		return typed.Public(), nil
	default:
		return nil, fmt.Errorf("unsupported private key type %T", key)
	}
}

func encodePrivateKeyPEM(key any) ([]byte, error) {
	switch typed := key.(type) {
	case *ecdsa.PrivateKey:
		keyDER, err := x509.MarshalECPrivateKey(typed)
		if err != nil {
			return nil, err
		}
		return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
	case *rsa.PrivateKey:
		keyDER := x509.MarshalPKCS1PrivateKey(typed)
		return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER}), nil
	default:
		return nil, fmt.Errorf("unsupported private key type %T", key)
	}
}

func normalizeServerName(serverName string) string {
	name := strings.TrimSpace(serverName)
	if name == "" {
		return DefaultServerName
	}
	return name
}
