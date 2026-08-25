package socks

import (
	"net"
	"strconv"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec"
)

type GreetingDecoder struct {
	*codec.ByteToMessageDecoder
}

func NewGreetingDecoder() *GreetingDecoder {
	d := &GreetingDecoder{}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d
}

func (d *GreetingDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	if in.ReadableBytes() < 2 {
		return nil, nil
	}
	reader := in.ReaderIndex()
	version, _ := in.GetByte(reader)
	if version != Version5 {
		return nil, ErrInvalidFrame
	}
	countByte, _ := in.GetByte(reader + 1)
	total := 2 + int(countByte)
	if in.ReadableBytes() < total {
		return nil, nil
	}
	methods := make([]byte, int(countByte))
	for i := range methods {
		b, _ := in.GetByte(reader + 2 + i)
		methods[i] = b
	}
	if err := in.SkipBytes(total); err != nil {
		return nil, err
	}
	return Greeting{Methods: methods}, nil
}

type GreetingEncoder struct{}

func NewGreetingEncoder() *GreetingEncoder {
	return &GreetingEncoder{}
}

func (e *GreetingEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	greeting, ok := msg.(Greeting)
	if !ok {
		return ctx.Write(msg)
	}
	if len(greeting.Methods) == 0 || len(greeting.Methods) > 255 {
		return ErrInvalidFrame
	}
	out, err := ctx.Channel().Allocator().Acquire(2 + len(greeting.Methods))
	if err != nil {
		return err
	}
	if _, err := out.WriteBytes([]byte{Version5, byte(len(greeting.Methods))}); err != nil {
		out.Release()
		return err
	}
	if _, err := out.WriteBytes(greeting.Methods); err != nil {
		out.Release()
		return err
	}
	return ctx.Write(out)
}

type MethodSelectionDecoder struct {
	*codec.ByteToMessageDecoder
}

func NewMethodSelectionDecoder() *MethodSelectionDecoder {
	d := &MethodSelectionDecoder{}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d
}

func (d *MethodSelectionDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	if in.ReadableBytes() < 2 {
		return nil, nil
	}
	reader := in.ReaderIndex()
	version, _ := in.GetByte(reader)
	method, _ := in.GetByte(reader + 1)
	if version != Version5 {
		return nil, ErrInvalidFrame
	}
	if err := in.SkipBytes(2); err != nil {
		return nil, err
	}
	return MethodSelection{Method: method}, nil
}

type MethodSelectionEncoder struct{}

func NewMethodSelectionEncoder() *MethodSelectionEncoder {
	return &MethodSelectionEncoder{}
}

func (e *MethodSelectionEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	selection, ok := msg.(MethodSelection)
	if !ok {
		return ctx.Write(msg)
	}
	return writeBytes(ctx, []byte{Version5, selection.Method})
}

type CommandDecoder struct {
	*codec.ByteToMessageDecoder
	reply bool
}

func NewCommandRequestDecoder() *CommandDecoder {
	return newCommandDecoder(false)
}

func NewCommandReplyDecoder() *CommandDecoder {
	return newCommandDecoder(true)
}

func newCommandDecoder(reply bool) *CommandDecoder {
	d := &CommandDecoder{reply: reply}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d
}

func (d *CommandDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	if in.ReadableBytes() < 4 {
		return nil, nil
	}
	reader := in.ReaderIndex()
	version, _ := in.GetByte(reader)
	if version != Version5 {
		return nil, ErrInvalidFrame
	}
	cmdOrStatus, _ := in.GetByte(reader + 1)
	reserved, _ := in.GetByte(reader + 2)
	if reserved != 0 {
		return nil, ErrInvalidFrame
	}
	address, total, ok, err := parseAddress(in, reader+3)
	if err != nil || !ok {
		return nil, err
	}
	if err := in.SkipBytes(total + 3); err != nil {
		return nil, err
	}
	if d.reply {
		return CommandReply{Version: Version5, Status: cmdOrStatus, Address: address}, nil
	}
	return CommandRequest{Version: Version5, Command: cmdOrStatus, Address: address}, nil
}

type CommandRequestEncoder struct{}

func NewCommandRequestEncoder() *CommandRequestEncoder {
	return &CommandRequestEncoder{}
}

func (e *CommandRequestEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	req, ok := msg.(CommandRequest)
	if !ok {
		return ctx.Write(msg)
	}
	if req.Version == 0 {
		req.Version = Version5
	}
	if req.Version != Version5 {
		return ErrInvalidFrame
	}
	payload, err := appendAddress([]byte{Version5, req.Command, 0}, req.Address)
	if err != nil {
		return err
	}
	return writeBytes(ctx, payload)
}

type CommandReplyEncoder struct{}

func NewCommandReplyEncoder() *CommandReplyEncoder {
	return &CommandReplyEncoder{}
}

func (e *CommandReplyEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	reply, ok := msg.(CommandReply)
	if !ok {
		return ctx.Write(msg)
	}
	if reply.Version == 0 {
		reply.Version = Version5
	}
	if reply.Version != Version5 {
		return ErrInvalidFrame
	}
	payload, err := appendAddress([]byte{Version5, reply.Status, 0}, reply.Address)
	if err != nil {
		return err
	}
	return writeBytes(ctx, payload)
}

type SOCKS4RequestEncoder struct{}

