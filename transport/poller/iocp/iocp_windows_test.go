//go:build windows

package iocp

import (
	"errors"
	"testing"
	"unsafe"

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

func TestPendingRequestOverlappedRoundTrip(t *testing.T) {
	if offset := unsafe.Offsetof(pendingRequest{}.ov); offset != 0 {
		t.Fatalf("overlapped offset=%d, want zero", offset)
	}
	pending := &pendingRequest{}
	if got := pendingFromOverlapped(&pending.ov); got != pending {
		t.Fatalf("pendingFromOverlapped returned %p, want %p", got, pending)
	}
}

func TestPendingRequestFreelistResetsState(t *testing.T) {
	buf := buffer.NewHeapBuffer(8)
	defer buf.Release()
	if _, err := buf.WriteBytes([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	p := &Poller{}
	first := p.acquirePending(poller.IORequest{
		Op:  poller.OpWrite,
		FD:  poller.FDRef{FD: 10},
		Buf: buf,
	})
	first.accept = &acceptContext{}
	first.wsabufs = append(first.wsabufs, windows.WSABuf{Len: 4})
	first.fromLen = 8
	first.toLen = 8
	p.unlinkPending(first)
	p.releasePending(first)

	second := p.acquirePending(poller.IORequest{
		Op:  poller.OpRead,
		FD:  poller.FDRef{FD: 11},
		Buf: buf,
	})
	if second != first {
		t.Fatal("pending freelist did not reuse released state")
	}
	if second.req.Op != poller.OpRead || second.req.FD.FD != 11 {
		t.Fatalf("request=%+v, want fresh read request", second.req)
	}
	if second.accept != nil || len(second.wsabufs) != 0 || second.fromLen != 0 || second.toLen != 0 {
		t.Fatalf("pending was not reset: %+v", second)
	}
	if second.prev != nil || second.next != nil || p.active != second {
		t.Fatal("active list was not reset correctly")
	}
}

func ensureWSAStartup() error {
	var data windows.WSAData
	return windows.WSAStartup(0x202, &data)
}
