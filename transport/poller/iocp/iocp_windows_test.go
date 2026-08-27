//go:build windows

package iocp

import (
	"errors"
	"testing"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/transport/poller"
	"golang.org/x/sys/windows"
)

func TestSubmitAcceptClosesAcceptedSocketOnImmediateFailure(t *testing.T) {
	if err := ensureWSAStartup(); err != nil {
		t.Fatal(err)
	}
	p, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	accepted, err := windows.WSASocket(windows.AF_INET, windows.SOCK_STREAM, windows.IPPROTO_TCP, nil, 0, windows.WSA_FLAG_OVERLAPPED)
	if err != nil {
		t.Fatal(err)
	}
	err = p.Submit(poller.IORequest{
		Op:         poller.OpAccept,
		FD:         poller.FDRef{FD: 1},
		AcceptedFD: poller.FDRef{FD: int(accepted)},
	})
	if err == nil {
		_ = windows.Closesocket(accepted)
		t.Fatal("Submit unexpectedly succeeded")
	}
	if closeErr := windows.Closesocket(accepted); !errors.Is(closeErr, windows.WSAENOTSOCK) {
		t.Fatalf("accepted socket was not closed, close err=%v", closeErr)
	}
}

func TestMakeWriteBuffersUsesInlineStorageForSingleBuffer(t *testing.T) {
	buf := buffer.NewHeapBuffer(8)
	defer buf.Release()
	if _, err := buf.WriteBytes([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	var inline [1]windows.WSABuf
	wsabufs, err := makeWriteBuffers(poller.IORequest{Buf: buf}, inline[:0])
	if err != nil {
		t.Fatal(err)
	}
	if len(wsabufs) != 1 || wsabufs[0].Len != 4 {
		t.Fatalf("wsabufs=%+v, want one 4-byte buffer", wsabufs)
	}
	if &wsabufs[0] != &inline[0] {
		t.Fatal("single-buffer write should reuse inline WSABuf storage")
	}
}

func ensureWSAStartup() error {
	var data windows.WSAData
	return windows.WSAStartup(0x202, &data)
}
