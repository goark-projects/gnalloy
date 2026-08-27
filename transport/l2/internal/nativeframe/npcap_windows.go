//go:build windows

package nativeframe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	defaultNpcapReadTimeout = 100 * time.Millisecond
	pcapErrbufSize          = 256
	pcapDLTEn10MB           = 1
)

var npcap = newNpcapLibrary()

type npcapEndpoint struct {
	handle  atomic.Uintptr
	device  string
	ifName  string
	ifIndex int
	ether   uint16
}

type npcapLibrary struct {
	dll          *windows.LazyDLL
	create       *windows.LazyProc
	setSnaplen   *windows.LazyProc
	setPromisc   *windows.LazyProc
	setTimeout   *windows.LazyProc
	setBuffer    *windows.LazyProc
	setImmediate *windows.LazyProc
	activate     *windows.LazyProc
	nextEx       *windows.LazyProc
	sendPacketP  *windows.LazyProc
	close        *windows.LazyProc
	datalink     *windows.LazyProc
	compile      *windows.LazyProc
	setFilter    *windows.LazyProc
	freeCode     *windows.LazyProc
	getErr       *windows.LazyProc
	findAllDevs  *windows.LazyProc
	freeAllDevs  *windows.LazyProc
}

// OpenNpcap 打开 Windows Npcap 设备并返回原生二层帧 endpoint。
func OpenNpcap(ctx context.Context, cfg Config) (Endpoint, error) {
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
	device, iface, err := resolveNpcapDevice(cfg)
	if err != nil {
		return nil, err
	}
	handle, err := npcap.openLiveHandle(device, snapshot, cfg.Promiscuous, cfg.ReadTimeout, cfg.Immediate, cfg.BufferSize)
	if err != nil {
		return nil, err
	}
	if linkType, err := npcap.dataLink(handle); err != nil {
		npcap.closeHandle(handle)
		return nil, err
	} else if linkType != pcapDLTEn10MB {
		npcap.closeHandle(handle)
		return nil, fmt.Errorf("%w: unsupported datalink type %d", ErrInvalidConfig, linkType)
	}
	if cfg.EtherType != 0 {
		if err := npcap.setEtherTypeFilter(handle, cfg.EtherType); err != nil {
			npcap.closeHandle(handle)
			return nil, err
		}
	}
	ep := &npcapEndpoint{device: device, ether: cfg.EtherType}
	if iface != nil {
		ep.ifName = iface.Name
		ep.ifIndex = iface.Index
	} else {
		ep.ifName = device
	}
	ep.handle.Store(handle)
	return ep, nil
}

func (e *npcapEndpoint) Addr() string {
	if e == nil {
		return ""
	}
	if e.ifName != "" {
		return e.ifName
	}
	return e.device
}

func (e *npcapEndpoint) ReadFrame(ctx context.Context, _ int) (Frame, error) {
	if e == nil {
		return Frame{}, ErrClosed
	}
	for {
		handle := e.handle.Load()
		if handle == 0 {
			return Frame{}, ErrClosed
		}
		if ctx != nil {
			select {
			case <-ctx.Done():
				return Frame{}, ctx.Err()
			default:
			}
		}
		frame, ok, err := npcap.nextFrame(handle, e.ifName, e.ifIndex, e.ether)
		if err != nil {
			return Frame{}, err
		}
		if ok {
			return frame, nil
		}
	}
}

func (e *npcapEndpoint) WriteFrame(_ context.Context, data []byte) error {
	if e == nil {
		return ErrClosed
	}
	if len(data) == 0 {
		return ErrInvalidFrame
	}
	handle := e.handle.Load()
	if handle == 0 {
		return ErrClosed
	}
	return npcap.sendPacket(handle, data)
}

func (e *npcapEndpoint) Close() error {
	if e == nil {
		return nil
	}
	handle := e.handle.Swap(0)
	if handle == 0 {
		return nil
	}
	npcap.closeHandle(handle)
	return nil
}

func newNpcapLibrary() *npcapLibrary {
	dll := windows.NewLazySystemDLL("wpcap.dll")
	return &npcapLibrary{
		dll:          dll,
		create:       dll.NewProc("pcap_create"),
		setSnaplen:   dll.NewProc("pcap_set_snaplen"),
		setPromisc:   dll.NewProc("pcap_set_promisc"),
		setTimeout:   dll.NewProc("pcap_set_timeout"),
		setBuffer:    dll.NewProc("pcap_set_buffer_size"),
		setImmediate: dll.NewProc("pcap_set_immediate_mode"),
		activate:     dll.NewProc("pcap_activate"),
		nextEx:       dll.NewProc("pcap_next_ex"),
		sendPacketP:  dll.NewProc("pcap_sendpacket"),
		close:        dll.NewProc("pcap_close"),
		datalink:     dll.NewProc("pcap_datalink"),
		compile:      dll.NewProc("pcap_compile"),
		setFilter:    dll.NewProc("pcap_setfilter"),
		freeCode:     dll.NewProc("pcap_freecode"),
		getErr:       dll.NewProc("pcap_geterr"),
		findAllDevs:  dll.NewProc("pcap_findalldevs"),
		freeAllDevs:  dll.NewProc("pcap_freealldevs"),
	}
}

