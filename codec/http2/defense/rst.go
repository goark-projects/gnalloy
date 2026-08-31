package defense

import (
	"time"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/codec/http2"
)

const defaultRSTWindow = time.Second

// MaxRstFrameConfig 描述 RST_STREAM flood 防护窗口。
type MaxRstFrameConfig struct {
	// MaxFrames 是窗口内允许的最大 RST_STREAM 数，0 表示不限制。
	MaxFrames int
	// Window 是计数窗口，0 使用 1 秒。
	Window time.Duration
}

// MaxRstFrameDecoder 限制单位时间内入站 RST_STREAM 帧数量。
type MaxRstFrameDecoder struct {
	cfg    MaxRstFrameConfig
	now    func() time.Time
	events []time.Time
	head   int
}

// NewMaxRstFrameDecoder 创建 RST_STREAM flood 防护 handler。
func NewMaxRstFrameDecoder(cfg MaxRstFrameConfig) *MaxRstFrameDecoder {
	if cfg.Window <= 0 {
		cfg.Window = defaultRSTWindow
	}
	return &MaxRstFrameDecoder{cfg: cfg, now: time.Now}
}

// Count 返回当前窗口内仍有效的 RST_STREAM 数量。
func (d *MaxRstFrameDecoder) Count() int {
	if d == nil {
		return 0
	}
	d.expire(d.now())
	return len(d.events) - d.head
}

// ChannelRead 在 RST_STREAM 超出窗口预算时触发异常并阻断该帧。
func (d *MaxRstFrameDecoder) ChannelRead(ctx *channel.HandlerContext, msg any) {
	if _, ok := msg.(http2.RSTStreamFrame); !ok || d == nil || d.cfg.MaxFrames <= 0 {
		ctx.FireChannelRead(msg)
		return
	}
	now := d.now()
	d.expire(now)
	if len(d.events)-d.head >= d.cfg.MaxFrames {
		ctx.FireExceptionCaught(ErrTooManyRSTFrames)
		return
	}
	d.events = append(d.events, now)
	ctx.FireChannelRead(msg)
}

func (d *MaxRstFrameDecoder) expire(now time.Time) {
	if d == nil || len(d.events) == d.head {
		return
	}
	cutoff := now.Add(-d.cfg.Window)
	for d.head < len(d.events) && !d.events[d.head].After(cutoff) {
		d.events[d.head] = time.Time{}
		d.head++
	}
	if d.head == len(d.events) {
		d.events = d.events[:0]
		d.head = 0
		return
	}
	if d.head > len(d.events)/2 {
		copy(d.events, d.events[d.head:])
		tail := len(d.events) - d.head
		for i := tail; i < len(d.events); i++ {
			d.events[i] = time.Time{}
		}
		d.events = d.events[:tail]
		d.head = 0
	}
}
