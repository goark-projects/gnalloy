package quic

import (
	"crypto/tls"
	"fmt"
	"time"

	nativequic "github.com/quic-go/quic-go"
)

const (
	// DefaultALPN 是未显式指定应用协议时使用的 Gnalloy QUIC ALPN。
	DefaultALPN = "gnalloy-quic"
	// MinInitialPacketSize 是 RFC 9000 要求 Initial 包承载的最小 UDP payload。
	MinInitialPacketSize = 1200

	defaultHandshakeIdleTimeout = 5 * time.Second
	defaultMaxIdleTimeout       = 30 * time.Second
	defaultStreamWindow         = 512 * 1024
	defaultMaxStreamWindow      = 6 * 1024 * 1024
	defaultConnWindow           = 512 * 1024
	defaultMaxConnWindow        = 15 * 1024 * 1024
	defaultIncomingStreams      = 100
)

const (
	// DefaultClientTokenStoreMaxOrigins 是 0-RTT 地址验证 token store 的默认 origin 数。
	DefaultClientTokenStoreMaxOrigins = 64
	// DefaultClientTokenStoreTokensPerOrigin 是每个 origin 保留的默认 token 数。
	DefaultClientTokenStoreTokensPerOrigin = 4
)

// Config 描述 RFC 9000 QUIC 连接栈的公共配置。
type Config struct {
	// TLS 是 QUIC 必需的 TLS 1.3 配置；NormalizeConfig 会克隆该配置后再补齐 ALPN。
	TLS *tls.Config
	// NextProtos 是 QUIC TLS ALPN 列表；为空时优先使用 TLS.NextProtos，再退回 DefaultALPN。
	NextProtos []string
	// Versions 是可协商 QUIC 版本；当前生产适配层只启用 RFC 9000 QUIC v1。
	Versions []Version
	// HandshakeIdleTimeout 是握手阶段无网络活动的超时；0 使用默认 5 秒。
	HandshakeIdleTimeout time.Duration
	// MaxIdleTimeout 是握手完成后的最大空闲超时；0 使用默认 30 秒。
	MaxIdleTimeout time.Duration
	// InitialStreamReceiveWindow 是单 stream 初始接收窗口；0 使用默认 512 KiB。
	InitialStreamReceiveWindow uint64
	// MaxStreamReceiveWindow 是单 stream 自动调优后的最大接收窗口；0 使用默认 6 MiB。
	MaxStreamReceiveWindow uint64
	// InitialConnectionReceiveWindow 是连接级初始接收窗口；0 使用默认 512 KiB。
	InitialConnectionReceiveWindow uint64
	// MaxConnectionReceiveWindow 是连接级自动调优后的最大接收窗口；0 使用默认 15 MiB。
	MaxConnectionReceiveWindow uint64
	// MaxIncomingStreams 是对端可打开的最大双向 stream 数；0 使用默认 100，负数表示禁止。
	MaxIncomingStreams int64
	// MaxIncomingUniStreams 是对端可打开的最大单向 stream 数；0 使用默认 100，负数表示禁止。
	MaxIncomingUniStreams int64
	// KeepAlivePeriod 控制 keepalive 周期；0 表示关闭 keepalive。
	KeepAlivePeriod time.Duration
	// InitialPacketSize 控制初始 QUIC 包大小；0 使用 RFC 最小值 1200。
	InitialPacketSize uint16
	// DisablePathMTUDiscovery 禁用路径 MTU 探测，适合明确知道链路 MTU 的受控环境。
	DisablePathMTUDiscovery bool
	// EnableDatagrams 启用 RFC 9221 QUIC datagram 能力。
	EnableDatagrams bool
	// EnableStreamResetPartialDelivery 启用带部分交付语义的 stream reset 扩展。
	EnableStreamResetPartialDelivery bool
	// Enable0RTT 启用 QUIC 0-RTT 能力；客户端仍必须提供可复用的 TLS ClientSessionCache。
	Enable0RTT bool
	// ClientTokenStore 保存服务端下发的 NEW_TOKEN；复用同一个 store 可跳过后续地址验证。
	ClientTokenStore ClientTokenStore
	// ClientTokenStoreMaxOrigins 控制客户端 NEW_TOKEN LRU store 的 origin 容量；0 在 0-RTT 开启时使用默认值。
	ClientTokenStoreMaxOrigins int
	// ClientTokenStoreTokensPerOrigin 控制每个 origin 保留的 NEW_TOKEN 数；0 在 0-RTT 开启时使用默认值。
	ClientTokenStoreTokensPerOrigin int
	// EnableWebTransport 启用 WebTransport over HTTP/3 所需的 QUIC datagram 和 reset 扩展。
	EnableWebTransport bool
	// QLog 为每条连接打开 qlog trace；默认关闭，避免热路径额外开销。
	QLog QLogConfig
}

