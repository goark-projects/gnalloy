package rxtx

import "goark.dev/gnalloy/transport"

const defaultReadBufferSize = 4096

type Parity uint8

const (
	ParityNone Parity = iota
	ParityOdd
	ParityEven
	ParityMark
	ParitySpace
)

type StopBits uint8

const (
	StopBitsOne StopBits = iota + 1
	StopBitsOnePointFive
	StopBitsTwo
)

type Config struct {
	PortName             string
	BaudRate             int
	DataBits             int
	StopBits             StopBits
	Parity               Parity
	ReadBufferSize       int
	WriteBufferWatermark transport.WriteBufferWatermark
	Driver               Driver
}

func DefaultConfig() Config {
	return Config{
		BaudRate:             9600,
		DataBits:             8,
		StopBits:             StopBitsOne,
		Parity:               ParityNone,
		ReadBufferSize:       defaultReadBufferSize,
		WriteBufferWatermark: transport.DefaultWriteBufferWatermark(),
	}
}

func normalizeConfig(cfg Config, address string) Config {
	if cfg.PortName == "" {
		cfg.PortName = address
	}
	if cfg.BaudRate <= 0 {
		cfg.BaudRate = 9600
	}
	if cfg.DataBits < 5 || cfg.DataBits > 8 {
		cfg.DataBits = 8
	}
	if cfg.StopBits == 0 {
		cfg.StopBits = StopBitsOne
	}
	if cfg.Parity > ParitySpace {
		cfg.Parity = ParityNone
	}
	if cfg.ReadBufferSize <= 0 {
		cfg.ReadBufferSize = defaultReadBufferSize
	}
	cfg.WriteBufferWatermark = transport.NormalizeWriteBufferWatermark(cfg.WriteBufferWatermark)
	return cfg
}
