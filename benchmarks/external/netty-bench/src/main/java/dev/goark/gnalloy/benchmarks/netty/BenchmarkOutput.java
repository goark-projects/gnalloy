package dev.goark.gnalloy.benchmarks.netty;

final class BenchmarkOutput {
    private static final String BENCHMARK_NAME = "BenchmarkNettyTCPEcho";

    private BenchmarkOutput() {
    }

    static void write(Config config, BenchmarkResult result) {
        System.out.printf(
                "framework=netty protocol=%s backend=%s eventLoops=%d payload=%d connections=%d messages=%d total=%d errors=%d elapsed=%s throughput=%.2f ops/s%n",
                config.protocol(),
                config.backend().wireName(),
                config.eventLoops(),
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
