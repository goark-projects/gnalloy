package dev.goark.gnalloy.benchmarks.netty;

record LatencySummary(
        long samples,
        long p50LatencyNanos,
        long p95LatencyNanos,
        long p99LatencyNanos,
        long p999LatencyNanos,
        long maxLatencyNanos) {

    static LatencySummary empty() {
        return new LatencySummary(0L, 0L, 0L, 0L, 0L, 0L);
    }
}
