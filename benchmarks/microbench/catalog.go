package microbench

import (
	"regexp"
	"sort"
	"strings"
)

// Scenario 描述一个 Go benchmark 包及其稳定入口。
type Scenario struct {
	Name       string
	Package    string
	Benchmarks []string
	Tags       []string
}

// Suite 是一组可重复运行的 microbenchmark 场景。
type Suite struct {
	Name        string
	Description string
	Scenarios   []Scenario
}

var catalog = []Suite{
	{
		Name:        "hotpath",
		Description: "ByteBuf、Pipeline、codec、timer、queue 和观测热路径。",
		Scenarios: []Scenario{
			{Name: "buffer", Package: "./buffer", Benchmarks: []string{
				"BenchmarkHeapAllocatorAcquireRelease",
				"BenchmarkPooledAllocatorAcquireRelease",
				"BenchmarkPooledAllocatorParallelAcquireRelease",
				"BenchmarkMmapAllocatorAcquireRelease",
				"BenchmarkCompositeReadableSlicesFullComponents",
				"BenchmarkCompositeGetByteFragmented",
				"BenchmarkCompositeIndexByteFragmented",
				"BenchmarkCopyReadableBytesComposite",
				"BenchmarkWriteReadableBytesComposite",
			}, Tags: []string{"buffer", "allocator"}},
			{Name: "channel", Package: "./channel", Benchmarks: []string{
				"BenchmarkPipelineInboundNoop",
				"BenchmarkPipelineWriteAndFlushDirectSink",
				"BenchmarkUnsafeWriteAndFlushDrained",
				"BenchmarkUnsafeFileRegionDirectWriterDrained",
				"BenchmarkUnsafeVectorWriteAndFlushSingleDirectBufferDrained",
				"BenchmarkFileRegionEncoderChunks",
			}, Tags: []string{"pipeline", "write"}},
			{Name: "channel-pool", Package: "./channel/pool", Benchmarks: []string{
				"BenchmarkFixedPoolGetPut",
				"BenchmarkChannelPoolMapGet",
			}, Tags: []string{"pool"}},
			{Name: "codec-core", Package: "./codec", Benchmarks: []string{
				"BenchmarkFixedLengthFrameDecoder",
				"BenchmarkLineBasedFrameDecoder",
				"BenchmarkLineBasedFrameDecoderFragmented",
				"BenchmarkDelimiterBasedFrameDecoder",
				"BenchmarkDelimiterBasedFrameDecoderFragmented",
				"BenchmarkByteToMessageListDecoder",
				"BenchmarkByteSliceDecoderComposite",
				"BenchmarkStringDecoderComposite",
				"BenchmarkBase64EncoderComposite",
				"BenchmarkBase64DecoderComposite",
				"BenchmarkLengthFieldDecoder",
			}, Tags: []string{"codec", "frame"}},
			{Name: "protocol-codec", Package: "./codec/http1", Benchmarks: []string{
				"BenchmarkFindHeaderEndFragmented",
				"BenchmarkStringSliceFragmented",
				"BenchmarkRequestDecoderFragmentedHeader",
				"BenchmarkRequestDecoderChunkedFragmentedBody",
			}, Tags: []string{"http1"}},
			{Name: "http2", Package: "./codec/http2", Benchmarks: []string{"BenchmarkStreamMultiplexerReadData"}, Tags: []string{"http2"}},
			{Name: "http3", Package: "./codec/http3", Benchmarks: []string{
				"BenchmarkDecoderDataFrame",
				"BenchmarkHeaderDecoderFragmentedBlock",
			}, Tags: []string{"http3"}},
			{Name: "websocket", Package: "./codec/websocket", Benchmarks: []string{
				"BenchmarkFrameEncoderMaskedCompositePayload",
				"BenchmarkFrameDecoderMaskedFragmentedPayload",
			}, Tags: []string{"websocket"}},
			{Name: "tls", Package: "./handler/tls", Benchmarks: []string{"BenchmarkCopyReadableBytesComposite"}, Tags: []string{"tls"}},
			{Name: "runtime", Package: "./transport", Benchmarks: []string{"BenchmarkEventLoopSubmitBurst"}, Tags: []string{"eventloop"}},
			{Name: "queue", Package: "./queue", Benchmarks: []string{"BenchmarkMPSCOfferPoll"}, Tags: []string{"queue"}},
			{Name: "timer", Package: "./timer", Benchmarks: []string{"BenchmarkWheelScheduleAdvance"}, Tags: []string{"timer"}},
			{Name: "observability", Package: "./observability", Benchmarks: []string{
				"BenchmarkAtomicChannelRecorderRead",
				"BenchmarkPrometheusExporter",
			}, Tags: []string{"observability"}},
			{Name: "handlers", Package: "./handler/ipfilter", Benchmarks: []string{"BenchmarkIPFilterAllowedDatagram"}, Tags: []string{"handler"}},
			{Name: "pcap", Package: "./handler/pcap", Benchmarks: []string{"BenchmarkPCAPCaptureByteBuf"}, Tags: []string{"handler"}},
		},
	},
	{
		Name:        "native-io",
		Description: "平台原生 I/O microbenchmark，部分包只在目标 GOOS 可用。",
		Scenarios: []Scenario{
			{Name: "tcp", Package: "./transport/tcp", Benchmarks: []string{
				"BenchmarkNativeTCPEchoRoundTrip",
				"BenchmarkLengthFieldTCPRoundTrip",
			}, Tags: []string{"tcp", "transport"}},
			{Name: "udp", Package: "./transport/udp", Benchmarks: []string{"BenchmarkEndpointBackpressureQueue"}, Tags: []string{"udp", "transport"}},
			{Name: "raw", Package: "./transport/raw", Benchmarks: []string{"BenchmarkEndpointBackpressureQueue"}, Tags: []string{"raw", "transport"}},
			{Name: "iouring", Package: "./transport/poller/iouring", Benchmarks: []string{"BenchmarkMakeIOVectorsComposite"}, Tags: []string{"linux", "io_uring"}},
			{Name: "iocp", Package: "./transport/poller/iocp", Benchmarks: []string{"BenchmarkMakeWriteBuffersComposite"}, Tags: []string{"windows", "iocp"}},
		},
	},
}

