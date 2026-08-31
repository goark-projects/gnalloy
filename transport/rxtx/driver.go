package rxtx

import (
	"context"
	"reflect"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
)

// Driver 把外部串口实现适配到 gnalloy Dialer。
type Driver interface {
	Dial(ctx context.Context, cfg bootstrap.ClientConfig, serialCfg Config) (channel.Channel, error)
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
