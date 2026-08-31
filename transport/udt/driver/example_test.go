package driver_test

import (
	"context"

	"goark.dev/gnalloy/bootstrap"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/udt"
	udtdriver "goark.dev/gnalloy/transport/udt/driver"
)

func ExampleNewDriver() {
	backend := udtdriver.BackendFuncs{
		Bind: func(context.Context, udtdriver.BindConfig) (bootstrap.Server, error) {
			return nil, nil
		},
		Dial: func(context.Context, udtdriver.DialConfig) (channel.Channel, error) {
			return nil, nil
		},
	}

	_ = udt.NewTransport(udt.Config{
		Driver: udtdriver.NewDriver(backend),
	})
}
