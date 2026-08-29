package dev.goark.gnalloy.benchmarks.netty;

final class BenchmarkOutput {
    private static final String BENCHMARK_NAME = "BenchmarkNettyTCPEcho";

    private BenchmarkOutput() {
    }

    static void write(Config config, BenchmarkResult result) {
        System.out.printf(
                "framework=netty protocol=%s backend=%s eventLoops=%d latencySampleRate=%d latencySamples=%d p50LatencyNs=%d p95LatencyNs=%d p99LatencyNs=%d p999LatencyNs=%d maxLatencyNs=%d rssBytes=%d heapAllocBytes=%d heapSysBytes=%d heapObjects=%d gcCount=%d gcPauseNs=%d goroutines=%d payload=%d connections=%d messages=%d total=%d errors=%d elapsed=%s throughput=%.2f ops/s%n",
                config.protocol(),
                config.backend().wireName(),
                config.eventLoops(),
                config.latencySampleRate(),
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
                BENCHMARK_NAME,
                Runtime.getRuntime().availableProcessors(),
                result.totalRequests(),
                result.nsPerOp());
    }
}
