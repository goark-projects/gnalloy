package quic

import (
	"context"
	"io"
	"reflect"

	nativequic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/qlogwriter"
)

// QLogTraceInfo 描述即将创建的 qlog trace。
type QLogTraceInfo struct {
	// Client 表示 trace 位于客户端视角。
	Client bool
	// OriginalDestinationConnectionID 是连接初始目的 CID 的稳定字节快照。
	OriginalDestinationConnectionID []byte
}

// QLogWriterFactory 为每条连接打开独立 qlog 输出。
type QLogWriterFactory interface {
	// OpenQLogTrace 返回一个新 trace 的写入器；返回 nil 表示跳过该连接。
	OpenQLogTrace(ctx context.Context, info QLogTraceInfo) (io.WriteCloser, error)
}

// QLogWriterFactoryFunc 允许函数直接作为 qlog writer factory。
type QLogWriterFactoryFunc func(ctx context.Context, info QLogTraceInfo) (io.WriteCloser, error)

// OpenQLogTrace 实现 QLogWriterFactory。
func (f QLogWriterFactoryFunc) OpenQLogTrace(ctx context.Context, info QLogTraceInfo) (io.WriteCloser, error) {
	if f == nil {
		return nil, nil
	}
	return f(ctx, info)
}

// QLogConfig 描述 RFC 9000 适配层的 qlog 输出边界。
type QLogConfig struct {
	// WriterFactory 为每条连接创建 qlog writer；为空时关闭 qlog。
	WriterFactory QLogWriterFactory
	// EventSchemas 传给 qlog 文件头，用于声明扩展事件 schema。
	EventSchemas []string
}

// Enabled 返回当前配置是否启用 qlog。
func (c QLogConfig) Enabled() bool {
	if c.WriterFactory == nil {
		return false
	}
	value := reflect.ValueOf(c.WriterFactory)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !value.IsNil()
	default:
		return true
	}
}

func newNativeTracer(cfg QLogConfig) func(context.Context, bool, nativequic.ConnectionID) qlogwriter.Trace {
	if !cfg.Enabled() {
		return nil
	}
	schemas := cloneStrings(cfg.EventSchemas)
	return func(ctx context.Context, isClient bool, connID nativequic.ConnectionID) qlogwriter.Trace {
		id := append([]byte(nil), connID.Bytes()...)
		writer, err := cfg.WriterFactory.OpenQLogTrace(normalizeContext(ctx), QLogTraceInfo{
			Client:                          isClient,
			OriginalDestinationConnectionID: id,
		})
		if err != nil || writer == nil {
			return nil
		}
		trace := qlogwriter.NewConnectionFileSeq(writer, isClient, connID, schemas)
		go trace.Run()
		return trace
	}
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
