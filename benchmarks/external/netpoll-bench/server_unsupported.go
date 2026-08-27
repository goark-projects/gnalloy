//go:build windows || (!linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly)

package main

import "context"

type echoServer struct {
	addr string
}

func startEchoServer(context.Context, config) (*echoServer, error) {
	return nil, errUnsupportedPlatform
}

func (*echoServer) stop() {}