// Suites 返回全部 microbenchmark 套件副本。
func Suites() []Suite {
	out := make([]Suite, len(catalog))
	for i := range catalog {
		out[i] = cloneSuite(catalog[i])
	}
	return out
}

// Lookup 按名称查找 microbenchmark 套件。
func Lookup(name string) (Suite, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, suite := range catalog {
		if suite.Name == name {
			return cloneSuite(suite), true
		}
	}
	return Suite{}, false
}

// Packages 返回套件涉及的去重包路径。
func (s Suite) Packages() []string {
	seen := make(map[string]struct{}, len(s.Scenarios))
	out := make([]string, 0, len(s.Scenarios))
	for _, scenario := range s.Scenarios {
		pkg := strings.TrimSpace(scenario.Package)
		if pkg == "" {
			continue
		}
		if _, ok := seen[pkg]; ok {
			continue
		}
		seen[pkg] = struct{}{}
		out = append(out, pkg)
	}
	sort.Strings(out)
	return out
}

// BenchmarkRegexp 返回可直接传给 go test -bench 的稳定正则。
func (s Suite) BenchmarkRegexp() string {
	names := s.benchmarkNames()
	if len(names) == 0 {
		return "."
	}
	quoted := make([]string, len(names))
	for i := range names {
		quoted[i] = regexp.QuoteMeta(names[i])
	}
	return "^(" + strings.Join(quoted, "|") + ")$"
}

func (s Suite) benchmarkNames() []string {
	seen := make(map[string]struct{}, len(s.Scenarios))
	out := make([]string, 0, len(s.Scenarios))
	for _, scenario := range s.Scenarios {
		for _, name := range scenario.Benchmarks {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func cloneSuite(in Suite) Suite {
	out := Suite{
		Name:        in.Name,
		Description: in.Description,
		Scenarios:   make([]Scenario, len(in.Scenarios)),
	}
	for i := range in.Scenarios {
		out.Scenarios[i] = cloneScenario(in.Scenarios[i])
	}
	return out
}

func cloneScenario(in Scenario) Scenario {
	return Scenario{
		Name:       in.Name,
		Package:    in.Package,
		Benchmarks: append([]string(nil), in.Benchmarks...),
		Tags:       append([]string(nil), in.Tags...),
	}
}
