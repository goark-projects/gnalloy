package dev.goark.gnalloy.benchmarks.netty;

import io.netty.bootstrap.Bootstrap;
import io.netty.buffer.PooledByteBufAllocator;
import io.netty.channel.Channel;
import io.netty.channel.ChannelFuture;
import io.netty.channel.ChannelOption;
import io.netty.channel.EventLoopGroup;
import io.netty.channel.SimpleChannelInboundHandler;
import io.netty.channel.epoll.Epoll;
import io.netty.channel.epoll.EpollDatagramChannel;
import io.netty.channel.epoll.EpollEventLoopGroup;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.DatagramChannel;
import io.netty.channel.socket.DatagramPacket;
import io.netty.channel.socket.nio.NioDatagramChannel;

import java.net.InetSocketAddress;

final class DatagramEchoServer implements AutoCloseable {
    private final EventLoopGroup group;
    private final Channel channel;

    private DatagramEchoServer(EventLoopGroup group, Channel channel) {
        this.group = group;
        this.channel = channel;
    }

    static DatagramEchoServer start(Config config) throws Exception {
        EventLoopGroup group = createGroup(config);
        try {
            Bootstrap bootstrap = new Bootstrap()
                    .group(group)
                    .channel(channelType(config))
                    .option(ChannelOption.SO_REUSEADDR, true)
                    .option(ChannelOption.ALLOCATOR, PooledByteBufAllocator.DEFAULT)
                    .handler(new DatagramEchoHandler());
            Channel channel = bootstrap.bind(config.host(), config.port()).sync().channel();
            return new DatagramEchoServer(group, channel);
        } catch (Throwable t) {
            group.shutdownGracefully().sync();
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
        group.shutdownGracefully().sync();
    }

    private static EventLoopGroup createGroup(Config config) {
        return switch (config.backend()) {
            case NIO -> new NioEventLoopGroup(config.eventLoops());
            case EPOLL -> createEpollGroup(config);
        };
    }

    private static EventLoopGroup createEpollGroup(Config config) {
        if (!Epoll.isAvailable()) {
            throw new IllegalStateException("netty-bench: epoll unavailable", Epoll.unavailabilityCause());
        }
        return new EpollEventLoopGroup(config.eventLoops());
    }

    private static Class<? extends DatagramChannel> channelType(Config config) {
        return switch (config.backend()) {
            case NIO -> NioDatagramChannel.class;
            case EPOLL -> EpollDatagramChannel.class;
        };
    }

    private static final class DatagramEchoHandler extends SimpleChannelInboundHandler<DatagramPacket> {
        @Override
        protected void channelRead0(io.netty.channel.ChannelHandlerContext ctx, DatagramPacket packet) {
            ctx.writeAndFlush(new DatagramPacket(packet.content().retainedDuplicate(), packet.sender()));
        }

        @Override
        public void exceptionCaught(io.netty.channel.ChannelHandlerContext ctx, Throwable cause) {
            ctx.close();
        }
    }
}
