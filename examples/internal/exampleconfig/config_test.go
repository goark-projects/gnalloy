package exampleconfig

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/transport"
)

func TestParseBackendNames(t *testing.T) {
	tests := []struct {
		name string
		want transport.BackendKind
	}{
		{name: "memory", want: transport.BackendMemory},
		{name: "epoll", want: transport.BackendEpoll},
		{name: "iouring", want: transport.BackendIOUring},
		{name: "io_uring", want: transport.BackendIOUring},
		{name: "kqueue", want: transport.BackendKqueue},
		{name: "iocp", want: transport.BackendIOCP},
	}

	for _, tt := range tests {
		got, err := ParseBackend(tt.name)
		if err != nil {
			t.Fatalf("ParseBackend(%q) err=%v", tt.name, err)
		}
		if got != tt.want {
			t.Fatalf("ParseBackend(%q)=%v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestParseBackendRejectsUnknownName(t *testing.T) {
	_, err := ParseBackend("bad")
	if !errors.Is(err, ErrInvalidBackend) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidBackend)
	}
}

func TestOptionsResolveRejectsInvalidSizes(t *testing.T) {
	opts := &Options{BackendName: "memory", Boss: 1, Workers: 0, ReadBufferSize: 4096}
	if err := opts.Resolve(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidConfig)
	}
}

func TestOptionsResolveRejectsFixedBuffersWithoutIOUringMmap(t *testing.T) {
	tests := []*Options{
		{BackendName: "iouring", Boss: 1, Workers: 1, ReadBufferSize: 4096, IOUringFixedBuffers: true},
		{BackendName: "memory", Boss: 1, Workers: 1, ReadBufferSize: 4096, Mmap: true, IOUringFixedBuffers: true},
	}
	for _, opts := range tests {
		if err := opts.Resolve(); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("Resolve(%+v) err=%v, want %v", opts, err, ErrInvalidConfig)
		}
	}
}

func TestOptionsTCPConfigEnablesFixedBuffers(t *testing.T) {
	opts := &Options{
		BackendName:            "iouring",
		Boss:                   1,
		Workers:                1,
		ReadBufferSize:         4096,
		Mmap:                   true,
		MmapBlockSize:          4096,
		MmapBlocks:             16,
		IOUringFixedBuffers:    true,
		IOUringMultishotAccept: true,
	}
	cfg, err := opts.TCPConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IOUringFixedBuffers {
		t.Fatal("tcp config did not enable io_uring fixed buffers")
	}
}
