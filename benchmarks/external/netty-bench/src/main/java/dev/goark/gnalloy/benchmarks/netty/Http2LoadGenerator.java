package dev.goark.gnalloy.benchmarks.netty;

import java.io.EOFException;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.nio.ByteBuffer;
import java.time.Duration;
import java.util.Arrays;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicReference;

final class Http2LoadGenerator {
    private static final byte FRAME_DATA = 0x0;
    private static final byte FRAME_HEADERS = 0x1;
    private static final byte FRAME_SETTINGS = 0x4;
    private static final byte FRAME_PING = 0x6;
    private static final byte FRAME_WINDOW_UPDATE = 0x8;
    private static final byte FLAG_END_STREAM = 0x1;
    private static final byte FLAG_ACK = 0x1;
    private static final byte FLAG_END_HEADERS = 0x4;
    private static final byte[] CLIENT_PREFACE = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n".getBytes(java.nio.charset.StandardCharsets.US_ASCII);

    private Http2LoadGenerator() {
    }

    static BenchmarkResult run(InetSocketAddress address, Config config) throws Exception {
        AtomicLong successes = new AtomicLong();
        AtomicLong errors = new AtomicLong();
        AtomicReference<Throwable> firstError = new AtomicReference<>();
        long[][] latencySamples = LatencyRecorder.samplingEnabled(config.latencySampleRate())
                ? new long[config.connections()][]
                : new long[0][];
        Http2Session[] clients = prepareClients(address, config);
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

    private static Http2Session[] prepareClients(InetSocketAddress address, Config config) throws Exception {
        Http2Session[] clients = new Http2Session[config.connections()];
        try {
            for (int i = 0; i < clients.length; i++) {
                Socket socket = SslSupport.connect(address, config);
                sendClientPreface(socket.getOutputStream());
                clients[i] = new Http2Session(
                        socket,
                        socket.getInputStream(),
                        socket.getOutputStream(),
                        requestHeaderBlock(config.host(), config.tlsEnabled()),
                        HttpPayload.body(config.payload()),
                        new byte[config.payload()],
                        1,
                        SslSupport.negotiatedProtocol(socket));
            }
            return clients;
        } catch (Exception failure) {
            closeClients(clients);
            throw failure;
        }
    }

    private static void sendClientPreface(OutputStream out) throws IOException {
        out.write(CLIENT_PREFACE);
        writeFrame(out, FRAME_SETTINGS, (byte) 0, 0, new byte[0]);
        ByteBuffer window = ByteBuffer.allocate(4);
        window.putInt(1 << 30);
        writeFrame(out, FRAME_WINDOW_UPDATE, (byte) 0, 0, window.array());
    }

    private static void runWarmup(Http2Session[] clients, Config config) throws Exception {
        if (config.warmupMessages() <= 0) {
            return;
        }
        AtomicReference<Throwable> firstError = new AtomicReference<>();
        ExecutorService pool = Executors.newFixedThreadPool(config.connections());
        for (Http2Session client : clients) {
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
            Http2Session client,
            int messageCount,
            AtomicLong successes,
            long[] latencySamples) throws IOException {
        int sampleIndex = 0;
        for (int i = 0; i < messageCount; i++) {
            int streamId = client.nextStreamId();
            boolean recordLatency = latencySamples.length > 0
                    && LatencyRecorder.shouldRecord(i, config.latencySampleRate());
            long requestStarted = recordLatency ? System.nanoTime() : 0L;
            writeFrame(client.out(), FRAME_HEADERS, (byte) (FLAG_END_HEADERS | FLAG_END_STREAM), streamId, client.headerBlock());
            readResponse(client, streamId);
            if (recordLatency && sampleIndex < latencySamples.length) {
                latencySamples[sampleIndex++] = LatencyRecorder.elapsedNanos(requestStarted);
            }
            if (successes != null) {
                successes.incrementAndGet();
            }
        }
    }

    private static void readResponse(Http2Session client, int streamId) throws IOException {
        int received = 0;
        while (true) {
            FrameHeader header = readFrameHeader(client.in());
            switch (header.type()) {
                case FRAME_SETTINGS -> {
                    skipFully(client.in(), header.length());
                    if ((header.flags() & FLAG_ACK) == 0) {
                        writeFrame(client.out(), FRAME_SETTINGS, FLAG_ACK, 0, new byte[0]);
                    }
                }
                case FRAME_PING -> {
                    byte[] payload = readPayload(client.in(), header.length());
                    if ((header.flags() & FLAG_ACK) == 0) {
                        writeFrame(client.out(), FRAME_PING, FLAG_ACK, 0, payload);
                    }
                }
                case FRAME_HEADERS -> skipFully(client.in(), header.length());
                case FRAME_DATA -> {
                    if (header.streamId() != streamId) {
                        skipFully(client.in(), header.length());
                        continue;
                    }
                    if (header.length() > client.reply().length - received) {
                        throw new IOException("netty-bench: response body too large");
                    }
                    readFully(client.in(), client.reply(), received, header.length());
                    received += header.length();
                    if ((header.flags() & FLAG_END_STREAM) == 0) {
                        continue;
                    }
                    if (received != client.expected().length) {
                        throw new IOException("netty-bench: response body length " + received + ", want " + client.expected().length);
                    }
                    if (!Arrays.equals(client.reply(), client.expected())) {
                        throw new IOException("netty-bench: response body mismatch");
                    }
                    return;
                }
                default -> skipFully(client.in(), header.length());
            }
        }
    }

    private static FrameHeader readFrameHeader(InputStream in) throws IOException {
        byte[] header = new byte[9];
        readFully(in, header, 0, header.length);
        int length = (header[0] & 0xff) << 16 | (header[1] & 0xff) << 8 | (header[2] & 0xff);
        int streamId = ByteBuffer.wrap(header, 5, 4).getInt() & 0x7fffffff;
        return new FrameHeader(length, header[3], header[4], streamId);
    }

    private static void writeFrame(OutputStream out, byte type, byte flags, int streamId, byte[] payload) throws IOException {
        byte[] header = new byte[9];
        header[0] = (byte) (payload.length >> 16);
        header[1] = (byte) (payload.length >> 8);
        header[2] = (byte) payload.length;
        header[3] = type;
        header[4] = flags;
        ByteBuffer.wrap(header, 5, 4).putInt(streamId & 0x7fffffff);
        out.write(header);
        if (payload.length > 0) {
            out.write(payload);
        }
    }

    private static byte[] requestHeaderBlock(String host, boolean tlsEnabled) {
        String authority = host == null || host.isBlank() ? "127.0.0.1" : host;
        byte[] path = "/bench".getBytes(java.nio.charset.StandardCharsets.US_ASCII);
        byte[] hostBytes = authority.getBytes(java.nio.charset.StandardCharsets.US_ASCII);
        byte[] out = new byte[4 + path.length + 2 + hostBytes.length];
        int offset = 0;
        out[offset++] = (byte) 0x82;
        out[offset++] = tlsEnabled ? (byte) 0x87 : (byte) 0x86;
        out[offset++] = 0x04;
        out[offset++] = (byte) path.length;
        System.arraycopy(path, 0, out, offset, path.length);
        offset += path.length;
        out[offset++] = 0x01;
        out[offset++] = (byte) hostBytes.length;
        System.arraycopy(hostBytes, 0, out, offset, hostBytes.length);
        return out;
    }

    private static byte[] readPayload(InputStream in, int length) throws IOException {
        byte[] payload = new byte[length];
        readFully(in, payload, 0, payload.length);
        return payload;
    }

    private static void skipFully(InputStream in, int length) throws IOException {
        long remaining = length;
        while (remaining > 0) {
            long skipped = in.skip(remaining);
            if (skipped > 0) {
                remaining -= skipped;
                continue;
            }
            if (in.read() < 0) {
                throw new EOFException("netty-bench: connection closed");
            }
            remaining--;
        }
    }

    private static void readFully(InputStream in, byte[] dst, int off, int length) throws IOException {
        int end = off + length;
        while (off < end) {
            int n = in.read(dst, off, end - off);
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

    private static String firstNegotiatedProtocol(Http2Session[] clients) {
        for (Http2Session client : clients) {
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

    private static void closeClients(Http2Session[] clients) {
        for (Http2Session client : clients) {
            if (client == null) {
                continue;
            }
            try {
                client.close();
            } catch (IOException ignored) {
            }
        }
    }

    private record FrameHeader(int length, byte type, byte flags, int streamId) {
    }

    private static final class Http2Session implements AutoCloseable {
        private final Socket socket;
        private final InputStream in;
        private final OutputStream out;
        private final byte[] headerBlock;
        private final byte[] expected;
        private final byte[] reply;
        private final String negotiatedProtocol;
        private int nextStreamId;

        private Http2Session(
                Socket socket,
                InputStream in,
                OutputStream out,
                byte[] headerBlock,
                byte[] expected,
                byte[] reply,
                int nextStreamId,
                String negotiatedProtocol) {
            this.socket = socket;
            this.in = in;
            this.out = out;
            this.headerBlock = headerBlock;
            this.expected = expected;
            this.reply = reply;
            this.nextStreamId = nextStreamId;
            this.negotiatedProtocol = negotiatedProtocol;
        }

        InputStream in() {
            return in;
        }

        OutputStream out() {
            return out;
        }

        byte[] headerBlock() {
            return headerBlock;
        }

        byte[] expected() {
            return expected;
        }

        byte[] reply() {
            return reply;
        }

        String negotiatedProtocol() {
            return negotiatedProtocol;
        }

        int nextStreamId() {
            int id = nextStreamId;
            nextStreamId += 2;
            return id;
        }

        @Override
        public void close() throws IOException {
            socket.close();
        }
    }
}
