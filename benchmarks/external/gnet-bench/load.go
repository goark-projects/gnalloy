package main

import "context"

func runLoad(ctx context.Context, addr string, cfg config) (benchResult, error) {
	if cfg.Protocol == "http1" {
		return runHTTP1Load(ctx, addr, cfg)
	}
	return runTCPEchoLoad(ctx, addr, cfg)
}
