package dev.goark.gnalloy.benchmarks.netty;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.time.Duration;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicReference;

final class Http3LoadGenerator {
    private Http3LoadGenerator() {
    }

    static BenchmarkResult run(InetSocketAddress address, Config config) throws Exception {
        try (DatagramEventLoopResources resources = DatagramEventLoopResources.create(config)) {
            Http3Client[] clients = Http3ClientBootstrap.prepareClients(address, config, resources);
            try {
                return runClients(config, clients);
            } finally {
                closeClients(clients);
            }
        }
    }

    private static BenchmarkResult runClients(Config config, Http3Client[] clients) throws Exception {
        AtomicLong successes = new AtomicLong();
        AtomicLong errors = new AtomicLong();
        AtomicReference<Throwable> firstError = new AtomicReference<>();
        long[][] latencySamples = LatencyRecorder.samplingEnabled(config.latencySampleRate())
                ? new long[config.connections()][]
                : new long[0][];
        runWarmup(clients, config);
        ExecutorService pool = Executors.newFixedThreadPool(config.connections());
        ResourceSnapshot resourcesBefore = ResourceSnapshot.capture();
        long started = System.nanoTime();
        for (int i = 0; i < config.connections(); i++) {
            final int clientId = i;
            pool.execute(() -> {
                try {
                    long[] clientSamples = LatencyRecorder.newSamples(config.messages(), config.latencySampleRate());
                    if (clientSamples.length > 0) {
                        latencySamples[clientId] = clientSamples;
                    }
                    runClientMessages(config, clients[clientId], config.messages(), successes, clientSamples);
                } catch (Throwable t) {
                    errors.incrementAndGet();
                    firstError.compareAndSet(null, t);
                }
            });
        }
        pool.shutdown();
        boolean finished = pool.awaitTermination(config.timeout().toNanos(), TimeUnit.NANOSECONDS);
        long elapsedNanos = System.nanoTime() - started;
        if (!finished) {
            pool.shutdownNow();
            errors.incrementAndGet();
            firstError.compareAndSet(null, new IOException("netty-bench: timeout"));
        }
        long total = successes.get();
        BenchmarkResult result = result(total, errors.get(), elapsedNanos,
                LatencyRecorder.summarize(latencySamples),
                firstNegotiatedProtocol(clients),
                resourcesBefore.delta(ResourceSnapshot.capture()));
        rethrowFirstError(firstError.get());
        long expected = (long) config.connections() * config.messages();
        if (total != expected) {
            throw new IOException("netty-bench: completed " + total + " requests, want " + expected);
        }
        return result;
    }

    private static void runWarmup(Http3Client[] clients, Config config) throws Exception {
        if (config.warmupMessages() <= 0) {
            return;
        }
        AtomicReference<Throwable> firstError = new AtomicReference<>();
        ExecutorService pool = Executors.newFixedThreadPool(config.connections());
        for (Http3Client client : clients) {
            pool.execute(() -> {
                try {
                    runClientMessages(config, client, config.warmupMessages(), null, new long[0]);
                } catch (Throwable t) {
                    firstError.compareAndSet(null, t);
                }
            });
        }
        pool.shutdown();
        boolean finished = pool.awaitTermination(config.timeout().toNanos(), TimeUnit.NANOSECONDS);
        if (!finished) {
            pool.shutdownNow();
            firstError.compareAndSet(null, new IOException("netty-bench: warmup timeout"));
        }
        rethrowFirstError(firstError.get());
    }

    private static void runClientMessages(
            Config config,
            Http3Client client,
            int messageCount,
            AtomicLong successes,
            long[] latencySamples) throws Exception {
        int sampleIndex = 0;
        for (int i = 0; i < messageCount; i++) {
            boolean recordLatency = latencySamples.length > 0
                    && LatencyRecorder.shouldRecord(i, config.latencySampleRate());
            long requestStarted = recordLatency ? System.nanoTime() : 0L;
            client.request(config.timeout());
            if (recordLatency && sampleIndex < latencySamples.length) {
                latencySamples[sampleIndex++] = LatencyRecorder.elapsedNanos(requestStarted);
            }
            if (successes != null) {
                successes.incrementAndGet();
            }
        }
    }

    private static BenchmarkResult result(
            long total,
            long errors,
            long elapsedNanos,
            LatencySummary latency,
            String negotiatedProtocol,
            ResourceDelta resources) {
        double throughput = elapsedNanos > 0 ? total * 1_000_000_000.0 / elapsedNanos : 0.0;
        double nsPerOp = total > 0 ? (double) elapsedNanos / total : 0.0;
        return new BenchmarkResult(total, errors, Duration.ofNanos(elapsedNanos), throughput, nsPerOp, negotiatedProtocol, latency, resources);
    }

    private static String firstNegotiatedProtocol(Http3Client[] clients) {
        for (Http3Client client : clients) {
            if (client != null && !client.negotiatedProtocol().isEmpty()) {
                return client.negotiatedProtocol();
            }
        }
        return "";
    }

    private static void rethrowFirstError(Throwable failure) throws Exception {
        if (failure == null) {
            return;
        }
        if (failure instanceof Exception exception) {
            throw exception;
        }
        throw new RuntimeException(failure);
    }

    static void closeClients(Http3Client[] clients) {
        for (Http3Client client : clients) {
            if (client == null) {
                continue;
            }
            try {
                client.close();
            } catch (Exception ignored) {
            }
        }
    }

}
