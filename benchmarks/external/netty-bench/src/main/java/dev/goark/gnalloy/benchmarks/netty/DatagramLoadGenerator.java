package dev.goark.gnalloy.benchmarks.netty;

import java.io.IOException;
import java.net.DatagramPacket;
import java.net.DatagramSocket;
import java.net.InetSocketAddress;
import java.time.Duration;
import java.util.Arrays;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicReference;

final class DatagramLoadGenerator {
    private DatagramLoadGenerator() {
    }

    static BenchmarkResult run(InetSocketAddress address, Config config) throws Exception {
        AtomicLong successes = new AtomicLong();
        AtomicLong errors = new AtomicLong();
        AtomicReference<Throwable> firstError = new AtomicReference<>();
        long[][] latencySamples = LatencyRecorder.samplingEnabled(config.latencySampleRate())
                ? new long[config.connections()][]
                : new long[0][];
        DatagramClientSession[] clients = prepareClients(address, config);
        try {
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
                        runClientMessages(config, clientId, clients[clientId], config.messages(), successes, clientSamples);
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
                    resourcesBefore.delta(ResourceSnapshot.capture()));
            rethrowFirstError(firstError.get());
            long expected = (long) config.connections() * config.messages();
            if (total != expected) {
                throw new IOException("netty-bench: completed " + total + " requests, want " + expected);
            }
            return result;
        } finally {
            closeClients(clients);
        }
    }

    private static DatagramClientSession[] prepareClients(InetSocketAddress address, Config config) throws IOException {
        DatagramClientSession[] clients = new DatagramClientSession[config.connections()];
        try {
            for (int i = 0; i < clients.length; i++) {
                DatagramSocket socket = new DatagramSocket();
                socket.connect(address);
                socket.setSoTimeout(Math.toIntExact(config.timeout().toMillis()));
                clients[i] = new DatagramClientSession(socket, makePayload(config.payload(), i), new byte[config.payload()]);
            }
            return clients;
        } catch (IOException | RuntimeException failure) {
            closeClients(clients);
            throw failure;
        }
    }

    private static void runWarmup(DatagramClientSession[] clients, Config config) throws Exception {
        if (config.warmupMessages() <= 0) {
            return;
        }
        AtomicReference<Throwable> firstError = new AtomicReference<>();
        ExecutorService pool = Executors.newFixedThreadPool(config.connections());
        for (int i = 0; i < clients.length; i++) {
            final int clientId = i;
            pool.execute(() -> {
                try {
                    runClientMessages(config, clientId, clients[clientId], config.warmupMessages(), null, new long[0]);
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
            int clientId,
            DatagramClientSession client,
            int messageCount,
            AtomicLong successes,
            long[] latencySamples) throws IOException {
        int sampleIndex = 0;
        for (int i = 0; i < messageCount; i++) {
            byte[] payload = client.payload();
            payload[0] = (byte) (clientId + i);
            boolean recordLatency = latencySamples.length > 0
                    && LatencyRecorder.shouldRecord(i, config.latencySampleRate());
            long requestStarted = recordLatency ? System.nanoTime() : 0L;
            DatagramPacket request = new DatagramPacket(payload, payload.length);
            DatagramPacket reply = new DatagramPacket(client.reply(), client.reply().length);
            client.socket().send(request);
            client.socket().receive(reply);
            if (reply.getLength() != payload.length || !Arrays.equals(client.reply(), payload)) {
                throw new IOException("netty-bench: udp echo mismatch");
            }
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
            ResourceDelta resources) {
        double throughput = elapsedNanos > 0 ? total * 1_000_000_000.0 / elapsedNanos : 0.0;
        double nsPerOp = total > 0 ? (double) elapsedNanos / total : 0.0;
        return new BenchmarkResult(total, errors, Duration.ofNanos(elapsedNanos), throughput, nsPerOp, "", latency, resources);
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

    private static byte[] makePayload(int size, int clientId) {
        byte[] payload = new byte[size];
        for (int i = 0; i < payload.length; i++) {
            payload[i] = (byte) (clientId + i);
        }
        return payload;
    }

    private static void closeClients(DatagramClientSession[] clients) {
        for (DatagramClientSession client : clients) {
            if (client == null) {
                continue;
            }
            client.close();
        }
    }

    private record DatagramClientSession(DatagramSocket socket, byte[] payload, byte[] reply) implements AutoCloseable {
        @Override
        public void close() {
            socket.close();
        }
    }
}
