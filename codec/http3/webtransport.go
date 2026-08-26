package http3

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// SettingEnableConnectProtocol 是 HTTP/3 extended CONNECT 的 SETTINGS 开关。
	SettingEnableConnectProtocol uint64 = 0x08
	// SettingH3Datagram 是 HTTP Datagram over HTTP/3 的 SETTINGS 开关。
	SettingH3Datagram uint64 = 0x33
	// SettingWTInitialMaxData 是 WebTransport session 初始数据窗口配置。
	SettingWTInitialMaxData uint64 = 0x2b61
	// SettingWTInitialMaxStreamsUni 是 WebTransport 单向 stream 初始额度配置。
	SettingWTInitialMaxStreamsUni uint64 = 0x2b64
	// SettingWTInitialMaxStreamsBidi 是 WebTransport 双向 stream 初始额度配置。
	SettingWTInitialMaxStreamsBidi uint64 = 0x2b65
	// SettingWTEnabled 是 WebTransport over HTTP/3 的 SETTINGS 开关。
	SettingWTEnabled uint64 = 0x2c7cf000
)

const (
	// WebTransportProtocolH3 是 WebTransport over HTTP/3 extended CONNECT 的协议令牌。
	WebTransportProtocolH3 = "webtransport-h3"
	// WebTransportProtocolLegacy 是旧草案和部分实现仍可能发送的协议令牌。
	WebTransportProtocolLegacy = "webtransport"
)

const (
	headerMethod    = ":method"
	headerProtocol  = ":protocol"
	headerScheme    = ":scheme"
	headerAuthority = ":authority"
	headerPath      = ":path"
	headerStatus    = ":status"
	headerOrigin    = "origin"
)

// WebTransportRole 描述本端在 WebTransport 建连中的 HTTP/3 角色。
type WebTransportRole uint8

const (
	// WebTransportRoleClient 表示本端是发起 extended CONNECT 的客户端。
	WebTransportRoleClient WebTransportRole = iota + 1
	// WebTransportRoleServer 表示本端是接受 extended CONNECT 的服务端。
	WebTransportRoleServer
)

// WebTransportSettings 描述 WebTransport SETTINGS 中可调的初始额度。
type WebTransportSettings struct {
	// InitialMaxData 是 WebTransport session 初始数据窗口，0 表示不主动发送该设置。
	InitialMaxData uint64
	// InitialMaxStreamsBidi 是本端允许对端打开的双向 stream 初始数量，0 表示不主动发送该设置。
	InitialMaxStreamsBidi uint64
	// InitialMaxStreamsUni 是本端允许对端打开的单向 stream 初始数量，0 表示不主动发送该设置。
	InitialMaxStreamsUni uint64
}

// RequiredWebTransportSettings 返回指定角色应放入 HTTP/3 control stream 的 SETTINGS。
func RequiredWebTransportSettings(role WebTransportRole, cfg WebTransportSettings) []Setting {
	settings := []Setting{{ID: SettingWTEnabled, Value: 1}}
	if role == WebTransportRoleServer {
		settings = append(settings, Setting{ID: SettingEnableConnectProtocol, Value: 1})
	}
	settings = append(settings, Setting{ID: SettingH3Datagram, Value: 1})
	if cfg.InitialMaxData > 0 {
		settings = append(settings, Setting{ID: SettingWTInitialMaxData, Value: cfg.InitialMaxData})
	}
	if cfg.InitialMaxStreamsBidi > 0 {
		settings = append(settings, Setting{ID: SettingWTInitialMaxStreamsBidi, Value: cfg.InitialMaxStreamsBidi})
	}
	if cfg.InitialMaxStreamsUni > 0 {
		settings = append(settings, Setting{ID: SettingWTInitialMaxStreamsUni, Value: cfg.InitialMaxStreamsUni})
	}
	return settings
}

// ValidateWebTransportPeerSettings 校验 peer SETTINGS 是否满足本端 WebTransport 角色需求。
func ValidateWebTransportPeerSettings(role WebTransportRole, settings []Setting) error {
	values := make(map[uint64]uint64, len(settings))
	for _, setting := range settings {
		if _, exists := values[setting.ID]; exists {
			return fmt.Errorf("%w: duplicate setting %x", ErrInvalidWebTransportSetting, setting.ID)
		}
		values[setting.ID] = setting.Value
	}
	required := []uint64{SettingWTEnabled, SettingH3Datagram}
	if role == WebTransportRoleClient {
		required = append(required, SettingEnableConnectProtocol)
	}
	for _, id := range required {
		value, ok := values[id]
		if !ok {
			return fmt.Errorf("%w: %x", ErrMissingWebTransportSetting, id)
		}
		if value != 1 {
			return fmt.Errorf("%w: %x=%d", ErrInvalidWebTransportSetting, id, value)
		}
	}
	return nil
}

