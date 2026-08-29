package dev.goark.gnalloy.benchmarks.netty;

import java.io.BufferedInputStream;
import java.io.ByteArrayOutputStream;
import java.io.EOFException;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.Arrays;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicReference;

final class Http1LoadGenerator {
    private Http1LoadGenerator() {
    }

    static BenchmarkResult run(InetSocketAddress address, Config config) throws Exception {
        AtomicLong successes = new AtomicLong();
        AtomicLong errors = new AtomicLong();
        AtomicReference<Throwable> firstError = new AtomicReference<>();
        long[][] latencySamples = LatencyRecorder.samplingEnabled(config.latencySampleRate())
                ? new long[config.connections()][]
                : new long[0][];
        Http1Session[] clients = prepareClients(address, config);
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
        } finally {
            closeClients(clients);
        }
    }

    private static Http1Session[] prepareClients(InetSocketAddress address, Config config) throws Exception {
        Http1Session[] clients = new Http1Session[config.connections()];
        try {
            for (int i = 0; i < clients.length; i++) {
                Socket socket = SslSupport.connect(address, config);
                clients[i] = new Http1Session(
                        socket,
                        new BufferedInputStream(socket.getInputStream(), 16 * 1024),
                        socket.getOutputStream(),
                        HttpPayload.request(config.host()),
                        HttpPayload.body(config.payload()),
                        new byte[config.payload()],
                        SslSupport.negotiatedProtocol(socket));
            }
            return clients;
        } catch (Exception failure) {
            closeClients(clients);
            throw failure;
        }
    }

    private static void runWarmup(Http1Session[] clients, Config config) throws Exception {
        if (config.warmupMessages() <= 0) {
            return;
        }
        AtomicReference<Throwable> firstError = new AtomicReference<>();
        ExecutorService pool = Executors.newFixedThreadPool(config.connections());
        for (Http1Session client : clients) {
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
            Http1Session client,
            int messageCount,
            AtomicLong successes,
            long[] latencySamples) throws IOException {
        int sampleIndex = 0;
        for (int i = 0; i < messageCount; i++) {
            boolean recordLatency = latencySamples.length > 0
                    && LatencyRecorder.shouldRecord(i, config.latencySampleRate());
            long requestStarted = recordLatency ? System.nanoTime() : 0L;
            client.out().write(client.request());
            readResponse(client.in(), client.reply(), client.expected());
            if (recordLatency && sampleIndex < latencySamples.length) {
                latencySamples[sampleIndex++] = LatencyRecorder.elapsedNanos(requestStarted);
            }
            if (successes != null) {
                successes.incrementAndGet();
            }
        }
    }

    private static void readResponse(InputStream in, byte[] reply, byte[] expected) throws IOException {
        String status = readLine(in);
        if (!status.startsWith("HTTP/1.1 200")) {
            throw new IOException("netty-bench: unexpected status " + status);
        }
        int contentLength = -1;
        while (true) {
            String line = readLine(in);
            if (line.isEmpty()) {
                break;
            }
            int sep = line.indexOf(':');
            if (sep > 0 && line.substring(0, sep).equalsIgnoreCase("Content-Length")) {
                contentLength = Integer.parseInt(line.substring(sep + 1).trim());
            }
        }
        if (contentLength != expected.length) {
            throw new IOException("netty-bench: content length " + contentLength + ", want " + expected.length);
        }
        readFully(in, reply, contentLength);
        if (!Arrays.equals(reply, expected)) {
            throw new IOException("netty-bench: response body mismatch");
        }
    }

    private static String readLine(InputStream in) throws IOException {
        ByteArrayOutputStream line = new ByteArrayOutputStream(128);
        int previous = -1;
        while (true) {
            int b = in.read();
            if (b < 0) {
                throw new EOFException("netty-bench: connection closed");
            }
            if (previous == '\r' && b == '\n') {
                byte[] bytes = line.toByteArray();
                return new String(bytes, 0, bytes.length - 1, StandardCharsets.US_ASCII);
            }
            line.write(b);
            previous = b;
        }
    }

    private static void readFully(InputStream in, byte[] dst, int length) throws IOException {
        int off = 0;
        while (off < length) {
            int n = in.read(dst, off, length - off);
            if (n < 0) {
                throw new EOFException("netty-bench: connection closed");
            }
            off += n;
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

    private static String firstNegotiatedProtocol(Http1Session[] clients) {
        for (Http1Session client : clients) {
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

    private static void closeClients(Http1Session[] clients) {
        for (Http1Session client : clients) {
            if (client == null) {
                continue;
            }
            try {
                client.close();
            } catch (IOException ignored) {
                }
        }
    }

    private record Http1Session(
            Socket socket,
            InputStream in,
            OutputStream out,
            byte[] request,
            byte[] expected,
            byte[] reply,
            String negotiatedProtocol) implements AutoCloseable {
        @Override
        public void close() throws IOException {
            socket.close();
        }
    }
}
