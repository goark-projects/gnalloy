package dev.goark.gnalloy.benchmarks.netty;

import java.io.IOException;
import java.net.Socket;

record ClientSession(Socket socket, byte[] payload, byte[] reply) implements AutoCloseable {
    @Override
    public void close() throws IOException {
        socket.close();
    }
}
