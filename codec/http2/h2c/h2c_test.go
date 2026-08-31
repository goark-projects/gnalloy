package h2c

import (
	"testing"

	"goark.dev/gnalloy/codec/http1"
	"goark.dev/gnalloy/codec/http2"
)

func TestEncodeDecodeHTTP2SettingsHeader(t *testing.T) {
	settings := []http2.Setting{
		{ID: 1, Value: 4096},
		{ID: 4, Value: 65535},
	}

	header := EncodeHTTP2Settings(settings)
	decoded, err := DecodeHTTP2Settings(header)
	if err != nil {
		t.Fatal(err)
	}
	if header != "AAEAABAAAAQAAP__" {
		t.Fatalf("header=%q", header)
	}
	if len(decoded) != len(settings) || decoded[0] != settings[0] || decoded[1] != settings[1] {
		t.Fatalf("decoded=%+v, want %+v", decoded, settings)
	}
}

func TestApplyUpgradeHeadersAddsH2CHeaders(t *testing.T) {
	req, err := ApplyUpgradeHeaders(http1.Request{
		Method:  "GET",
		URI:     "/h2c",
		Version: "HTTP/1.1",
		Headers: http1.Headers{
			"Host":       "example.test",
			"Connection": "keep-alive",
		},
	}, []http2.Setting{{ID: 4, Value: 65535}})
	if err != nil {
		t.Fatal(err)
	}

	if !http1.IsUpgradeRequest(req, ProtocolName) {
		t.Fatalf("request is not h2c upgrade: %+v", req)
	}
	if !req.Headers.ContainsToken("Connection", HTTP2SettingsHeader) {
		t.Fatalf("connection=%q", req.Headers.Get("Connection"))
	}
	if req.Headers.Get(HTTP2SettingsHeader) == "" {
		t.Fatalf("missing %s", HTTP2SettingsHeader)
	}
}
