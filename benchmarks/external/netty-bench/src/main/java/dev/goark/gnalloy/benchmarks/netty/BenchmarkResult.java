package dev.goark.gnalloy.benchmarks.netty;

import java.time.Duration;

record BenchmarkResult(
        long totalRequests,
        long errors,
        Duration elapsed,
        double throughput,
        double nsPerOp,
        String negotiatedProtocol,
        LatencySummary latency,
        ResourceDelta resources) {
}
