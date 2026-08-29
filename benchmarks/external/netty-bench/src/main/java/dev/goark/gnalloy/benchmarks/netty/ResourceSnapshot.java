package dev.goark.gnalloy.benchmarks.netty;

import java.lang.management.GarbageCollectorMXBean;
import java.lang.management.ManagementFactory;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Locale;

record ResourceDelta(
        long rssBytes,
        long heapAllocBytes,
        long heapSysBytes,
        long heapObjects,
        long gcCount,
        long gcPauseNanos,
        long goroutines) {
}

record ResourceSnapshot(
        long rssBytes,
        long heapAllocBytes,
        long heapSysBytes,
        long heapObjects,
        long gcCount,
        long gcPauseNanos,
        long goroutines) {

    static ResourceSnapshot capture() {
        Runtime runtime = Runtime.getRuntime();
        return new ResourceSnapshot(
                currentRSSBytes(),
                runtime.totalMemory() - runtime.freeMemory(),
                runtime.totalMemory(),
                0L,
                currentGCCount(),
                currentGCPauseNanos(),
                0L);
    }

    ResourceDelta delta(ResourceSnapshot end) {
        return new ResourceDelta(
                end.rssBytes(),
                end.heapAllocBytes(),
                end.heapSysBytes(),
                end.heapObjects(),
                Math.max(0L, end.gcCount() - gcCount()),
                Math.max(0L, end.gcPauseNanos() - gcPauseNanos()),
                end.goroutines());
    }

    private static long currentGCCount() {
        long total = 0L;
        for (GarbageCollectorMXBean bean : ManagementFactory.getGarbageCollectorMXBeans()) {
            long count = bean.getCollectionCount();
            if (count > 0) {
                total += count;
            }
        }
        return total;
    }

    private static long currentGCPauseNanos() {
        long totalMillis = 0L;
        for (GarbageCollectorMXBean bean : ManagementFactory.getGarbageCollectorMXBeans()) {
            long millis = bean.getCollectionTime();
            if (millis > 0) {
                totalMillis += millis;
            }
        }
        return totalMillis * 1_000_000L;
    }

    private static long currentRSSBytes() {
        if (!System.getProperty("os.name", "").toLowerCase(Locale.ROOT).startsWith("linux")) {
            return 0L;
        }
        try {
            String statm = Files.readString(Path.of("/proc/self/statm"));
            String[] fields = statm.strip().split("\\s+");
            if (fields.length < 2) {
                return 0L;
            }
            return Long.parseLong(fields[1]) * 4096L;
        } catch (RuntimeException | java.io.IOException ignored) {
            return 0L;
        }
    }
}
