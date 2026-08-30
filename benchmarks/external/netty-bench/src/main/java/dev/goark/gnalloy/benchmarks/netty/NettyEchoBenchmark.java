package dev.goark.gnalloy.benchmarks.netty;

import java.net.InetSocketAddress;

public final class NettyEchoBenchmark {
    private NettyEchoBenchmark() {
    }

    public static void main(String[] args) throws Exception {
        Config config = Config.parse(args);
        BenchmarkResult result = run(config);
        if (result.totalRequests() > 0) {
            BenchmarkOutput.write(config, result);
        }
    }

    static BenchmarkResult run(Config config) throws Exception {
        config.validate();
        if (config.udpEcho()) {
            try (DatagramEchoServer server = DatagramEchoServer.start(config)) {
                return DatagramLoadGenerator.run(server.address(), config);
            }
        }
        if (config.http3Family()) {
            try (Http3Server server = Http3Server.start(config)) {
                return Http3LoadGenerator.run(server.address(), config);
            }
        }
        try (EchoServer server = EchoServer.start(config)) {
            InetSocketAddress address = server.address();
            if (config.http1Family()) {
                return Http1LoadGenerator.run(address, config);
            }
            if (config.http2Family()) {
                return Http2LoadGenerator.run(address, config);
            }
            return LoadGenerator.run(address, config);
        }
    }
}
