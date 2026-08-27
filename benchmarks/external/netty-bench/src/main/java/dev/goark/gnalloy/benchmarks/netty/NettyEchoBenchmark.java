package dev.goark.gnalloy.benchmarks.netty;

import io.netty.bootstrap.ServerBootstrap;
import io.netty.buffer.ByteBuf;
import io.netty.channel.Channel;
import io.netty.channel.ChannelFuture;
import io.netty.channel.ChannelHandlerContext;
import io.netty.channel.ChannelInboundHandlerAdapter;
import io.netty.channel.ChannelInitializer;
import io.netty.channel.ChannelOption;
import io.netty.channel.EventLoopGroup;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.SocketChannel;
import io.netty.channel.socket.nio.NioServerSocketChannel;

import java.io.EOFException;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.time.Duration;
import java.util.Arrays;
import java.util.HashMap;
import java.util.Map;
import java.util.Objects;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicReference;

public final class NettyEchoBenchmark {
    private static final String BENCHMARK_NAME = "BenchmarkNettyTCPEcho";

    public static void main(String[] args) throws Exception {
        Config config = Config.parse(args);
        BenchmarkResult result = run(config);
        if (result.totalRequests() > 0) {
            writeResult(config, result);
        }
    }

    static BenchmarkResult run(Config config) throws Exception {
        config.validate();
        EchoServer server = EchoServer.start(config);
        try {
            return runLoad(server.address(), config);
        } finally {
            server.close();
        }
    }

    static BenchmarkResult runLoad(InetSocketAddress address, Config config) throws Exception {
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
        double throughput = elapsedNanos > 0 ? total * 1_000_000_000.0 / elapsedNanos : 0.0;
        double nsPerOp = total > 0 ? (double) elapsedNanos / total : 0.0;
        BenchmarkResult result = new BenchmarkResult(total, errors.get(), Duration.ofNanos(elapsedNanos), throughput, nsPerOp);

        Throwable failure = firstError.get();
        if (failure != null) {
            if (failure instanceof Exception exception) {
                throw exception;
            }
            throw new RuntimeException(failure);
        }
        long expected = (long) config.connections() * config.messages();
        if (total != expected) {
            throw new IOException("netty-bench: completed " + total + " requests, want " + expected);
        }
        return result;
    }

