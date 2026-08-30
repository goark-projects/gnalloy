package dev.goark.gnalloy.benchmarks.netty;

import io.netty.bootstrap.Bootstrap;
import io.netty.buffer.PooledByteBufAllocator;
import io.netty.channel.Channel;
import io.netty.channel.ChannelFuture;
import io.netty.channel.ChannelHandler;
import io.netty.channel.ChannelInitializer;
import io.netty.channel.ChannelOption;
import io.netty.handler.codec.http3.Http3;
import io.netty.handler.codec.http3.Http3ServerConnectionHandler;
import io.netty.handler.codec.quic.InsecureQuicTokenHandler;
import io.netty.handler.codec.quic.QuicSslContext;
import io.netty.handler.codec.quic.QuicSslContextBuilder;
import io.netty.handler.ssl.util.SelfSignedCertificate;

import java.net.InetSocketAddress;
import java.util.concurrent.TimeUnit;

final class Http3Server implements AutoCloseable {
    private static final long STREAM_WINDOW = 6L * 1024 * 1024;
    private static final long CONNECTION_WINDOW = 16L * 1024 * 1024;

    private final DatagramEventLoopResources resources;
    private final Channel channel;

    private Http3Server(DatagramEventLoopResources resources, Channel channel) {
        this.resources = resources;
        this.channel = channel;
    }

    static Http3Server start(Config config) throws Exception {
        DatagramEventLoopResources resources = DatagramEventLoopResources.create(config);
        try {
            ChannelHandler codec = serverCodec(config);
            Channel channel = new Bootstrap()
                    .group(resources.group())
                    .channel(resources.channelType())
                    .option(ChannelOption.SO_REUSEADDR, true)
                    .option(ChannelOption.ALLOCATOR, PooledByteBufAllocator.DEFAULT)
                    .handler(codec)
                    .bind(config.host(), config.port())
                    .sync()
                    .channel();
            return new Http3Server(resources, channel);
        } catch (Throwable t) {
            resources.close();
            if (t instanceof InterruptedException interrupted) {
                throw interrupted;
            }
            if (t instanceof Exception exception) {
                throw exception;
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
        resources.close();
    }

    private static ChannelHandler serverCodec(Config config) throws Exception {
        SelfSignedCertificate certificate = new SelfSignedCertificate("gnalloy.local");
        QuicSslContext sslContext = QuicSslContextBuilder
                .forServer(certificate.key(), null, certificate.cert())
                .applicationProtocols(http3ApplicationProtocols(config))
                .build();
        return Http3.newQuicServerCodecBuilder()
                .sslContext(sslContext)
                .maxIdleTimeout(config.timeout().toMillis(), TimeUnit.MILLISECONDS)
                .initialMaxData(CONNECTION_WINDOW)
                .initialMaxStreamDataBidirectionalLocal(STREAM_WINDOW)
                .initialMaxStreamDataBidirectionalRemote(STREAM_WINDOW)
                .initialMaxStreamDataUnidirectional(STREAM_WINDOW)
                .initialMaxStreamsBidirectional(Math.max(1024, config.connections() * 4L))
                .initialMaxStreamsUnidirectional(Http3.MIN_INITIAL_MAX_STREAMS_UNIDIRECTIONAL)
                .tokenHandler(InsecureQuicTokenHandler.INSTANCE)
                .handler(new ChannelInitializer<Channel>() {
                    @Override
                    protected void initChannel(Channel channel) {
                        channel.pipeline().addLast(new Http3ServerConnectionHandler(new Http3ServerHandler(config.payload())));
                    }
                })
                .build();
    }

    static String[] http3ApplicationProtocols(Config config) {
        if (config.alpnProtocols().isEmpty()) {
            return Http3.supportedApplicationProtocols();
        }
        return config.alpnProtocols().toArray(String[]::new);
    }
}
