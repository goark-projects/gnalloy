package dev.goark.gnalloy.benchmarks.netty;

import io.netty.buffer.ByteBuf;
import io.netty.buffer.Unpooled;
import io.netty.channel.Channel;
import io.netty.channel.ChannelHandlerContext;
import io.netty.handler.codec.http3.DefaultHttp3HeadersFrame;
import io.netty.handler.codec.http3.Http3;
import io.netty.handler.codec.http3.Http3DataFrame;
import io.netty.handler.codec.http3.Http3Headers;
import io.netty.handler.codec.http3.Http3HeadersFrame;
import io.netty.handler.codec.http3.Http3RequestStreamInboundHandler;
import io.netty.handler.codec.quic.QuicChannel;
import io.netty.handler.codec.quic.QuicStreamChannel;
import io.netty.util.concurrent.Future;

import java.io.IOException;
import java.time.Duration;
import java.util.Arrays;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;

record Http3Client(Channel datagram, QuicChannel channel, Http3Headers headers, byte[] expected) implements AutoCloseable {
    void request(Duration timeout) throws Exception {
        byte[] reply = new byte[expected.length];
        ResponseHandler handler = new ResponseHandler(expected, reply);
        Future<QuicStreamChannel> streamFuture = Http3.newRequestStream(channel, handler);
        QuicStreamChannel stream = streamFuture.get(timeout.toMillis(), TimeUnit.MILLISECONDS);
        stream.writeAndFlush(new DefaultHttp3HeadersFrame(headers))
                .addListener(QuicStreamChannel.SHUTDOWN_OUTPUT)
                .get(timeout.toMillis(), TimeUnit.MILLISECONDS);
        handler.await(timeout);
    }

    String negotiatedProtocol() {
        String protocol = channel.sslEngine().getApplicationProtocol();
        return protocol == null ? "" : protocol;
    }

    @Override
    public void close() throws Exception {
        try {
            channel.close(true, 0, Unpooled.EMPTY_BUFFER).get(5, TimeUnit.SECONDS);
        } finally {
            datagram.close().sync();
        }
    }

    private static final class ResponseHandler extends Http3RequestStreamInboundHandler {
        private final byte[] expected;
        private final byte[] reply;
        private final CompletableFuture<Void> done = new CompletableFuture<>();
        private int received;
        private boolean statusOK;

        private ResponseHandler(byte[] expected, byte[] reply) {
            this.expected = expected;
            this.reply = reply;
        }

        void await(Duration timeout) throws Exception {
            done.get(timeout.toMillis(), TimeUnit.MILLISECONDS);
        }

        @Override
        protected void channelRead(ChannelHandlerContext ctx, Http3HeadersFrame frame) {
            CharSequence status = frame.headers().status();
            statusOK = status != null && "200".contentEquals(status);
            completeIfReady(ctx);
        }

        @Override
        protected void channelRead(ChannelHandlerContext ctx, Http3DataFrame frame) {
            try {
                ByteBuf content = frame.content();
                int readable = content.readableBytes();
                if (readable > reply.length - received) {
                    throw new IOException("netty-bench: response body too large");
                }
                content.readBytes(reply, received, readable);
                received += readable;
                completeIfReady(ctx);
            } catch (Throwable t) {
                done.completeExceptionally(t);
                ctx.close();
            } finally {
                frame.release();
            }
        }

        @Override
        protected void channelInputClosed(ChannelHandlerContext ctx) {
            if (!done.isDone()) {
                done.completeExceptionally(new IOException("netty-bench: h3 stream closed"));
            }
            ctx.close();
        }

        @Override
        public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) {
            done.completeExceptionally(cause);
            ctx.close();
        }

        private void completeIfReady(ChannelHandlerContext ctx) {
            if (!statusOK || received != expected.length) {
                return;
            }
            if (!Arrays.equals(reply, expected)) {
                done.completeExceptionally(new IOException("netty-bench: response body mismatch"));
                ctx.close();
                return;
            }
            done.complete(null);
            ctx.close();
        }
    }
}
