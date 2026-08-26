package tcp

import (
	"testing"

	"goark.dev/gnalloy/channel"
	"goark.dev/gnalloy/transport"
)

func TestNormalizeConfigCarriesWriteBufferWatermark(t *testing.T) {
	cfg := normalizeConfig(Config{WriteBufferWatermark: transport.WriteBufferWatermark{Low: 3, High: 9}})
	if cfg.WriteBufferWatermark.Low != 3 || cfg.WriteBufferWatermark.High != 9 {
		t.Fatalf("watermark=%+v", cfg.WriteBufferWatermark)
	}

	cfg = normalizeConfig(Config{WriteBufferWatermark: transport.WriteBufferWatermark{Low: 10, High: 9}})
	if cfg.WriteBufferWatermark.Low != 4 || cfg.WriteBufferWatermark.High != 9 {
		t.Fatalf("normalized watermark=%+v", cfg.WriteBufferWatermark)
	}
}

func TestNormalizeConfigFillsNettyStyleSocketDefaults(t *testing.T) {
	cfg := normalizeConfig(Config{})
	if cfg.Backlog != defaultBacklog {
		t.Fatalf("backlog=%d, want %d", cfg.Backlog, defaultBacklog)
	}
	if !cfg.ReuseAddr {
		t.Fatal("reuse addr should default to true")
	}
	if !cfg.NoDelay {
		t.Fatal("tcp no delay should default to true")
	}
	if cfg.ReadBufferSize != defaultReadBufferSize {
		t.Fatalf("read buffer size=%d, want %d", cfg.ReadBufferSize, defaultReadBufferSize)
	}
	if cfg.ConnectTimeoutMillis != defaultConnectTimeoutMillis {
		t.Fatalf("connect timeout=%d, want %d", cfg.ConnectTimeoutMillis, defaultConnectTimeoutMillis)
	}
	if cfg.SoLinger != -1 {
		t.Fatalf("so linger=%d, want disabled", cfg.SoLinger)
	}
}

func TestSocketOptionsApplyListenChannelOptions(t *testing.T) {
	options := channel.NewChannelOptions()
	options.Apply(
		channel.OptionSoBacklog.Assignment(2048),
		channel.OptionSoReuseAddr.Assignment(false),
		channel.OptionSoReusePort.Assignment(true),
		channel.OptionSoSndBuf.Assignment(65536),
		channel.OptionSoRcvBuf.Assignment(32768),
	)

	got := normalizeConfig(Config{}).socketOptions().withListenOptions(options)
	if got.backlog != 2048 {
		t.Fatalf("backlog=%d", got.backlog)
	}
	if got.reuseAddr {
		t.Fatal("reuse addr should be disabled by explicit option")
	}
	if !got.reusePort {
		t.Fatal("reuse port should be enabled by explicit option")
	}
	if got.sendBufferSize != 65536 || got.receiveBufferSize != 32768 {
		t.Fatalf("socket buffers send=%d recv=%d", got.sendBufferSize, got.receiveBufferSize)
	}
}

func TestSocketOptionsApplyChildChannelOptions(t *testing.T) {
	options := channel.NewChannelOptions()
	watermark := transport.WriteBufferWatermark{Low: 8192, High: 4096}
	options.Apply(
		channel.OptionTcpNoDelay.Assignment(false),
		channel.OptionSoKeepAlive.Assignment(true),
		channel.OptionSoSndBuf.Assignment(16384),
		channel.OptionSoRcvBuf.Assignment(8192),
		channel.OptionSoLinger.Assignment(0),
		channel.OptionReadBufferSize.Assignment(2048),
		channel.OptionWriteBufferWatermark.Assignment(watermark),
	)

	got := normalizeConfig(Config{}).socketOptions().withChildOptions(options)
	if got.noDelay {
		t.Fatal("tcp no delay should be disabled by explicit option")
	}
	if !got.keepAlive {
		t.Fatal("keepalive should be enabled by explicit option")
	}
	if got.sendBufferSize != 16384 || got.receiveBufferSize != 8192 {
		t.Fatalf("socket buffers send=%d recv=%d", got.sendBufferSize, got.receiveBufferSize)
	}
	if got.soLinger != 0 {
		t.Fatalf("so linger=%d, want explicit zero", got.soLinger)
	}
	if got.readBufferSize != 2048 {
		t.Fatalf("read buffer size=%d", got.readBufferSize)
	}
	if got.writeBufferWatermark.Low != 2048 || got.writeBufferWatermark.High != 4096 {
		t.Fatalf("watermark=%+v", got.writeBufferWatermark)
	}
}

func TestSocketOptionsApplyClientConnectTimeout(t *testing.T) {
	options := channel.NewChannelOptions()
	options.Apply(channel.OptionConnectTimeoutMillis.Assignment(0))
	got := normalizeConfig(Config{}).socketOptions().withClientOptions(options)
	if got.connectTimeoutMillis != 0 {
		t.Fatalf("connect timeout=%d, want disabled", got.connectTimeoutMillis)
	}

	options.Apply(channel.OptionConnectTimeoutMillis.Assignment(-1))
	got = normalizeConfig(Config{}).socketOptions().withClientOptions(options)
	if got.connectTimeoutMillis != 0 {
		t.Fatalf("negative connect timeout=%d, want disabled", got.connectTimeoutMillis)
	}
}
