package dev.goark.gnalloy.benchmarks.netty;

import io.netty.buffer.Unpooled;
import io.netty.channel.ChannelHandler;
import io.netty.channel.ChannelHandlerContext;
import io.netty.handler.codec.http.HttpHeaderNames;
import io.netty.handler.codec.http.HttpHeaderValues;
import io.netty.handler.codec.http3.DefaultHttp3DataFrame;
import io.netty.handler.codec.http3.DefaultHttp3Headers;
import io.netty.handler.codec.http3.DefaultHttp3HeadersFrame;
import io.netty.handler.codec.http3.Http3DataFrame;
import io.netty.handler.codec.http3.Http3Headers;
import io.netty.handler.codec.http3.Http3HeadersFrame;
import io.netty.handler.codec.http3.Http3RequestStreamInboundHandler;
import io.netty.handler.codec.quic.QuicStreamChannel;

@ChannelHandler.Sharable
final class Http3ServerHandler extends Http3RequestStreamInboundHandler {
    private final byte[] body;
    private final String contentLength;

    Http3ServerHandler(int payload) {
        this.body = HttpPayload.body(payload);
        this.contentLength = Integer.toString(body.length);
    }

    @Override
    protected void channelRead(ChannelHandlerContext ctx, Http3HeadersFrame frame) {
        ctx.write(new DefaultHttp3HeadersFrame(responseHeaders()));
        ctx.writeAndFlush(new DefaultHttp3DataFrame(Unpooled.wrappedBuffer(body)))
                .addListener(QuicStreamChannel.SHUTDOWN_OUTPUT);
    }

    @Override
    protected void channelRead(ChannelHandlerContext ctx, Http3DataFrame frame) {
        frame.release();
    }

    @Override
    protected void channelInputClosed(ChannelHandlerContext ctx) {
        ctx.close();
    }

    @Override
    public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) {
        ctx.close();
    }

    private Http3Headers responseHeaders() {
        return new DefaultHttp3Headers()
                .status("200")
                .add(HttpHeaderNames.CONTENT_TYPE, HttpHeaderValues.APPLICATION_OCTET_STREAM)
                .add(HttpHeaderNames.CONTENT_LENGTH, contentLength);
    }
}