func NewSOCKS4RequestEncoder() *SOCKS4RequestEncoder {
	return &SOCKS4RequestEncoder{}
}

func (e *SOCKS4RequestEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	req, ok := msg.(SOCKS4Request)
	if !ok {
		return ctx.Write(msg)
	}
	host, port, err := splitHostPort(req.Address)
	if err != nil {
		return err
	}
	out := []byte{Version4, req.Command, byte(port >> 8), byte(port)}
	ip := net.ParseIP(host).To4()
	if ip == nil {
		out = append(out, 0, 0, 0, 1)
	} else {
		out = append(out, ip...)
	}
	out = append(out, req.UserID...)
	out = append(out, 0)
	if ip == nil {
		out = append(out, host...)
		out = append(out, 0)
	}
	return writeBytes(ctx, out)
}

type SOCKS4ReplyDecoder struct {
	*codec.ByteToMessageDecoder
}

func NewSOCKS4ReplyDecoder() *SOCKS4ReplyDecoder {
	d := &SOCKS4ReplyDecoder{}
	d.ByteToMessageDecoder = codec.NewByteToMessageDecoder(d)
	return d
}

func (d *SOCKS4ReplyDecoder) Decode(_ *channel.HandlerContext, in *buffer.CompositeByteBuf) (any, error) {
	if in.ReadableBytes() < 8 {
		return nil, nil
	}
	reader := in.ReaderIndex()
	null, _ := in.GetByte(reader)
	status, _ := in.GetByte(reader + 1)
	if null != 0 {
		return nil, ErrInvalidFrame
	}
	port, err := in.ReadUnsigned(reader+2, 2, buffer.BigEndian)
	if err != nil {
		return nil, err
	}
	var ip [4]byte
	for i := range ip {
		ip[i], _ = in.GetByte(reader + 4 + i)
	}
	if err := in.SkipBytes(8); err != nil {
		return nil, err
	}
	return SOCKS4Reply{Status: status, Address: net.JoinHostPort(net.IP(ip[:]).String(), strconv.Itoa(int(port)))}, nil
}

func parseAddress(in *buffer.CompositeByteBuf, index int) (string, int, bool, error) {
	if in.WriterIndex()-index < 1 {
		return "", 0, false, nil
	}
	addressType, _ := in.GetByte(index)
	switch addressType {
	case AddressIPv4:
		if in.WriterIndex()-index < 1+4+2 {
			return "", 0, false, nil
		}
		var ip [4]byte
		for i := range ip {
			ip[i], _ = in.GetByte(index + 1 + i)
		}
		port, err := in.ReadUnsigned(index+5, 2, buffer.BigEndian)
		if err != nil {
			return "", 0, false, err
		}
		return net.JoinHostPort(net.IP(ip[:]).String(), strconv.Itoa(int(port))), 1 + 4 + 2, true, nil
	case AddressIPv6:
		if in.WriterIndex()-index < 1+16+2 {
			return "", 0, false, nil
		}
		ip := make(net.IP, net.IPv6len)
		for i := range ip {
			ip[i], _ = in.GetByte(index + 1 + i)
		}
		port, err := in.ReadUnsigned(index+17, 2, buffer.BigEndian)
		if err != nil {
			return "", 0, false, err
		}
		return net.JoinHostPort(ip.String(), strconv.Itoa(int(port))), 1 + 16 + 2, true, nil
	case AddressDomain:
		if in.WriterIndex()-index < 2 {
			return "", 0, false, nil
		}
		lengthByte, _ := in.GetByte(index + 1)
		length := int(lengthByte)
		if in.WriterIndex()-index < 2+length+2 {
			return "", 0, false, nil
		}
		name := make([]byte, length)
		for i := range name {
			name[i], _ = in.GetByte(index + 2 + i)
		}
		port, err := in.ReadUnsigned(index+2+length, 2, buffer.BigEndian)
		if err != nil {
			return "", 0, false, err
		}
		return net.JoinHostPort(string(name), strconv.Itoa(int(port))), 2 + length + 2, true, nil
	default:
		return "", 0, false, ErrInvalidAddress
	}
}

func appendAddress(dst []byte, address string) ([]byte, error) {
	host, port, err := splitHostPort(address)
	if err != nil {
		return nil, err
	}
	dstPort := [2]byte{byte(port >> 8), byte(port)}
	if ip := net.ParseIP(host).To4(); ip != nil {
		dst = append(dst, AddressIPv4)
		dst = append(dst, ip...)
		dst = append(dst, dstPort[:]...)
		return dst, nil
	}
	if ip := net.ParseIP(host).To16(); ip != nil {
		dst = append(dst, AddressIPv6)
		dst = append(dst, ip...)
		dst = append(dst, dstPort[:]...)
		return dst, nil
	}
	if len(host) == 0 || len(host) > 255 {
		return nil, ErrInvalidAddress
	}
	dst = append(dst, AddressDomain, byte(len(host)))
	dst = append(dst, host...)
	dst = append(dst, dstPort[:]...)
	return dst, nil
}

func writeBytes(ctx *channel.HandlerContext, data []byte) error {
	out, err := ctx.Channel().Allocator().Acquire(len(data))
	if err != nil {
		return err
	}
	if _, err := out.WriteBytes(data); err != nil {
		out.Release()
		return err
	}
	return ctx.Write(out)
}
