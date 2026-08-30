package dev.goark.gnalloy.benchmarks.netty;

import io.netty.channel.EventLoopGroup;
import io.netty.channel.epoll.Epoll;
import io.netty.channel.epoll.EpollDatagramChannel;
import io.netty.channel.epoll.EpollEventLoopGroup;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.DatagramChannel;
import io.netty.channel.socket.nio.NioDatagramChannel;

final class DatagramEventLoopResources implements AutoCloseable {
    private final EventLoopGroup group;
    private final Class<? extends DatagramChannel> channelType;

    private DatagramEventLoopResources(EventLoopGroup group, Class<? extends DatagramChannel> channelType) {
        this.group = group;
        this.channelType = channelType;
    }

    static DatagramEventLoopResources create(Config config) {
        return switch (config.backend()) {
            case NIO -> new DatagramEventLoopResources(
                    new NioEventLoopGroup(config.eventLoops()),
                    NioDatagramChannel.class);
            case EPOLL -> createEpoll(config);
        };
    }

    private static DatagramEventLoopResources createEpoll(Config config) {
        if (!Epoll.isAvailable()) {
            throw new IllegalStateException("netty-bench: epoll unavailable", Epoll.unavailabilityCause());
        }
        return new DatagramEventLoopResources(
                new EpollEventLoopGroup(config.eventLoops()),
                EpollDatagramChannel.class);
    }

    EventLoopGroup group() {
        return group;
    }

    Class<? extends DatagramChannel> channelType() {
        return channelType;
    }

    @Override
    public void close() throws InterruptedException {
        group.shutdownGracefully().sync();
    }
}

