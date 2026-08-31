package driver_test

import (
	"context"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/rxtx"
	rxtxdriver "goark.dev/gnalloy/transport/rxtx/driver"
)

func ExampleNewDriver() {
	backend := rxtxdriver.BackendFunc(func(context.Context, rxtxdriver.DialConfig) (channel.Channel, error) {
		return nil, nil
	})

	_ = rxtx.NewTransport(rxtx.Config{
		Driver: rxtxdriver.NewDriver(backend),
	})
}
