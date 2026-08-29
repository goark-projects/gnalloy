package dev.goark.gnalloy.benchmarks.netty;

import io.netty.buffer.Unpooled;
import io.netty.channel.ChannelHandlerContext;
import io.netty.channel.SimpleChannelInboundHandler;
import io.netty.handler.codec.http.HttpHeaderNames;
import io.netty.handler.codec.http.HttpHeaderValues;
import io.netty.handler.codec.http2.DefaultHttp2DataFrame;
import io.netty.handler.codec.http2.DefaultHttp2Headers;
import io.netty.handler.codec.http2.DefaultHttp2HeadersFrame;
import io.netty.handler.codec.http2.Http2DataFrame;
import io.netty.handler.codec.http2.Http2Frame;
import io.netty.handler.codec.http2.Http2Headers;
import io.netty.handler.codec.http2.Http2HeadersFrame;

final class Http2ServerHandler extends SimpleChannelInboundHandler<Http2Frame> {
    private final byte[] body;
    private final String contentLength;

    Http2ServerHandler(int payload) {
        this.body = HttpPayload.body(payload);
        this.contentLength = Integer.toString(body.length);
    }

    private Http2Headers responseHeaders() {
        return new DefaultHttp2Headers()
                .status("200")
                .add(HttpHeaderNames.CONTENT_TYPE, HttpHeaderValues.APPLICATION_OCTET_STREAM)
                .add(HttpHeaderNames.CONTENT_LENGTH, contentLength);
    }

    @Override
    protected void channelRead0(ChannelHandlerContext ctx, Http2Frame frame) {
        if (frame instanceof Http2HeadersFrame headersFrame) {
            ctx.write(new DefaultHttp2HeadersFrame(responseHeaders(), false).stream(headersFrame.stream()));
            ctx.writeAndFlush(new DefaultHttp2DataFrame(Unpooled.wrappedBuffer(body), true).stream(headersFrame.stream()));
            return;
        }
    }

    @Override
    public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) {
        ctx.close();
    }
}
