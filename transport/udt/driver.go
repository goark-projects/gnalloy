package udt

import (
	"context"
	"reflect"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
)

// Driver 把外部 UDT 实现适配到 gnalloy Bootstrap/Dialer。
type Driver interface {
	Bind(ctx context.Context, cfg bootstrap.ServerConfig, udtCfg Config) (bootstrap.Server, error)
	Dial(ctx context.Context, cfg bootstrap.ClientConfig, udtCfg Config) (channel.Channel, error)
}

func driverMissing(driver Driver) bool {
	if driver == nil {
		return true
	}
	value := reflect.ValueOf(driver)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
