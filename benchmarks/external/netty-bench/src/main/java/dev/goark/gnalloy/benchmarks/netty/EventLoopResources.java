package dev.goark.gnalloy.benchmarks.netty;

import io.netty.channel.EventLoopGroup;
import io.netty.channel.ServerChannel;
import io.netty.channel.epoll.Epoll;
import io.netty.channel.epoll.EpollEventLoopGroup;
import io.netty.channel.epoll.EpollServerSocketChannel;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.nio.NioServerSocketChannel;

final class EventLoopResources implements AutoCloseable {
    private final EventLoopGroup bossGroup;
    private final EventLoopGroup workerGroup;
    private final Class<? extends ServerChannel> serverChannelType;

    private EventLoopResources(
            EventLoopGroup bossGroup,
            EventLoopGroup workerGroup,
            Class<? extends ServerChannel> serverChannelType) {
        this.bossGroup = bossGroup;
        this.workerGroup = workerGroup;
        this.serverChannelType = serverChannelType;
    }

    static EventLoopResources create(Config config) {
        return switch (config.backend()) {
            case NIO -> new EventLoopResources(
                    new NioEventLoopGroup(1),
                    new NioEventLoopGroup(config.eventLoops()),
                    NioServerSocketChannel.class);
            case EPOLL -> createEpoll(config);
        };
    }

    private static EventLoopResources createEpoll(Config config) {
        if (!Epoll.isAvailable()) {
            throw new IllegalStateException("netty-bench: epoll unavailable", Epoll.unavailabilityCause());
        }
        return new EventLoopResources(
                new EpollEventLoopGroup(1),
                new EpollEventLoopGroup(config.eventLoops()),
                EpollServerSocketChannel.class);
    }

    EventLoopGroup bossGroup() {
        return bossGroup;
    }

    EventLoopGroup workerGroup() {
        return workerGroup;
    }

    Class<? extends ServerChannel> serverChannelType() {
        return serverChannelType;
    }

    @Override
    public void close() throws InterruptedException {
        bossGroup.shutdownGracefully().sync();
        workerGroup.shutdownGracefully().sync();
    }
}
