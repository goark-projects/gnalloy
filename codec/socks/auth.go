package socks

import (
	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

// UsernamePasswordAuthRequestDecoder 解码 RFC1929 用户名密码认证请求。
type UsernamePasswordAuthRequestDecoder struct {
	*codec.ByteToMessageDecoder
}

func NewUsernamePasswordAuthRequestDecoder() *UsernamePasswordAuthRequestDecoder {
	d := &UsernamePasswordAuthRequestDecoder{}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d
}

func (d *UsernamePasswordAuthRequestDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	if in.ReadableBytes() < 2 {
		return nil, nil
	}
	reader := in.ReaderIndex()
	version, _ := in.GetByte(reader)
	if version != AuthVersionUserPassword {
		return nil, ErrInvalidFrame
	}
	usernameLenByte, _ := in.GetByte(reader + 1)
	usernameLen := int(usernameLenByte)
	if usernameLen == 0 {
		return nil, ErrInvalidFrame
	}
	if in.ReadableBytes() < 2+usernameLen+1 {
		return nil, nil
	}
	passwordLenIndex := reader + 2 + usernameLen
	passwordLenByte, _ := in.GetByte(passwordLenIndex)
	passwordLen := int(passwordLenByte)
	if passwordLen == 0 {
		return nil, ErrInvalidFrame
	}
	total := 2 + usernameLen + 1 + passwordLen
	if in.ReadableBytes() < total {
		return nil, nil
	}
	username := make([]byte, usernameLen)
	for i := range username {
		username[i], _ = in.GetByte(reader + 2 + i)
	}
	password := make([]byte, passwordLen)
	for i := range password {
		password[i], _ = in.GetByte(passwordLenIndex + 1 + i)
	}
	if err := in.SkipBytes(total); err != nil {
		return nil, err
	}
	return UsernamePasswordAuthRequest{Username: string(username), Password: string(password)}, nil
}

// UsernamePasswordAuthRequestEncoder 编码 RFC1929 用户名密码认证请求。
type UsernamePasswordAuthRequestEncoder struct{}

func NewUsernamePasswordAuthRequestEncoder() *UsernamePasswordAuthRequestEncoder {
	return &UsernamePasswordAuthRequestEncoder{}
}

func (e *UsernamePasswordAuthRequestEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	req, ok := msg.(UsernamePasswordAuthRequest)
	if !ok {
		return ctx.Write(msg)
	}
	if len(req.Username) == 0 || len(req.Username) > 255 || len(req.Password) == 0 || len(req.Password) > 255 {
		return ErrInvalidFrame
	}
	payload := make([]byte, 0, 3+len(req.Username)+len(req.Password))
	payload = append(payload, AuthVersionUserPassword, byte(len(req.Username)))
	payload = append(payload, req.Username...)
	payload = append(payload, byte(len(req.Password)))
	payload = append(payload, req.Password...)
	return writeBytes(ctx, payload)
}

// UsernamePasswordAuthResponseDecoder 解码 RFC1929 用户名密码认证响应。
type UsernamePasswordAuthResponseDecoder struct {
	*codec.ByteToMessageDecoder
}

func NewUsernamePasswordAuthResponseDecoder() *UsernamePasswordAuthResponseDecoder {
	d := &UsernamePasswordAuthResponseDecoder{}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d
}

func (d *UsernamePasswordAuthResponseDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	status, ok, err := readAuthStatus(in)
	if err != nil || !ok {
		return nil, err
	}
	return UsernamePasswordAuthResponse{Status: status}, nil
}

// UsernamePasswordAuthResponseEncoder 编码 RFC1929 用户名密码认证响应。
type UsernamePasswordAuthResponseEncoder struct{}

func NewUsernamePasswordAuthResponseEncoder() *UsernamePasswordAuthResponseEncoder {
	return &UsernamePasswordAuthResponseEncoder{}
}

func (e *UsernamePasswordAuthResponseEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	resp, ok := msg.(UsernamePasswordAuthResponse)
	if !ok {
		return ctx.Write(msg)
	}
	return writeBytes(ctx, []byte{AuthVersionUserPassword, resp.Status})
}

// PrivateAuthResponseDecoder 解码 Netty 风格私有认证响应状态。
type PrivateAuthResponseDecoder struct {
	*codec.ByteToMessageDecoder
}

func NewPrivateAuthResponseDecoder() *PrivateAuthResponseDecoder {
	d := &PrivateAuthResponseDecoder{}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d
}

func (d *PrivateAuthResponseDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	status, ok, err := readAuthStatus(in)
	if err != nil || !ok {
		return nil, err
	}
	return PrivateAuthResponse{Status: status}, nil
}

// PrivateAuthResponseEncoder 编码 Netty 风格私有认证响应状态。
type PrivateAuthResponseEncoder struct{}

func NewPrivateAuthResponseEncoder() *PrivateAuthResponseEncoder {
	return &PrivateAuthResponseEncoder{}
}

func (e *PrivateAuthResponseEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	resp, ok := msg.(PrivateAuthResponse)
	if !ok {
		return ctx.Write(msg)
	}
	return writeBytes(ctx, []byte{AuthVersionUserPassword, resp.Status})
}

func readAuthStatus(in *buffer.CompositeByteBuf) (byte, bool, error) {
	if in.ReadableBytes() < 2 {
		return 0, false, nil
	}
	reader := in.ReaderIndex()
	version, _ := in.GetByte(reader)
	if version != AuthVersionUserPassword {
		return 0, false, ErrInvalidFrame
	}
	status, _ := in.GetByte(reader + 1)
	if err := in.SkipBytes(2); err != nil {
		return 0, false, err
	}
	return status, true, nil
}
