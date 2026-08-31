package driver

import (
	"context"
	"fmt"
	"reflect"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/rxtx"
)

// Driver 把 Backend 适配成 transport/rxtx.Driver。
type Driver struct {
	Backend Backend
}

// NewDriver 创建 RXTX/serial driver 适配器。
func NewDriver(backend Backend) Driver {
	return Driver{Backend: backend}
}

// Dial 清理递归 driver 引用后委派给真实串口后端。
func (d Driver) Dial(ctx context.Context, cfg bootstrap.ClientConfig, serialCfg rxtx.Config) (channel.Channel, error) {
	backend := d.Backend
	if backendMissing(backend) {
		return nil, unsupported(ErrMissingBackend)
	}
	serialCfg.Driver = nil
	return backend.DialRXTX(ctx, DialConfig{Bootstrap: cfg, Serial: serialCfg})
}

func backendMissing(backend Backend) bool {
	if backend == nil {
		return true
	}
	if _, ok := backend.(BackendFunc); ok {
		return false
	}
	value := reflect.ValueOf(backend)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func unsupported(err error) error {
	return fmt.Errorf("%w: %w", rxtx.ErrUnsupportedRXTX, err)
}

var _ rxtx.Driver = Driver{}