type normalizedConfig struct {
	public Config
	tls    *tls.Config
	quic   *nativequic.Config
}

// DefaultConfig 返回 RFC9000 适配层的安全默认配置。
func DefaultConfig() Config {
	return Config{
		NextProtos:                       []string{DefaultALPN},
		Versions:                         []Version{Version1},
		HandshakeIdleTimeout:             defaultHandshakeIdleTimeout,
		MaxIdleTimeout:                   defaultMaxIdleTimeout,
		InitialStreamReceiveWindow:       defaultStreamWindow,
		MaxStreamReceiveWindow:           defaultMaxStreamWindow,
		InitialConnectionReceiveWindow:   defaultConnWindow,
		MaxConnectionReceiveWindow:       defaultMaxConnWindow,
		MaxIncomingStreams:               defaultIncomingStreams,
		MaxIncomingUniStreams:            defaultIncomingStreams,
		InitialPacketSize:                MinInitialPacketSize,
		DisablePathMTUDiscovery:          false,
		EnableDatagrams:                  false,
		EnableStreamResetPartialDelivery: false,
		Enable0RTT:                       false,
		EnableWebTransport:               false,
	}
}

// NormalizeConfig 克隆 TLS 配置并补齐 QUIC v1、TLS 1.3、ALPN 和流控默认值。
func NormalizeConfig(cfg Config) (Config, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return Config{}, err
	}
	return normalized.public, nil
}

func normalizeConfig(cfg Config) (normalizedConfig, error) {
	tlsCfg, nextProtos, err := normalizeTLSConfig(cfg)
	if err != nil {
		return normalizedConfig{}, err
	}
	versions, nativeVersions, err := normalizeVersions(cfg.Versions)
	if err != nil {
		return normalizedConfig{}, err
	}

	out := cfg
	out.TLS = tlsCfg
	out.NextProtos = nextProtos
	out.Versions = versions
	if out.HandshakeIdleTimeout, err = normalizeDuration(out.HandshakeIdleTimeout, defaultHandshakeIdleTimeout, "handshake idle timeout"); err != nil {
		return normalizedConfig{}, err
	}
	if out.MaxIdleTimeout, err = normalizeDuration(out.MaxIdleTimeout, defaultMaxIdleTimeout, "max idle timeout"); err != nil {
		return normalizedConfig{}, err
	}
	if out.KeepAlivePeriod < 0 {
		return normalizedConfig{}, fmt.Errorf("%w: negative keepalive period", ErrInvalidConfig)
	}
	if out.InitialStreamReceiveWindow == 0 {
		out.InitialStreamReceiveWindow = defaultStreamWindow
	}
	if out.MaxStreamReceiveWindow == 0 {
		out.MaxStreamReceiveWindow = defaultMaxStreamWindow
	}
	if out.InitialConnectionReceiveWindow == 0 {
		out.InitialConnectionReceiveWindow = defaultConnWindow
	}
	if out.MaxConnectionReceiveWindow == 0 {
		out.MaxConnectionReceiveWindow = defaultMaxConnWindow
	}
	if out.InitialStreamReceiveWindow > out.MaxStreamReceiveWindow {
		return normalizedConfig{}, fmt.Errorf("%w: stream receive window exceeds max", ErrInvalidConfig)
	}
	if out.InitialConnectionReceiveWindow > out.MaxConnectionReceiveWindow {
		return normalizedConfig{}, fmt.Errorf("%w: connection receive window exceeds max", ErrInvalidConfig)
	}
	if out.MaxIncomingStreams == 0 {
		out.MaxIncomingStreams = defaultIncomingStreams
	}
	if out.MaxIncomingUniStreams == 0 {
		out.MaxIncomingUniStreams = defaultIncomingStreams
	}
	if out.InitialPacketSize == 0 {
		out.InitialPacketSize = MinInitialPacketSize
	}
	if out.InitialPacketSize < MinInitialPacketSize {
		return normalizedConfig{}, fmt.Errorf("%w: initial packet size below RFC minimum", ErrInvalidConfig)
	}
	if out.EnableWebTransport {
		out.EnableDatagrams = true
		out.EnableStreamResetPartialDelivery = true
	}
	if err := normalizeClientTokenStore(&out); err != nil {
		return normalizedConfig{}, err
	}

	return normalizedConfig{
		public: out,
		tls:    tlsCfg,
		quic:   toNativeConfig(out, nativeVersions),
	}, nil
}

