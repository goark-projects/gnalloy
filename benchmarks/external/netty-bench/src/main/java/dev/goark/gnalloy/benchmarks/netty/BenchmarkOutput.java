package dev.goark.gnalloy.benchmarks.netty;

final class BenchmarkOutput {
    private BenchmarkOutput() {
    }

    static void write(Config config, BenchmarkResult result) {
        System.out.printf(
                "framework=netty protocol=%s backend=%s eventLoops=%d tlsVersion=%s cipherSuites=%s negotiatedProtocol=%s latencySampleRate=%d warmupMessages=%d latencySamples=%d p50LatencyNs=%d p95LatencyNs=%d p99LatencyNs=%d p999LatencyNs=%d maxLatencyNs=%d rssBytes=%d heapAllocBytes=%d heapSysBytes=%d heapObjects=%d gcCount=%d gcPauseNs=%d goroutines=%d payload=%d connections=%d messages=%d total=%d errors=%d elapsed=%s throughput=%.2f ops/s%n",
                config.protocol(),
                config.backend().wireName(),
                config.eventLoops(),
                config.tlsVersion().id(),
                config.cipherSuiteOutput(),
                result.negotiatedProtocol(),
                config.latencySampleRate(),
                config.warmupMessages(),
                result.latency().samples(),
                result.latency().p50LatencyNanos(),
                result.latency().p95LatencyNanos(),
                result.latency().p99LatencyNanos(),
                result.latency().p999LatencyNanos(),
                result.latency().maxLatencyNanos(),
                result.resources().rssBytes(),
                result.resources().heapAllocBytes(),
                result.resources().heapSysBytes(),
                result.resources().heapObjects(),
                result.resources().gcCount(),
                result.resources().gcPauseNanos(),
                result.resources().goroutines(),
                config.payload(),
                config.connections(),
                config.messages(),
                result.totalRequests(),
                result.errors(),
                result.elapsed(),
                result.throughput());
        System.out.printf(
                "%s-%d %d %.0f ns/op%n",
                benchmarkName(config.protocol()),
                Runtime.getRuntime().availableProcessors(),
                result.totalRequests(),
                result.nsPerOp());
    }

    private static String benchmarkName(String protocol) {
        if ("http1".equals(protocol)) {
            return "BenchmarkNettyHTTP1";
        }
        if ("https1".equals(protocol)) {
            return "BenchmarkNettyHTTPS1";
        }
        if ("http2".equals(protocol)) {
            return "BenchmarkNettyHTTP2";
        }
        if ("https2".equals(protocol)) {
            return "BenchmarkNettyHTTPS2";
        }
        if ("http3".equals(protocol)) {
            return "BenchmarkNettyHTTP3";
        }
        if ("udp-echo".equals(protocol)) {
            return "BenchmarkNettyUDPEcho";
        }
        return "BenchmarkNettyTCPEcho";
    }
}
