package http3

import "errors"

var (
	ErrInvalidFrame       = errors.New("http3: invalid frame")
	ErrFrameTooLarge      = errors.New("http3: frame too large")
	ErrInvalidVarInt      = errors.New("http3: invalid varint")
	ErrDuplicateSetting   = errors.New("http3: duplicate setting")
	ErrTooManySettings    = errors.New("http3: too many settings")
	ErrUnsupportedFrame   = errors.New("http3: unsupported frame")
	ErrInvalidFrameOrder  = errors.New("http3: invalid frame order")
	ErrHeaderListTooLarge = errors.New("http3: header list too large")
	ErrInvalidPipeline    = errors.New("http3: invalid pipeline config")
	// ErrMissingWebTransportSetting 表示 peer SETTINGS 缺少 WebTransport 必需项。
	ErrMissingWebTransportSetting = errors.New("http3: missing webtransport setting")
	// ErrInvalidWebTransportSetting 表示 peer SETTINGS 中的 WebTransport 开关值非法。
	ErrInvalidWebTransportSetting = errors.New("http3: invalid webtransport setting")
	// ErrInvalidWebTransportConnect 表示 extended CONNECT 头部不是合法 WebTransport 建连请求。
	ErrInvalidWebTransportConnect = errors.New("http3: invalid webtransport connect request")
)
