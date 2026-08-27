package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"goark.dev/gnalloy/examples/internal/exampleconfig"
	appprotocol "goark.dev/gnalloy/protocol"
	"goark.dev/gnalloy/transport"
	"goark.dev/gnalloy/transport/l2"
	"goark.dev/gnalloy/transport/raw"
	"goark.dev/gnalloy/transport/tcp"
	"goark.dev/gnalloy/transport/udp"
)

type transportKind string

const (
	transportTCP transportKind = "tcp"
	transportUDP transportKind = "udp"
	transportRaw transportKind = "raw"
	transportL2  transportKind = "l2"
)

var (
	errInvalidTransport = errors.New("gnalloy/examples/protocol-exchange: invalid transport")
	errInvalidPayload   = errors.New("gnalloy/examples/protocol-exchange: invalid payload")
	errInvalidRawFamily = errors.New("gnalloy/examples/protocol-exchange: invalid raw family")
	errInvalidEtherType = errors.New("gnalloy/examples/protocol-exchange: invalid ether type")
)

type protocolConfig struct {
	kind              transportKind
	rawProtocol       int
	rawFamilyText     string
	rawHeaderIncluded bool
	l2EtherTypeText   string
	l2Promiscuous     bool

	rawFamily  raw.Family
	l2EtherTyp uint16
}

func (c protocolConfig) resolve() (protocolConfig, error) {
	if c.kind == transportRaw {
		family, err := parseRawFamily(c.rawFamilyText)
		if err != nil {
			return protocolConfig{}, err
		}
		if c.rawProtocol <= 0 || c.rawProtocol > 255 {
			return protocolConfig{}, fmt.Errorf("%w: raw protocol must be 1..255", errInvalidTransport)
		}
		c.rawFamily = family
	}
	if c.kind == transportL2 {
		etherType, err := parseEtherType(c.l2EtherTypeText)
		if err != nil {
			return protocolConfig{}, err
		}
		c.l2EtherTyp = etherType
	}
	return c, nil
}

func parseTransportKind(text string) (transportKind, error) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "tcp", "stream":
		return transportTCP, nil
	case "udp", "datagram":
		return transportUDP, nil
	case "raw", "packet":
		return transportRaw, nil
	case "l2", "frame":
		return transportL2, nil
	default:
		return "", fmt.Errorf("%w: %s", errInvalidTransport, text)
	}
}

func parseRawFamily(text string) (raw.Family, error) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "ipv4", "ip4", "4":
		return raw.FamilyIPv4, nil
	case "ipv6", "ip6", "6":
		return raw.FamilyIPv6, nil
	default:
		return 0, fmt.Errorf("%w: %s", errInvalidRawFamily, text)
	}
}

func parseEtherType(text string) (uint16, error) {
	value, err := strconv.ParseUint(strings.TrimSpace(text), 0, 16)
	if err != nil {
		return 0, fmt.Errorf("%w: %s", errInvalidEtherType, text)
	}
	return uint16(value), nil
}

func requestPayload(message string, payloadHex string) ([]byte, error) {
	payloadHex = strings.TrimSpace(payloadHex)
	if payloadHex != "" {
		payload, err := hex.DecodeString(payloadHex)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errInvalidPayload, err)
		}
		if len(payload) == 0 {
			return nil, errInvalidPayload
		}
		return payload, nil
	}
	if message == "" {
		return nil, errInvalidPayload
	}
	return []byte(message), nil
}

func newExchanger(opts *exampleconfig.Options, group *transport.EventLoopGroup, cfg protocolConfig) (appprotocol.ChannelExchanger, error) {
	switch cfg.kind {
	case transportTCP:
		tcpConfig, err := opts.TCPConfig()
		if err != nil {
			return appprotocol.ChannelExchanger{}, err
		}
		return appprotocol.Stream(group, tcp.NewTransport(tcpConfig)), nil
	case transportUDP:
		udpConfig, err := opts.UDPConfig()
		if err != nil {
			return appprotocol.ChannelExchanger{}, err
		}
		return appprotocol.Datagram(group, udp.NewTransport(udpConfig)), nil
	case transportRaw:
		rawConfig := raw.DefaultConfig()
		rawConfig.Protocol = cfg.rawProtocol
		rawConfig.Family = cfg.rawFamily
		rawConfig.HeaderIncluded = cfg.rawHeaderIncluded
		rawConfig.ReadBufferSize = opts.ReadBufferSize
		rawConfig.WriteBufferWatermark = opts.WriteBufferWatermark()
		return appprotocol.Packet(group, raw.NewTransport(rawConfig)), nil
	case transportL2:
		l2Config := l2.DefaultConfig()
		l2Config.EtherType = cfg.l2EtherTyp
		l2Config.Promiscuous = cfg.l2Promiscuous
		l2Config.ReadBufferSize = opts.ReadBufferSize
		l2Config.WriteBufferWatermark = opts.WriteBufferWatermark()
		return appprotocol.Frame(group, l2.NewTransport(l2Config)), nil
	default:
		return appprotocol.ChannelExchanger{}, fmt.Errorf("%w: %s", errInvalidTransport, cfg.kind)
	}
}

func runtimeBoundaryError(kind transportKind, err error) error {
	switch kind {
	case transportRaw:
		return fmt.Errorf("%w; raw socket usually requires Administrator, root, or CAP_NET_RAW", err)
	case transportL2:
		return fmt.Errorf("%w; L2 access requires a supported driver and packet-capture privileges", err)
	default:
		return err
	}
}
