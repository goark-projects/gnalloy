package dev.goark.gnalloy.benchmarks.netty;

import io.netty.bootstrap.Bootstrap;
import io.netty.buffer.PooledByteBufAllocator;
import io.netty.channel.Channel;
import io.netty.channel.ChannelHandler;
import io.netty.channel.ChannelInboundHandlerAdapter;
import io.netty.channel.ChannelOption;
import io.netty.handler.codec.http3.DefaultHttp3Headers;
import io.netty.handler.codec.http3.Http3;
import io.netty.handler.codec.http3.Http3ClientConnectionHandler;
import io.netty.handler.codec.http3.Http3Headers;
import io.netty.handler.codec.quic.QuicChannel;
import io.netty.handler.codec.quic.QuicSslContext;
import io.netty.handler.codec.quic.QuicSslContextBuilder;
import io.netty.handler.ssl.util.InsecureTrustManagerFactory;

import java.net.InetSocketAddress;
import java.util.concurrent.TimeUnit;

final class Http3ClientBootstrap {
    private static final String REQUEST_PATH = "/bench";

    private Http3ClientBootstrap() {
    }

    static Channel openDatagramChannel(DatagramEventLoopResources resources, Config config) throws Exception {
        ChannelHandler codec = Http3.newQuicClientCodecBuilder()
                .sslContext(clientSslContext(config))
                .maxIdleTimeout(config.timeout().toMillis(), TimeUnit.MILLISECONDS)
                .initialMaxData(16L * 1024 * 1024)
                .initialMaxStreamDataBidirectionalLocal(6L * 1024 * 1024)
                .initialMaxStreamDataBidirectionalRemote(6L * 1024 * 1024)
                .initialMaxStreamDataUnidirectional(6L * 1024 * 1024)
                .initialMaxStreamsBidirectional(Math.max(1024, config.connections() * 4L))
                .initialMaxStreamsUnidirectional(Http3.MIN_INITIAL_MAX_STREAMS_UNIDIRECTIONAL)
                .build();
        return new Bootstrap()
                .group(resources.group())
                .channel(resources.channelType())
                .option(ChannelOption.ALLOCATOR, PooledByteBufAllocator.DEFAULT)
                .handler(codec)
                .bind(0)
                .sync()
                .channel();
    }

    static Http3Client[] prepareClients(InetSocketAddress address, Config config, DatagramEventLoopResources resources) throws Exception {
        Http3Client[] clients = new Http3Client[config.connections()];
        try {
            for (int i = 0; i < clients.length; i++) {
                clients[i] = connectClient(address, config, resources);
            }
            return clients;
        } catch (Exception failure) {
            Http3LoadGenerator.closeClients(clients);
            throw failure;
        }
    }

    private static Http3Client connectClient(InetSocketAddress address, Config config, DatagramEventLoopResources resources) throws Exception {
        Channel datagram = openDatagramChannel(resources, config);
        try {
            QuicChannel channel = QuicChannel.newBootstrap(datagram)
                    .handler(new Http3ClientConnectionHandler())
                    .streamHandler(new ChannelInboundHandlerAdapter())
                    .remoteAddress(address)
                    .connect()
                    .get(config.timeout().toMillis(), TimeUnit.MILLISECONDS);
            return new Http3Client(datagram, channel, requestHeaders(config.host()), HttpPayload.body(config.payload()));
        } catch (Exception failure) {
            datagram.close().sync();
            throw failure;
        }
    }

    private static QuicSslContext clientSslContext(Config config) throws Exception {
        return QuicSslContextBuilder
                .forClient()
                .trustManager(InsecureTrustManagerFactory.INSTANCE)
                .applicationProtocols(Http3Server.http3ApplicationProtocols(config))
                .build();
    }

    private static Http3Headers requestHeaders(String host) {
        String authority = host == null || host.isBlank() ? "127.0.0.1" : host;
        return new DefaultHttp3Headers()
                .method("GET")
                .scheme("https")
                .authority(authority)
                .path(REQUEST_PATH);
    }
}