func normalizeClientTokenStore(cfg *Config) error {
	if cfg.ClientTokenStoreMaxOrigins < 0 {
		return fmt.Errorf("%w: negative client token store origins", ErrInvalidConfig)
	}
	if cfg.ClientTokenStoreTokensPerOrigin < 0 {
		return fmt.Errorf("%w: negative client token store tokens", ErrInvalidConfig)
	}
	if cfg.ClientTokenStore != nil {
		return nil
	}
	if cfg.Enable0RTT || cfg.ClientTokenStoreMaxOrigins > 0 || cfg.ClientTokenStoreTokensPerOrigin > 0 {
		if cfg.ClientTokenStoreMaxOrigins == 0 {
			cfg.ClientTokenStoreMaxOrigins = DefaultClientTokenStoreMaxOrigins
		}
		if cfg.ClientTokenStoreTokensPerOrigin == 0 {
			cfg.ClientTokenStoreTokensPerOrigin = DefaultClientTokenStoreTokensPerOrigin
		}
		cfg.ClientTokenStore = nativequic.NewLRUTokenStore(cfg.ClientTokenStoreMaxOrigins, cfg.ClientTokenStoreTokensPerOrigin)
	}
	return nil
}

func normalizeTLSConfig(cfg Config) (*tls.Config, []string, error) {
	if cfg.TLS == nil {
		return nil, nil, ErrMissingTLSConfig
	}
	tlsCfg := cfg.TLS.Clone()
	if tlsCfg.MaxVersion != 0 && tlsCfg.MaxVersion < tls.VersionTLS13 {
		return nil, nil, fmt.Errorf("%w: max version below TLS 1.3", ErrInvalidTLSConfig)
	}
	if tlsCfg.MinVersion == 0 || tlsCfg.MinVersion < tls.VersionTLS13 {
		tlsCfg.MinVersion = tls.VersionTLS13
	}
	nextProtos, err := normalizeNextProtos(cfg.NextProtos, tlsCfg.NextProtos)
	if err != nil {
		return nil, nil, err
	}
	tlsCfg.NextProtos = nextProtos
	return tlsCfg, nextProtos, nil
}

func normalizeNextProtos(cfgProtos []string, tlsProtos []string) ([]string, error) {
	protos := cfgProtos
	if len(protos) == 0 {
		protos = tlsProtos
	}
	if len(protos) == 0 {
		protos = []string{DefaultALPN}
	}
	out := make([]string, len(protos))
	for i, proto := range protos {
		if proto == "" {
			return nil, fmt.Errorf("%w: empty alpn", ErrInvalidTLSConfig)
		}
		out[i] = proto
	}
	return out, nil
}

func normalizeVersions(versions []Version) ([]Version, []nativequic.Version, error) {
	if len(versions) == 0 {
		versions = []Version{Version1}
	}
	out := make([]Version, len(versions))
	nativeVersions := make([]nativequic.Version, len(versions))
	for i, version := range versions {
		if version != Version1 {
			return nil, nil, fmt.Errorf("%w: %s", ErrInvalidVersion, version)
		}
		out[i] = version
		nativeVersions[i] = nativequic.Version1
	}
	return out, nativeVersions, nil
}

func normalizeDuration(value time.Duration, def time.Duration, name string) (time.Duration, error) {
	if value < 0 {
		return 0, fmt.Errorf("%w: negative %s", ErrInvalidConfig, name)
	}
	if value == 0 {
		return def, nil
	}
	return value, nil
}

func toNativeConfig(cfg Config, versions []nativequic.Version) *nativequic.Config {
	return &nativequic.Config{
		Versions:                         versions,
		HandshakeIdleTimeout:             cfg.HandshakeIdleTimeout,
		MaxIdleTimeout:                   cfg.MaxIdleTimeout,
		InitialStreamReceiveWindow:       cfg.InitialStreamReceiveWindow,
		MaxStreamReceiveWindow:           cfg.MaxStreamReceiveWindow,
		InitialConnectionReceiveWindow:   cfg.InitialConnectionReceiveWindow,
		MaxConnectionReceiveWindow:       cfg.MaxConnectionReceiveWindow,
		MaxIncomingStreams:               cfg.MaxIncomingStreams,
		MaxIncomingUniStreams:            cfg.MaxIncomingUniStreams,
		KeepAlivePeriod:                  cfg.KeepAlivePeriod,
		InitialPacketSize:                cfg.InitialPacketSize,
		DisablePathMTUDiscovery:          cfg.DisablePathMTUDiscovery,
		EnableDatagrams:                  cfg.EnableDatagrams,
		EnableStreamResetPartialDelivery: cfg.EnableStreamResetPartialDelivery,
		Allow0RTT:                        cfg.Enable0RTT,
		TokenStore:                       cfg.ClientTokenStore,
		Tracer:                           newNativeTracer(cfg.QLog),
	}
}
