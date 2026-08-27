package dev.goark.gnalloy.benchmarks.netty;

record HostPort(String host, int port) {
    static HostPort parse(String addr) {
        int sep = addr.lastIndexOf(':');
        if (sep <= 0 || sep == addr.length() - 1) {
            throw new IllegalArgumentException("netty-bench: invalid addr " + addr);
        }
        return new HostPort(addr.substring(0, sep), Integer.parseInt(addr.substring(sep + 1)));
    }
}
