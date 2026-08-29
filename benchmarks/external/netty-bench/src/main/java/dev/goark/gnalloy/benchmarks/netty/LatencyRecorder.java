package dev.goark.gnalloy.benchmarks.netty;

import java.util.Arrays;

final class LatencyRecorder {
    private LatencyRecorder() {
    }

    static boolean samplingEnabled(int rate) {
        return rate > 0;
    }

    static boolean shouldRecord(int messageIndex, int rate) {
        return rate > 0 && messageIndex % rate == 0;
    }

    static long elapsedNanos(long started) {
        long elapsed = System.nanoTime() - started;
        return elapsed > 0 ? elapsed : 1L;
    }

    static long[] newSamples(int messages, int rate) {
        if (!samplingEnabled(rate)) {
            return new long[0];
        }
        int capacity = messages / rate;
        if (messages % rate != 0) {
            capacity++;
        }
        return new long[Math.max(capacity, 1)];
    }

    static LatencySummary summarize(long[][] samples) {
        int count = 0;
        for (long[] clientSamples : samples) {
            if (clientSamples != null) {
                count += clientSamples.length;
            }
        }
        if (count == 0) {
            return LatencySummary.empty();
        }
        long[] merged = new long[count];
        int offset = 0;
        for (long[] clientSamples : samples) {
            if (clientSamples == null) {
                continue;
            }
            System.arraycopy(clientSamples, 0, merged, offset, clientSamples.length);
            offset += clientSamples.length;
        }
        Arrays.sort(merged);
        return new LatencySummary(
                merged.length,
                percentile(merged, 0.50),
                percentile(merged, 0.95),
                percentile(merged, 0.99),
                percentile(merged, 0.999),
                merged[merged.length - 1]);
    }

    private static long percentile(long[] sorted, double q) {
        if (sorted.length == 0) {
            return 0L;
        }
        if (q <= 0.0) {
            return sorted[0];
        }
        if (q >= 1.0) {
            return sorted[sorted.length - 1];
        }
        int index = (int) ((sorted.length - 1) * q);
        return sorted[index];
    }
}