    static void runClient(InetSocketAddress address, Config config, int clientId, AtomicLong successes) throws IOException {
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

    static void writeResult(Config config, BenchmarkResult result) {
        System.out.printf(
                "framework=netty protocol=%s payload=%d connections=%d messages=%d total=%d errors=%d elapsed=%s throughput=%.2f ops/s%n",
                config.protocol(),
                config.payload(),
                config.connections(),
                config.messages(),
                result.totalRequests(),
                result.errors(),
                result.elapsed(),
                result.throughput());
        System.out.printf(
                "%s-%d %d %.0f ns/op%n",
                BENCHMARK_NAME,
                Runtime.getRuntime().availableProcessors(),
                result.totalRequests(),
                result.nsPerOp());
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

    static final class EchoServer implements AutoCloseable {
        private final EventLoopGroup bossGroup;
        private final EventLoopGroup workerGroup;
        private final Channel channel;

        private EchoServer(EventLoopGroup bossGroup, EventLoopGroup workerGroup, Channel channel) {
            this.bossGroup = bossGroup;
            this.workerGroup = workerGroup;
            this.channel = channel;
        }

        static EchoServer start(Config config) throws InterruptedException {
            EventLoopGroup bossGroup = new NioEventLoopGroup(1);
            EventLoopGroup workerGroup = new NioEventLoopGroup(config.eventLoops());
            try {
                ServerBootstrap bootstrap = new ServerBootstrap()
                        .group(bossGroup, workerGroup)
                        .channel(NioServerSocketChannel.class)
                        .option(ChannelOption.SO_REUSEADDR, true)
                        .childOption(ChannelOption.TCP_NODELAY, true)
                        .childHandler(new ChannelInitializer<SocketChannel>() {
                            @Override
                            protected void initChannel(SocketChannel ch) {
                                ch.pipeline().addLast(new EchoHandler());
                            }
                        });
                Channel channel = bootstrap.bind(config.host(), config.port()).sync().channel();
                return new EchoServer(bossGroup, workerGroup, channel);
            } catch (Throwable t) {
                bossGroup.shutdownGracefully();
                workerGroup.shutdownGracefully();
                if (t instanceof InterruptedException interrupted) {
                    throw interrupted;
                }
                if (t instanceof RuntimeException runtimeException) {
                    throw runtimeException;
                }
                throw new RuntimeException(t);
            }
        }

        InetSocketAddress address() {
            return (InetSocketAddress) channel.localAddress();
        }

        @Override
        public void close() throws InterruptedException {
            ChannelFuture closeFuture = channel.close().sync();
            closeFuture.await();
            bossGroup.shutdownGracefully().sync();
            workerGroup.shutdownGracefully().sync();
        }
    }

    static final class EchoHandler extends ChannelInboundHandlerAdapter {
        @Override
        public void channelRead(ChannelHandlerContext ctx, Object msg) {
            ctx.writeAndFlush(msg);
        }

        @Override
        public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) {
            ctx.close();
        }
    }

    record BenchmarkResult(long totalRequests, long errors, Duration elapsed, double throughput, double nsPerOp) {
    }

    record Config(
            String protocol,
            String host,
            int port,
            int payload,
            int connections,
            int messages,
            Duration timeout,
            int eventLoops) {

        static Config parse(String[] args) {
            Map<String, String> values = parseArgs(args);
            String protocol = values.getOrDefault("protocol", "tcp-echo");
            String addr = values.getOrDefault("addr", "127.0.0.1:0");
            HostPort hostPort = HostPort.parse(addr);
            return new Config(
                    protocol,
                    hostPort.host(),
                    hostPort.port(),
                    intValue(values, "payload", 1024),
                    intValue(values, "connections", 256),
                    intValue(values, "messages", 100000),
                    durationValue(values, "timeout", Duration.ofMinutes(5)),
                    intValue(values, "event-loops", Runtime.getRuntime().availableProcessors()));
        }

        void validate() {
            if (!Objects.equals(protocol, "tcp-echo")) {
                throw new IllegalArgumentException("netty-bench: unsupported protocol " + protocol);
            }
            if (host == null || host.isBlank()) {
                throw new IllegalArgumentException("netty-bench: empty host");
            }
            if (port < 0 || port > 65535) {
                throw new IllegalArgumentException("netty-bench: invalid port " + port);
            }
            if (payload <= 0 || connections <= 0 || messages <= 0) {
                throw new IllegalArgumentException("netty-bench: payload, connections and messages must be positive");
            }
            if (timeout == null || timeout.isZero() || timeout.isNegative()) {
                throw new IllegalArgumentException("netty-bench: timeout must be positive");
            }
            if (eventLoops <= 0) {
                throw new IllegalArgumentException("netty-bench: event-loops must be positive");
            }
        }
    }

    record HostPort(String host, int port) {
        static HostPort parse(String addr) {
            int sep = addr.lastIndexOf(':');
            if (sep <= 0 || sep == addr.length() - 1) {
                throw new IllegalArgumentException("netty-bench: invalid addr " + addr);
            }
            return new HostPort(addr.substring(0, sep), Integer.parseInt(addr.substring(sep + 1)));
        }
    }

    private static Map<String, String> parseArgs(String[] args) {
        Map<String, String> values = new HashMap<>();
        for (int i = 0; i < args.length; i++) {
            String key = args[i];
            if (!key.startsWith("--")) {
                throw new IllegalArgumentException("netty-bench: invalid argument " + key);
            }
            if (i + 1 >= args.length) {
                throw new IllegalArgumentException("netty-bench: missing value for " + key);
            }
            values.put(key.substring(2), args[++i]);
        }
        return values;
    }

    private static int intValue(Map<String, String> values, String key, int fallback) {
        String value = values.get(key);
        if (value == null || value.isBlank()) {
            return fallback;
        }
        return Integer.parseInt(value);
    }

    private static Duration durationValue(Map<String, String> values, String key, Duration fallback) {
        String value = values.get(key);
        if (value == null || value.isBlank()) {
            return fallback;
        }
        if (value.endsWith("ms")) {
            return Duration.ofMillis(Long.parseLong(value.substring(0, value.length() - 2)));
        }
        if (value.endsWith("s")) {
            return Duration.ofSeconds(Long.parseLong(value.substring(0, value.length() - 1)));
        }
        if (value.endsWith("m")) {
            return Duration.ofMinutes(Long.parseLong(value.substring(0, value.length() - 1)));
        }
        return Duration.ofMillis(Long.parseLong(value));
    }
}