// WebTransportConnectRequest 描述 WebTransport extended CONNECT 请求头。
type WebTransportConnectRequest struct {
	// Scheme 是目标 URI scheme，生产路径通常是 https。
	Scheme string
	// Authority 是 HTTP/3 :authority。
	Authority string
	// Path 是 WebTransport endpoint path。
	Path string
	// Origin 是浏览器安全模型使用的 Origin，空值表示不发送。
	Origin string
	// Headers 是额外非伪头，调用方可放入鉴权、追踪或子协议信息。
	Headers []HeaderField
}

// NewWebTransportConnectRequest 构造 WebTransport extended CONNECT HEADERS。
func NewWebTransportConnectRequest(req WebTransportConnectRequest) (HeadersBlock, error) {
	if req.Scheme == "" || req.Authority == "" || req.Path == "" {
		return HeadersBlock{}, ErrInvalidWebTransportConnect
	}
	fields := []HeaderField{
		{Name: headerMethod, Value: "CONNECT"},
		{Name: headerProtocol, Value: WebTransportProtocolH3},
		{Name: headerScheme, Value: req.Scheme},
		{Name: headerAuthority, Value: req.Authority},
		{Name: headerPath, Value: req.Path},
	}
	if req.Origin != "" {
		fields = append(fields, HeaderField{Name: headerOrigin, Value: req.Origin})
	}
	for _, field := range req.Headers {
		if strings.HasPrefix(field.Name, ":") {
			return HeadersBlock{}, ErrInvalidWebTransportConnect
		}
		fields = append(fields, field)
	}
	return HeadersBlock{Fields: fields}, nil
}

// ParseWebTransportConnectRequest 从 HEADERS 解析 WebTransport extended CONNECT 请求。
func ParseWebTransportConnectRequest(block HeadersBlock) (WebTransportConnectRequest, error) {
	var req WebTransportConnectRequest
	var method string
	var protocol string
	for _, field := range block.Fields {
		name := strings.ToLower(field.Name)
		switch name {
		case headerMethod:
			method = field.Value
		case headerProtocol:
			protocol = field.Value
		case headerScheme:
			req.Scheme = field.Value
		case headerAuthority:
			req.Authority = field.Value
		case headerPath:
			req.Path = field.Value
		case headerOrigin:
			req.Origin = field.Value
		default:
			if strings.HasPrefix(name, ":") {
				return WebTransportConnectRequest{}, ErrInvalidWebTransportConnect
			}
			req.Headers = append(req.Headers, HeaderField{Name: field.Name, Value: field.Value})
		}
	}
	if method != "CONNECT" || !isWebTransportProtocol(protocol) || req.Scheme == "" || req.Authority == "" || req.Path == "" {
		return WebTransportConnectRequest{}, ErrInvalidWebTransportConnect
	}
	return req, nil
}

// NewWebTransportConnectResponse 构造 WebTransport extended CONNECT 响应 HEADERS。
func NewWebTransportConnectResponse(status int, headers []HeaderField) HeadersBlock {
	fields := []HeaderField{{Name: headerStatus, Value: strconv.Itoa(status)}}
	for _, field := range headers {
		if strings.HasPrefix(field.Name, ":") {
			continue
		}
		fields = append(fields, field)
	}
	return HeadersBlock{Fields: fields}
}

// IsWebTransportConnectSuccess 判断响应是否为 2xx 成功状态。
func IsWebTransportConnectSuccess(block HeadersBlock) bool {
	for _, field := range block.Fields {
		if strings.ToLower(field.Name) != headerStatus {
			continue
		}
		status, err := strconv.Atoi(field.Value)
		return err == nil && status >= 200 && status < 300
	}
	return false
}

func isWebTransportProtocol(protocol string) bool {
	return protocol == WebTransportProtocolH3 || protocol == WebTransportProtocolLegacy
}
