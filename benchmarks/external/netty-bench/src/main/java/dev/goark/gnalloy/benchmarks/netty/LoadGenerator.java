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
        ExecutorService pool = Executors.newFixedThreadPool(config.connections());
        long started = System.nanoTime();
        for (int i = 0; i < config.connections(); i++) {
            final int clientId = i;
            pool.execute(() -> {
                try {
                    runClient(address, config, clientId, successes);
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
        BenchmarkResult result = result(total, errors.get(), elapsedNanos);
        rethrowFirstError(firstError.get());
        long expected = (long) config.connections() * config.messages();
        if (total != expected) {
            throw new IOException("netty-bench: completed " + total + " requests, want " + expected);
        }
        return result;
    }

    private static BenchmarkResult result(long total, long errors, long elapsedNanos) {
        double throughput = elapsedNanos > 0 ? total * 1_000_000_000.0 / elapsedNanos : 0.0;
        double nsPerOp = total > 0 ? (double) elapsedNanos / total : 0.0;
        return new BenchmarkResult(total, errors, Duration.ofNanos(elapsedNanos), throughput, nsPerOp);
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
            InetSocketAddress address,
            Config config,
            int clientId,
            AtomicLong successes) throws IOException {
        byte[] payload = makePayload(config.payload(), clientId);
        byte[] reply = new byte[config.payload()];
        try (Socket socket = new Socket()) {
            socket.setTcpNoDelay(true);
            socket.connect(address, Math.toIntExact(config.timeout().toMillis()));
            socket.setSoTimeout(Math.toIntExact(config.timeout().toMillis()));
            OutputStream out = socket.getOutputStream();
            InputStream in = socket.getInputStream();
            for (int i = 0; i < config.messages(); i++) {
                payload[0] = (byte) (clientId + i);
                out.write(payload);
                readFully(in, reply);
                if (!Arrays.equals(reply, payload)) {
                    throw new IOException("netty-bench: echo mismatch");
                }
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
