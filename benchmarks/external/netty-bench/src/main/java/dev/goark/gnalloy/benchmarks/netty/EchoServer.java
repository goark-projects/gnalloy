package dev.goark.gnalloy.benchmarks.netty;

import io.netty.bootstrap.ServerBootstrap;
import io.netty.buffer.PooledByteBufAllocator;
import io.netty.channel.Channel;
import io.netty.channel.ChannelFuture;
import io.netty.channel.ChannelInitializer;
import io.netty.channel.ChannelOption;
import io.netty.channel.socket.SocketChannel;
import io.netty.handler.codec.http.HttpServerCodec;
import io.netty.handler.codec.http2.Http2FrameCodecBuilder;
import io.netty.handler.ssl.SslContext;

import java.net.InetSocketAddress;

final class EchoServer implements AutoCloseable {
    private final EventLoopResources resources;
    private final Channel channel;

    private EchoServer(EventLoopResources resources, Channel channel) {
        this.resources = resources;
        this.channel = channel;
    }

    static EchoServer start(Config config) throws Exception {
        EventLoopResources resources = EventLoopResources.create(config);
        try {
            SslContext sslContext = SslSupport.serverContext(config);
            ServerBootstrap bootstrap = new ServerBootstrap()
                    .group(resources.bossGroup(), resources.workerGroup())
                    .channel(resources.serverChannelType())
                    .option(ChannelOption.SO_REUSEADDR, true)
                    .childOption(ChannelOption.TCP_NODELAY, true)
                    .childOption(ChannelOption.ALLOCATOR, PooledByteBufAllocator.DEFAULT)
                    .childHandler(new ChannelInitializer<SocketChannel>() {
                        @Override
                        protected void initChannel(SocketChannel ch) {
                            if (config.http1Family()) {
                                if (sslContext != null) {
                                    ch.pipeline().addLast(sslContext.newHandler(ch.alloc()));
                                }
                                ch.pipeline().addLast(new HttpServerCodec());
                                ch.pipeline().addLast(new Http1ServerHandler(config.payload()));
                                return;
                            }
                            if (config.http2Family()) {
                                if (sslContext != null) {
                                    ch.pipeline().addLast(sslContext.newHandler(ch.alloc()));
                                }
                                ch.pipeline().addLast(Http2FrameCodecBuilder.forServer().build());
                                ch.pipeline().addLast(new Http2ServerHandler(config.payload()));
                                return;
                            }
                            ch.pipeline().addLast(new EchoHandler());
                        }
                    });
            Channel channel = bootstrap.bind(config.host(), config.port()).sync().channel();
            return new EchoServer(resources, channel);
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
}
