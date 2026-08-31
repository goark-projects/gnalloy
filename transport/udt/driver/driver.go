package driver

import (
	"context"
	"fmt"
	"reflect"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/udt"
)

// Driver 把 Backend 适配成 transport/udt.Driver。
type Driver struct {
	Backend Backend
}

// NewDriver 创建 UDT driver 适配器。
func NewDriver(backend Backend) Driver {
	return Driver{Backend: backend}
}

// Bind 清理递归 driver 引用后委派给真实 UDT 后端。
func (d Driver) Bind(ctx context.Context, cfg bootstrap.ServerConfig, udtCfg udt.Config) (bootstrap.Server, error) {
	backend := d.Backend
	if backendMissing(backend) {
		return nil, unsupported(ErrMissingBackend)
	}
	udtCfg.Driver = nil
	return backend.BindUDT(ctx, BindConfig{Bootstrap: cfg, UDT: udtCfg})
}

// Dial 清理递归 driver 引用后委派给真实 UDT 后端。
func (d Driver) Dial(ctx context.Context, cfg bootstrap.ClientConfig, udtCfg udt.Config) (channel.Channel, error) {
	backend := d.Backend
	if backendMissing(backend) {
		return nil, unsupported(ErrMissingBackend)
	}
	udtCfg.Driver = nil
	return backend.DialUDT(ctx, DialConfig{Bootstrap: cfg, UDT: udtCfg})
}

func backendMissing(backend Backend) bool {
	if backend == nil {
		return true
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
	return fmt.Errorf("%w: %w", udt.ErrUnsupportedUDT, err)
}

var _ udt.Driver = Driver{}
