package quic

const (
	defaultMaxDatagramSize = 1200
	defaultInitialWindow   = defaultMaxDatagramSize * 10
	defaultMinimumWindow   = defaultMaxDatagramSize * 2
)

// CongestionConfig 描述 QUIC 发送端拥塞控制参数。
type CongestionConfig struct {
	MaxDatagramSize int
	InitialWindow   int
	MinimumWindow   int
}

// CongestionController 是轻量 Reno 风格 QUIC 拥塞控制器。
type CongestionController struct {
	maxDatagramSize int
	minimumWindow   int
	window          int
	slowStart       int
	inFlight        int
}

// NewCongestionController 创建拥塞控制器。
func NewCongestionController(cfg CongestionConfig) (*CongestionController, error) {
	maxDatagram := cfg.MaxDatagramSize
	if maxDatagram == 0 {
		maxDatagram = defaultMaxDatagramSize
	}
	initial := cfg.InitialWindow
	if initial == 0 {
		initial = defaultInitialWindow
	}
	minimum := cfg.MinimumWindow
	if minimum == 0 {
		minimum = defaultMinimumWindow
	}
	if maxDatagram <= 0 || initial <= 0 || minimum <= 0 {
		return nil, ErrInvalidConfig
	}
	return &CongestionController{
		maxDatagramSize: maxDatagram,
		minimumWindow:   minimum,
		window:          initial,
		slowStart:       1 << 30,
	}, nil
}

// CanSend 判断当前拥塞窗口是否允许继续发送。
func (c *CongestionController) CanSend(bytes int) bool {
	if c == nil || bytes < 0 {
		return false
	}
	return c.inFlight+bytes <= c.window
}

// OnPacketSent 增加 bytes_in_flight。
func (c *CongestionController) OnPacketSent(bytes int) error {
	if c == nil || bytes < 0 {
		return ErrInvalidConfig
	}
	if !c.CanSend(bytes) {
		return ErrCongestionLimited
	}
	c.inFlight += bytes
	return nil
}

// OnPacketAcked 根据 ACK 推进拥塞窗口。
func (c *CongestionController) OnPacketAcked(bytes int) {
	if c == nil || bytes <= 0 {
		return
	}
	c.decreaseInFlight(bytes)
	if c.window < c.slowStart {
		c.window += bytes
		return
	}
	increment := c.maxDatagramSize * bytes / c.window
	if increment <= 0 {
		increment = 1
	}
	c.window += increment
}

// OnPacketLost 按 Reno 语义收缩拥塞窗口。
func (c *CongestionController) OnPacketLost(bytes int) {
	if c == nil {
		return
	}
	c.decreaseInFlight(bytes)
	reduced := c.window / 2
	if reduced < c.minimumWindow {
		reduced = c.minimumWindow
	}
	c.window = reduced
	c.slowStart = reduced
}

func (c *CongestionController) Window() int {
	if c == nil {
		return 0
	}
	return c.window
}

func (c *CongestionController) InFlight() int {
	if c == nil {
		return 0
	}
	return c.inFlight
}

func (c *CongestionController) decreaseInFlight(bytes int) {
	if bytes <= 0 {
		return
	}
	if bytes >= c.inFlight {
		c.inFlight = 0
		return
	}
	c.inFlight -= bytes
}
