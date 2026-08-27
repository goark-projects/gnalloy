//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package nativeframe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

const maxBPFDevices = 256

type bpfEndpoint struct {
	fd       int
	iface    *net.Interface
	etherTyp uint16

	mu      sync.Mutex
	readBuf []byte
	pending []byte
	offset  int
	closed  bool
}

// OpenBPF 打开 BSD/macOS BPF 设备并绑定到指定网卡。
func OpenBPF(ctx context.Context, cfg Config) (Endpoint, error) {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	if cfg.BufferSize < 0 {
		return nil, fmt.Errorf("%w: negative buffer size", ErrInvalidConfig)
	}
	snapshot, err := normalizeSnapshotLength(cfg.SnapshotLength)
	if err != nil {
		return nil, err
	}
	iface, err := resolveInterface(cfg.InterfaceName, cfg.InterfaceIndex)
	if err != nil {
		return nil, err
	}
	fd, err := openBPFDevice()
	if err != nil {
		return nil, err
	}
	if err := configureBPF(fd, iface, cfg, snapshot); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	bufferLength, err := unix.IoctlGetInt(fd, unix.BIOCGBLEN)
	if err != nil || bufferLength <= 0 {
		bufferLength = snapshot + unix.SizeofBpfHdr
	}
	return &bpfEndpoint{
		fd:       fd,
		iface:    iface,
		etherTyp: cfg.EtherType,
		readBuf:  make([]byte, bufferLength),
	}, nil
}

func (e *bpfEndpoint) Addr() string {
	if e == nil || e.iface == nil {
		return ""
	}
	return e.iface.Name
}

func (e *bpfEndpoint) ReadFrame(ctx context.Context, readBufferSize int) (Frame, error) {
	if e == nil {
		return Frame{}, ErrClosed
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for {
		if frame, ok, err := e.nextPendingFrame(); ok || err != nil {
			return frame, err
		}
		if e.closed {
			return Frame{}, ErrClosed
		}
		if ctx != nil {
			select {
			case <-ctx.Done():
				return Frame{}, ctx.Err()
			default:
			}
		}
		if readBufferSize > len(e.readBuf) {
			e.readBuf = make([]byte, readBufferSize)
		}
		n, err := unix.Read(e.fd, e.readBuf)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			if errors.Is(err, unix.EBADF) {
				return Frame{}, ErrClosed
			}
			return Frame{}, err
		}
		if n <= 0 {
			continue
		}
		e.pending = e.readBuf[:n]
		e.offset = 0
	}
}

func (e *bpfEndpoint) WriteFrame(_ context.Context, data []byte) error {
	if e == nil {
		return ErrClosed
	}
	if len(data) == 0 {
		return ErrInvalidFrame
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return ErrClosed
	}
	for len(data) > 0 {
		n, err := unix.Write(e.fd, data)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return err
		}
		if n <= 0 {
			return ErrInvalidFrame
		}
		data = data[n:]
	}
	return nil
}

func (e *bpfEndpoint) Close() error {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	if e.fd < 0 {
		return nil
	}
	err := unix.Close(e.fd)
	e.fd = -1
	return err
}

func (e *bpfEndpoint) nextPendingFrame() (Frame, bool, error) {
	for e.offset < len(e.pending) {
		if len(e.pending)-e.offset < unix.SizeofBpfHdr {
			e.pending = nil
			e.offset = 0
			return Frame{}, false, ErrInvalidFrame
		}
		hdr := (*unix.BpfHdr)(unsafe.Pointer(&e.pending[e.offset]))
		headerLength := int(hdr.Hdrlen)
		capturedLength := int(hdr.Caplen)
		if headerLength <= 0 || capturedLength < 0 {
			e.pending = nil
			e.offset = 0
			return Frame{}, false, ErrInvalidFrame
		}
		packetStart := e.offset + headerLength
		packetEnd := packetStart + capturedLength
		nextOffset := e.offset + bpfWordAlign(headerLength+capturedLength)
		if packetEnd > len(e.pending) || nextOffset < packetEnd {
			e.pending = nil
			e.offset = 0
			return Frame{}, false, ErrInvalidFrame
		}
		e.offset = nextOffset
		data := append([]byte(nil), e.pending[packetStart:packetEnd]...)
		if len(data) == 0 || !matchEtherType(data, e.etherTyp) {
			continue
		}
		return Frame{Meta: parseEthernetMeta(e.iface.Name, e.iface.Index, data), Data: data}, true, nil
	}
	e.pending = nil
	e.offset = 0
	return Frame{}, false, nil
}

func configureBPF(fd int, iface *net.Interface, cfg Config, snapshot int) error {
	if cfg.BufferSize > 0 {
		if err := unix.IoctlSetPointerInt(fd, unix.BIOCSBLEN, cfg.BufferSize); err != nil {
			return err
		}
	}
	if err := ioctlBPFInterface(fd, iface.Name); err != nil {
		return err
	}
	if cfg.Immediate {
		if err := unix.IoctlSetPointerInt(fd, unix.BIOCIMMEDIATE, 1); err != nil {
			return err
		}
	}
	if err := unix.IoctlSetPointerInt(fd, unix.BIOCSHDRCMPLT, 1); err != nil {
		return err
	}
	if cfg.Promiscuous {
		if err := ioctlNoArg(fd, unix.BIOCPROMISC); err != nil {
			return err
		}
	}
	if cfg.EtherType != 0 {
		if err := attachEtherTypeFilter(fd, cfg.EtherType, snapshot); err != nil {
			return err
		}
	}
	return nil
}

func openBPFDevice() (int, error) {
	if fd, err := unix.Open("/dev/bpf", unix.O_RDWR|unix.O_CLOEXEC, 0); err == nil {
		return fd, nil
	}
	var first error
	for i := range maxBPFDevices {
		path := fmt.Sprintf("/dev/bpf%d", i)
		fd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC, 0)
		if err == nil {
			return fd, nil
		}
		if first == nil {
			first = err
		}
	}
	if first == nil {
		first = ErrUnavailable
	}
	return -1, fmt.Errorf("%w: %v", ErrUnavailable, first)
}

func ioctlBPFInterface(fd int, name string) error {
	var req bpfIfreq
	if len(name) >= len(req.Name) {
		return fmt.Errorf("%w: interface name too long", ErrInvalidConfig)
	}
	copy(req.Name[:], name)
	return ioctlPtr(fd, unix.BIOCSETIF, unsafe.Pointer(&req))
}

func attachEtherTypeFilter(fd int, etherType uint16, snapshot int) error {
	insns := []unix.BpfInsn{
		{Code: 0x28, K: 12},
		{Code: 0x15, Jt: 0, Jf: 1, K: uint32(etherType)},
		{Code: 0x06, K: uint32(snapshot)},
		{Code: 0x06, K: 0},
	}
	program := unix.BpfProgram{Len: uint32(len(insns)), Insns: &insns[0]}
	err := ioctlPtr(fd, unix.BIOCSETF, unsafe.Pointer(&program))
	runtime.KeepAlive(insns)
	return err
}

func ioctlNoArg(fd int, req uint) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), 0)
	if errno != 0 {
		return errno
	}
	return nil
}

func ioctlPtr(fd int, req uint, arg unsafe.Pointer) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

func bpfWordAlign(length int) int {
	alignment := int(unix.BPF_ALIGNMENT)
	return (length + alignment - 1) &^ (alignment - 1)
}

type bpfIfreq struct {
	Name [16]byte
	Data [128]byte
}
