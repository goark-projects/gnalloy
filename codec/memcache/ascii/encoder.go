package ascii

import (
	"strconv"
	"strings"

	"goark.dev/gnalloy/channel"
)

// RequestEncoder 编码 Memcached ASCII request。
type RequestEncoder struct{}

// NewRequestEncoder 创建 request encoder。
func NewRequestEncoder() *RequestEncoder {
	return &RequestEncoder{}
}

// Write 编码 request，其它消息透传。
func (e *RequestEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	req, ok := msg.(Request)
	if !ok {
		return ctx.Write(msg)
	}
	return e.writeRequest(ctx, req)
}

func (e *RequestEncoder) writeRequest(ctx *channel.HandlerContext, req Request) error {
	var builder strings.Builder
	switch req.Command {
	case CommandGet, CommandGets:
		builder.WriteString(string(req.Command))
		for _, key := range req.Keys {
			builder.WriteByte(' ')
			builder.WriteString(key)
		}
		builder.WriteString("\r\n")
		return writeASCIIString(ctx, builder.String())
	case CommandSet, CommandAdd, CommandReplace, CommandAppend, CommandPrepend:
		writeStorageLine(&builder, req, false)
		return writeRequestWithValue(ctx, req, builder.String())
	case CommandCAS:
		writeStorageLine(&builder, req, true)
		return writeRequestWithValue(ctx, req, builder.String())
	default:
		builder.WriteString(string(req.Command))
		for _, arg := range req.Arguments {
			builder.WriteByte(' ')
			builder.WriteString(arg)
		}
		if req.NoReply {
			builder.WriteString(" noreply")
		}
		builder.WriteString("\r\n")
		return writeASCIIString(ctx, builder.String())
	}
}

func writeStorageLine(builder *strings.Builder, req Request, includeCAS bool) {
	builder.WriteString(string(req.Command))
	builder.WriteByte(' ')
	builder.WriteString(req.Key)
	builder.WriteByte(' ')
	builder.WriteString(strconv.FormatUint(uint64(req.Flags), 10))
	builder.WriteByte(' ')
	builder.WriteString(strconv.FormatInt(req.Exptime, 10))
	builder.WriteByte(' ')
	builder.WriteString(strconv.Itoa(readable(req.Value)))
	if includeCAS {
		builder.WriteByte(' ')
		builder.WriteString(strconv.FormatUint(req.CAS, 10))
	}
	if req.NoReply {
		builder.WriteString(" noreply")
	}
	builder.WriteString("\r\n")
}

func writeRequestWithValue(ctx *channel.HandlerContext, req Request, line string) error {
	if err := writeASCIIString(ctx, line); err != nil {
		req.Release()
		return err
	}
	if req.Value != nil {
		if err := ctx.Write(req.Value); err != nil {
			req.Value.Release()
			return err
		}
		req.Value = nil
	}
	return writeASCIIString(ctx, "\r\n")
}

// ResponseEncoder 编码 Memcached ASCII response。
type ResponseEncoder struct{}

// NewResponseEncoder 创建 response encoder。
func NewResponseEncoder() *ResponseEncoder {
	return &ResponseEncoder{}
}

// Write 编码 response，其它消息透传。
func (e *ResponseEncoder) Write(ctx *channel.HandlerContext, msg any) error {
	resp, ok := msg.(Response)
	if !ok {
		return ctx.Write(msg)
	}
	return e.writeResponse(ctx, resp)
}

func (e *ResponseEncoder) writeResponse(ctx *channel.HandlerContext, resp Response) error {
	if len(resp.Values) > 0 {
		return writeValuesResponse(ctx, resp)
	}
	line := string(resp.Status)
	if resp.Message != "" {
		line += " " + resp.Message
	}
	return writeASCIIString(ctx, line+"\r\n")
}

func writeValuesResponse(ctx *channel.HandlerContext, resp Response) error {
	for i := range resp.Values {
		value := resp.Values[i]
		var builder strings.Builder
		builder.WriteString("VALUE ")
		builder.WriteString(value.Key)
		builder.WriteByte(' ')
		builder.WriteString(strconv.FormatUint(uint64(value.Flags), 10))
		builder.WriteByte(' ')
		builder.WriteString(strconv.Itoa(readable(value.Data)))
		if value.CAS != 0 {
			builder.WriteByte(' ')
			builder.WriteString(strconv.FormatUint(value.CAS, 10))
		}
		builder.WriteString("\r\n")
		if err := writeASCIIString(ctx, builder.String()); err != nil {
			releaseValues(resp.Values[i:])
			return err
		}
		if value.Data != nil {
			if err := ctx.Write(value.Data); err != nil {
				value.Data.Release()
				resp.Values[i].Data = nil
				releaseValues(resp.Values[i+1:])
				return err
			}
			resp.Values[i].Data = nil
		}
		if err := writeASCIIString(ctx, "\r\n"); err != nil {
			releaseValues(resp.Values[i+1:])
			return err
		}
	}
	return writeASCIIString(ctx, string(StatusEnd)+"\r\n")
}

func releaseValues(values []Value) {
	for i := range values {
		values[i].Release()
	}
}
