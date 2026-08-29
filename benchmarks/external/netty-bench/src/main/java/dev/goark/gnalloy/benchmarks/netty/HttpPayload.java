package dev.goark.gnalloy.benchmarks.netty;

import java.nio.charset.StandardCharsets;

final class HttpPayload {
    private HttpPayload() {
    }

    static byte[] body(int size) {
        byte[] body = new byte[size];
        for (int i = 0; i < body.length; i++) {
            body[i] = (byte) i;
        }
        return body;
    }

    static byte[] request(String host) {
        String normalized = host == null || host.isBlank() ? "127.0.0.1" : host;
        return ("GET /bench HTTP/1.1\r\nHost: " + normalized
                + "\r\nUser-Agent: gnalloy-bench\r\nAccept: */*\r\nConnection: keep-alive\r\n\r\n")
                .getBytes(StandardCharsets.US_ASCII);
    }
}
