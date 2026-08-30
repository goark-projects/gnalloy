package main

import (
	cryptotls "crypto/tls"

	"goark.dev/gnalloy/benchmarks/external/internal/benchtls"
	handlertls "goark.dev/gnalloy/handler/tls"
)

func benchmarkCertificates(cfg config) ([]cryptotls.Certificate, error) {
	return benchtls.SelfSignedCertificates(tlsServerName(), certificateAlgorithms(cfg.CipherSuiteIDs)...)
}

func certificateAlgorithms(suites []uint16) []benchtls.CertificateKeyAlgorithm {
	if len(suites) == 0 {
		return []benchtls.CertificateKeyAlgorithm{benchtls.CertificateKeyECDSA}
	}
	algorithms := make([]benchtls.CertificateKeyAlgorithm, 0, 2)
	add := func(algorithm benchtls.CertificateKeyAlgorithm) {
		for _, existing := range algorithms {
			if existing == algorithm {
				return
			}
		}
		algorithms = append(algorithms, algorithm)
	}
	for _, suite := range suites {
		info, err := handlertls.LookupCipherSuiteID(suite, handlertls.CipherSuiteOptions{IncludeInsecure: true})
		if err != nil {
			add(benchtls.CertificateKeyECDSA)
			add(benchtls.CertificateKeyRSA)
			continue
		}
		switch info.CertificateAuth {
		case handlertls.CipherSuiteCertificateRSA:
			add(benchtls.CertificateKeyRSA)
		case handlertls.CipherSuiteCertificateECDSA:
			add(benchtls.CertificateKeyECDSA)
		default:
			add(benchtls.CertificateKeyECDSA)
			add(benchtls.CertificateKeyRSA)
		}
	}
	return algorithms
}
