package main

import handlertls "goark.dev/gnalloy/handler/tls"

func parseCipherSuiteList(value string, allowInsecure bool) ([]uint16, error) {
	ids, err := handlertls.ParseCipherSuites(value, handlertls.CipherSuiteOptions{IncludeInsecure: allowInsecure})
	if err != nil {
		return nil, err
	}
	if err := handlertls.ValidateConfigurableCipherSuites(ids); err != nil {
		return nil, err
	}
	return ids, nil
}

func cipherSuiteNames(ids []uint16) []string {
	return handlertls.CipherSuiteNames(ids)
}
