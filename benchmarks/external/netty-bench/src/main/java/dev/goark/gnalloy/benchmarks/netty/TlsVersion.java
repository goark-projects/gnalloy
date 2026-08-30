package dev.goark.gnalloy.benchmarks.netty;

enum TlsVersion {
    TLS11("1.1", "TLSv1.1"),
    TLS12("1.2", "TLSv1.2"),
    TLS13("1.3", "TLSv1.3");

    private final String id;
    private final String protocolName;

    TlsVersion(String id, String protocolName) {
        this.id = id;
        this.protocolName = protocolName;
    }

    String id() {
        return id;
    }

    String protocolName() {
        return protocolName;
    }

    static TlsVersion parse(String value) {
        String normalized = value == null || value.isBlank() ? "1.3" : value.trim();
        for (TlsVersion version : values()) {
            if (version.id.equals(normalized)) {
                return version;
            }
        }
        throw new IllegalArgumentException("netty-bench: unsupported tls version " + value);
    }
}
