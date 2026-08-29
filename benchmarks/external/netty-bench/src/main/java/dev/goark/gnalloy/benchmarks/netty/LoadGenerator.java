package dev.goark.gnalloy.benchmarks.netty;

import java.io.EOFException;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.time.Duration;
import java.util.Arrays;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicReference;

final class LoadGenerator {
    private LoadGenerator() {
    }

    static BenchmarkResult run(InetSocketAddress address, Config config) throws Exception {
        AtomicLong successes = new AtomicLong();
        AtomicLong errors = new AtomicLong();
        AtomicReference<Throwable> firstError = new AtomicReference<>();
        long[][] latencySamples = LatencyRecorder.samplingEnabled(config.latencySampleRate())
                ? new long[config.connections()][]
                : new long[0][];
        ClientSession[] clients = prepareClients(address, config);
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
                        runClient(config, clientId, clients[clientId], successes, clientSamples);
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

    private static void runWarmup(ClientSession[] clients, Config config) throws Exception {
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

    private static ClientSession[] prepareClients(InetSocketAddress address, Config config) throws IOException {
        ClientSession[] clients = new ClientSession[config.connections()];
        try {
            for (int i = 0; i < clients.length; i++) {
                Socket socket = new Socket();
                socket.setTcpNoDelay(true);
                socket.connect(address, Math.toIntExact(config.timeout().toMillis()));
                socket.setSoTimeout(Math.toIntExact(config.timeout().toMillis()));
                clients[i] = new ClientSession(socket, makePayload(config.payload(), i), new byte[config.payload()]);
            }
            return clients;
        } catch (IOException | RuntimeException failure) {
            closeClients(clients);
            throw failure;
        }
    }

    private static void closeClients(ClientSession[] clients) {
        for (ClientSession client : clients) {
            if (client == null) {
                continue;
            }
            try {
                client.close();
            } catch (IOException ignored) {
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

    private static void runClient(
            Config config,
            int clientId,
            ClientSession client,
            AtomicLong successes,
            long[] latencySamples) throws IOException {
        runClientMessages(config, clientId, client, config.messages(), successes, latencySamples);
    }

    private static void runClientMessages(
            Config config,
            int clientId,
            ClientSession client,
            int messageCount,
            AtomicLong successes,
            long[] latencySamples) throws IOException {
        byte[] payload = client.payload();
        byte[] reply = client.reply();
        OutputStream out = client.socket().getOutputStream();
        InputStream in = client.socket().getInputStream();
        int sampleIndex = 0;
        for (int i = 0; i < messageCount; i++) {
            payload[0] = (byte) (clientId + i);
            boolean recordLatency = latencySamples.length > 0
                    && LatencyRecorder.shouldRecord(i, config.latencySampleRate());
            long requestStarted = recordLatency ? System.nanoTime() : 0L;
            out.write(payload);
            readFully(in, reply);
            if (!Arrays.equals(reply, payload)) {
                throw new IOException("netty-bench: echo mismatch");
            }
            if (recordLatency && sampleIndex < latencySamples.length) {
                latencySamples[sampleIndex++] = LatencyRecorder.elapsedNanos(requestStarted);
            }
            if (successes != null) {
                successes.incrementAndGet();
            }
        }
    }

    private static byte[] makePayload(int size, int clientId) {
        byte[] payload = new byte[size];
        for (int i = 0; i < payload.length; i++) {
            payload[i] = (byte) (clientId + i);
        }
        return payload;
    }

    private static void readFully(InputStream in, byte[] dst) throws IOException {
        int off = 0;
        while (off < dst.length) {
            int n = in.read(dst, off, dst.length - off);
            if (n < 0) {
                throw new EOFException("netty-bench: connection closed");
            }
            off += n;
        }
    }
}
