package pcap

import (
	"encoding/binary"
	"io"
	"sync"
	"time"

	"goark.dev/gnalloy/buffer"
	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport/raw"
	"goark.dev/gnalloy/transport/udp"
)

const (
	defaultSnapLen  = 65535
	linkTypeUser0   = 147
	pcapHeaderSize  = 24
	pcapRecordSize  = 16
	pcapMagicNumber = 0xa1b2c3d4
)

// Config 描述 pcap 捕获行为。
type Config struct {
	// Writer 接收 pcap 字节流。
	Writer io.Writer
	// SnapLen 是单条记录最大捕获字节数，0 表示 65535。
	SnapLen uint32
	// LinkType 是 libpcap network 字段，0 表示 LINKTYPE_USER0。
	LinkType uint32
	// CaptureRead 控制是否捕获入站消息，默认开启。
	CaptureRead bool
	// CaptureWrite 控制是否捕获出站消息，默认开启。
	CaptureWrite bool
	// Clock 便于测试注入稳定时间。
	Clock func() time.Time
	// FailClosed 为 true 时捕获失败会中断当前读写路径。
	FailClosed bool
}

// Handler 将通过 pipeline 的 payload 写成 pcap 记录。
type Handler struct {
	writer       io.Writer
	snapLen      uint32
	linkType     uint32
	captureRead  bool
	captureWrite bool
	clock        func() time.Time
	failClosed   bool

	mu          sync.Mutex
	headerWrote bool
}

// NewHandler 创建 pcap handler。
func NewHandler(cfg Config) (*Handler, error) {
	if cfg.Writer == nil {
		return nil, ErrInvalidConfig
	}
	snapLen := cfg.SnapLen
	if snapLen == 0 {
		snapLen = defaultSnapLen
	}
	linkType := cfg.LinkType
	if linkType == 0 {
		linkType = linkTypeUser0
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	captureRead := cfg.CaptureRead
	captureWrite := cfg.CaptureWrite
	if !captureRead && !captureWrite {
		captureRead = true
		captureWrite = true
	}
	return &Handler{
		writer:       cfg.Writer,
		snapLen:      snapLen,
		linkType:     linkType,
		captureRead:  captureRead,
		captureWrite: captureWrite,
		clock:        clock,
		failClosed:   cfg.FailClosed,
	}, nil
}

// HandlerAdded 写入 pcap 全局头。
func (h *Handler) HandlerAdded(*channel.HandlerContext) error {
	return h.writeHeader()
}

// ChannelRead 捕获入站 payload 后继续传播原消息。
func (h *Handler) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if h.captureRead {
		if err := h.capture(msg); err != nil {
			ctx.FireExceptionCaught(err)
			if h.failClosed {
				release(msg)
				return
			}
		}
	}
	ctx.FireChannelRead(msg)
}

// Write 捕获出站 payload 后继续写出原消息。
func (h *Handler) Write(ctx *channel.HandlerContext, msg any) error {
	if h.captureWrite {
		if err := h.capture(msg); err != nil {
			if h.failClosed {
				release(msg)
				return err
			}
			ctx.FireExceptionCaught(err)
		}
	}
	return ctx.Write(msg)
}

func (h *Handler) writeHeader() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.headerWrote {
		return nil
	}
	var header [pcapHeaderSize]byte
	binary.LittleEndian.PutUint32(header[0:4], pcapMagicNumber)
	binary.LittleEndian.PutUint16(header[4:6], 2)
	binary.LittleEndian.PutUint16(header[6:8], 4)
	binary.LittleEndian.PutUint32(header[16:20], h.snapLen)
	binary.LittleEndian.PutUint32(header[20:24], h.linkType)
	if _, err := h.writer.Write(header[:]); err != nil {
		return err
	}
	h.headerWrote = true
	return nil
}

func (h *Handler) capture(msg any) error {
	slices := payloadSlices(nil, msg)
	if len(slices) == 0 {
		return nil
	}
	if err := h.writeHeader(); err != nil {
		return err
	}
	now := h.clock()
	h.mu.Lock()
	defer h.mu.Unlock()
	remaining := int(h.snapLen)
	length := payloadLength(slices)
	captured := length
	if captured > remaining {
		captured = remaining
	}
	var record [pcapRecordSize]byte
	binary.LittleEndian.PutUint32(record[0:4], uint32(now.Unix()))
	binary.LittleEndian.PutUint32(record[4:8], uint32(now.Nanosecond()/1000))
	binary.LittleEndian.PutUint32(record[8:12], uint32(captured))
	binary.LittleEndian.PutUint32(record[12:16], uint32(length))
	if _, err := h.writer.Write(record[:]); err != nil {
		return err
	}
	for _, data := range slices {
		if remaining <= 0 {
			break
		}
		if len(data) > remaining {
			data = data[:remaining]
		}
		if _, err := h.writer.Write(data); err != nil {
			return err
		}
		remaining -= len(data)
	}
	return nil
}

func payloadSlices(dst [][]byte, msg any) [][]byte {
	switch v := msg.(type) {
	case buffer.ByteBuf:
		return v.ReadableSlices(dst)
	case udp.Datagram:
		return payloadSlices(dst, v.Payload)
	case udp.Addressed:
		return payloadSlices(dst, v.Message)
	case raw.Packet:
		return payloadSlices(dst, v.Payload)
	case raw.Addressed:
		return payloadSlices(dst, v.Message)
	case []byte:
		return append(dst, v)
	case string:
		return append(dst, []byte(v))
	default:
		return dst
	}
}

func payloadLength(slices [][]byte) int {
	total := 0
	for _, data := range slices {
		total += len(data)
	}
	return total
}

func release(msg any) {
	if releasable, ok := msg.(interface{ Release() }); ok {
		releasable.Release()
	}
}
