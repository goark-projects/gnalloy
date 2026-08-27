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
        try (EchoServer server = EchoServer.start(config)) {
            InetSocketAddress address = server.address();
            return LoadGenerator.run(address, config);
        }
    }
}
