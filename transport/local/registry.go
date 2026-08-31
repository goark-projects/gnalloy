package local

import "sync"

var globalRegistry = &registry{servers: make(map[string]*Server)}

type registry struct {
	mu      sync.Mutex
	servers map[string]*Server
}

func registerServer(address string, server *Server) error {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	if _, exists := globalRegistry.servers[address]; exists {
		return ErrAddressInUse
	}
	globalRegistry.servers[address] = server
	return nil
}

func lookupServer(address string) (*Server, bool) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	server, ok := globalRegistry.servers[address]
	return server, ok
}

func unregisterServer(address string, server *Server) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	if globalRegistry.servers[address] == server {
		delete(globalRegistry.servers, address)
	}
}
