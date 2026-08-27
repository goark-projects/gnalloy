package dev.goark.gnalloy.benchmarks.netty;

import java.util.Locale;

enum BackendKind {
    NIO("nio"),
    EPOLL("epoll");

    private final String wireName;

    BackendKind(String wireName) {
        this.wireName = wireName;
    }

    String wireName() {
        return wireName;
    }

    static BackendKind parse(String value) {
        String normalized = value == null || value.isBlank()
                ? NIO.wireName
                : value.toLowerCase(Locale.ROOT);
        for (BackendKind backend : values()) {
            if (backend.wireName.equals(normalized)) {
                return backend;
            }
        }
        throw new IllegalArgumentException("netty-bench: unsupported backend " + value);
    }
}