func (l *npcapLibrary) openLiveHandle(device string, snapshot int, promisc bool, timeout time.Duration, immediate bool, bufferSize int) (uintptr, error) {
	if err := l.ensureLoaded(); err != nil {
		return 0, err
	}
	deviceBytes, err := cBytes(device)
	if err != nil {
		return 0, err
	}
	errbuf := make([]byte, pcapErrbufSize)
	handle, _, callErr := l.create.Call(
		uintptr(unsafe.Pointer(&deviceBytes[0])),
		uintptr(unsafe.Pointer(&errbuf[0])),
	)
	runtime.KeepAlive(deviceBytes)
	runtime.KeepAlive(errbuf)
	if handle == 0 {
		return 0, l.callError(0, callErr, errbuf)
	}
	if err := l.setInt(handle, l.setSnaplen, snapshot); err != nil {
		l.closeHandle(handle)
		return 0, err
	}
	promiscValue := 0
	if promisc {
		promiscValue = 1
	}
	if err := l.setInt(handle, l.setPromisc, promiscValue); err != nil {
		l.closeHandle(handle)
		return 0, err
	}
	if err := l.setInt(handle, l.setTimeout, npcapTimeoutMillis(timeout)); err != nil {
		l.closeHandle(handle)
		return 0, err
	}
	if bufferSize > 0 {
		if err := l.setInt(handle, l.setBuffer, bufferSize); err != nil {
			l.closeHandle(handle)
			return 0, err
		}
	}
	if immediate {
		if err := l.setInt(handle, l.setImmediate, 1); err != nil {
			l.closeHandle(handle)
			return 0, err
		}
	}
	ret, _, callErr := l.activate.Call(handle)
	if int32(ret) < 0 {
		l.closeHandle(handle)
		return 0, l.callError(handle, callErr, nil)
	}
	return handle, nil
}

func (l *npcapLibrary) setInt(handle uintptr, proc *windows.LazyProc, value int) error {
	ret, _, callErr := proc.Call(handle, uintptr(value))
	if int32(ret) != 0 {
		return l.callError(handle, callErr, nil)
	}
	return nil
}

func (l *npcapLibrary) dataLink(handle uintptr) (int, error) {
	value, _, callErr := l.datalink.Call(handle)
	if int32(value) < 0 {
		return 0, l.callError(handle, callErr, nil)
	}
	return int(int32(value)), nil
}

func (l *npcapLibrary) nextFrame(handle uintptr, ifaceName string, ifaceIndex int, etherType uint16) (Frame, bool, error) {
	var header *pcapPkthdr
	var dataPtr *byte
	ret, _, callErr := l.nextEx.Call(
		handle,
		uintptr(unsafe.Pointer(&header)),
		uintptr(unsafe.Pointer(&dataPtr)),
	)
	switch int32(ret) {
	case 1:
		if header == nil || dataPtr == nil {
			return Frame{}, false, ErrInvalidFrame
		}
		if header.Caplen == 0 || header.Caplen > header.Len && header.Len != 0 {
			return Frame{}, false, ErrInvalidFrame
		}
		data := unsafe.Slice(dataPtr, int(header.Caplen))
		copied := append([]byte(nil), data...)
		if !matchEtherType(copied, etherType) {
			return Frame{}, false, nil
		}
		return Frame{Meta: parseEthernetMeta(ifaceName, ifaceIndex, copied), Data: copied}, true, nil
	case 0:
		return Frame{}, false, nil
	case -2:
		return Frame{}, false, ErrClosed
	default:
		return Frame{}, false, l.callError(handle, callErr, nil)
	}
}

func (l *npcapLibrary) sendPacket(handle uintptr, data []byte) error {
	ret, _, callErr := l.sendPacketP.Call(
		handle,
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
	)
	runtime.KeepAlive(data)
	if int32(ret) != 0 {
		return l.callError(handle, callErr, nil)
	}
	return nil
}

func (l *npcapLibrary) setEtherTypeFilter(handle uintptr, etherType uint16) error {
	filter := fmt.Sprintf("ether proto 0x%04x", etherType)
	filterBytes, err := cBytes(filter)
	if err != nil {
		return err
	}
	var program pcapBpfProgram
	ret, _, callErr := l.compile.Call(
		handle,
		uintptr(unsafe.Pointer(&program)),
		uintptr(unsafe.Pointer(&filterBytes[0])),
		1,
		0xffffffff,
	)
	runtime.KeepAlive(filterBytes)
	if int32(ret) != 0 {
		return l.callError(handle, callErr, nil)
	}
	defer l.freeCode.Call(uintptr(unsafe.Pointer(&program)))
	ret, _, callErr = l.setFilter.Call(handle, uintptr(unsafe.Pointer(&program)))
	if int32(ret) != 0 {
		return l.callError(handle, callErr, nil)
	}
	return nil
}

