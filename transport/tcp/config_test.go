package tcp

import (
	"testing"

	"goark.dev/gnalloy/transport"
)

func TestNormalizeConfigCarriesWriteBufferWatermark(t *testing.T) {
	cfg := normalizeConfig(Config{WriteBufferWatermark: transport.WriteBufferWatermark{Low: 3, High: 9}})
	if cfg.WriteBufferWatermark.Low != 3 || cfg.WriteBufferWatermark.High != 9 {
		t.Fatalf("watermark=%+v", cfg.WriteBufferWatermark)
	}

	cfg = normalizeConfig(Config{WriteBufferWatermark: transport.WriteBufferWatermark{Low: 10, High: 9}})
	if cfg.WriteBufferWatermark.Low != 4 || cfg.WriteBufferWatermark.High != 9 {
		t.Fatalf("normalized watermark=%+v", cfg.WriteBufferWatermark)
	}
}