func (l *npcapLibrary) closeHandle(handle uintptr) {
	if handle != 0 {
		l.close.Call(handle)
	}
}

func (l *npcapLibrary) callError(_ uintptr, callErr error, errbuf []byte) error {
	if text := cStringFromBytes(errbuf); text != "" {
		return fmt.Errorf("%w: %s", ErrUnavailable, text)
	}
	if callErr != nil && !errors.Is(callErr, windows.ERROR_SUCCESS) {
		return fmt.Errorf("%w: %v", ErrUnavailable, callErr)
	}
	return ErrUnavailable
}

func resolveNpcapDevice(cfg Config) (string, *net.Interface, error) {
	if cfg.InterfaceName == "" && cfg.InterfaceIndex <= 0 {
		return "", nil, fmt.Errorf("%w: missing interface", ErrInvalidConfig)
	}
	var iface *net.Interface
	name := cfg.InterfaceName
	if name == "" {
		resolved, err := net.InterfaceByIndex(cfg.InterfaceIndex)
		if err != nil {
			return "", nil, err
		}
		iface = resolved
		name = resolved.Name
	}
	if strings.HasPrefix(strings.ToLower(name), `\device\npf_`) {
		return name, iface, nil
	}
	device, ok := npcap.findDevice(name)
	if ok {
		return device, iface, nil
	}
	return name, iface, nil
}

func (l *npcapLibrary) findDevice(name string) (string, bool) {
	if strings.TrimSpace(name) == "" {
		return "", false
	}
	if err := l.ensureLoaded(); err != nil {
		return "", false
	}
	var alldevs *pcapIf
	errbuf := make([]byte, pcapErrbufSize)
	ret, _, _ := l.findAllDevs.Call(
		uintptr(unsafe.Pointer(&alldevs)),
		uintptr(unsafe.Pointer(&errbuf[0])),
	)
	runtime.KeepAlive(errbuf)
	if int32(ret) != 0 || alldevs == nil {
		return "", false
	}
	defer l.freeAllDevs.Call(uintptr(unsafe.Pointer(alldevs)))
	for dev := alldevs; dev != nil; dev = dev.Next {
		deviceName := cString(dev.Name)
		description := cString(dev.Description)
		if equalNpcapName(name, deviceName) || equalNpcapName(name, description) {
			return deviceName, true
		}
	}
	return "", false
}

func (l *npcapLibrary) ensureLoaded() error {
	if err := l.dll.Load(); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	for _, proc := range []*windows.LazyProc{
		l.create,
		l.setSnaplen,
		l.setPromisc,
		l.setTimeout,
		l.setBuffer,
		l.setImmediate,
		l.activate,
		l.nextEx,
		l.sendPacketP,
		l.close,
		l.datalink,
		l.compile,
		l.setFilter,
		l.freeCode,
		l.getErr,
		l.findAllDevs,
		l.freeAllDevs,
	} {
		if err := proc.Find(); err != nil {
			return fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}
	return nil
}

func equalNpcapName(want string, got string) bool {
	if strings.EqualFold(want, got) {
		return true
	}
	return strings.Contains(strings.ToLower(got), strings.ToLower(want))
}

func npcapTimeoutMillis(timeout time.Duration) int {
	if timeout <= 0 {
		timeout = defaultNpcapReadTimeout
	}
	millis := timeout.Milliseconds()
	if millis <= 0 {
		return 1
	}
	const maxInt32 = 1<<31 - 1
	if millis > maxInt32 {
		return maxInt32
	}
	return int(millis)
}

func cBytes(value string) ([]byte, error) {
	if strings.ContainsRune(value, 0) {
		return nil, fmt.Errorf("%w: string contains NUL", ErrInvalidConfig)
	}
	return append([]byte(value), 0), nil
}

func cString(ptr *byte) string {
	if ptr == nil {
		return ""
	}
	var out []byte
	for i := 0; i < 4096; i++ {
		b := *(*byte)(unsafe.Add(unsafe.Pointer(ptr), i))
		if b == 0 {
			break
		}
		out = append(out, b)
	}
	return string(out)
}

func cStringFromBytes(buf []byte) string {
	if len(buf) == 0 {
		return ""
	}
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

type pcapPkthdr struct {
	Sec    int32
	Usec   int32
	Caplen uint32
	Len    uint32
}

type pcapBpfProgram struct {
	Len   uint32
	Insns uintptr
}

type pcapIf struct {
	Next        *pcapIf
	Name        *byte
	Description *byte
	Addresses   uintptr
	Flags       uint32
}
