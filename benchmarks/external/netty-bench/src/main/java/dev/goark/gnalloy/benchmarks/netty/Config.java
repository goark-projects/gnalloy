package dev.goark.gnalloy.benchmarks.netty;

import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Objects;

record Config(
        String protocol,
        BackendKind backend,
        String host,
        int port,
        int payload,
        int connections,
        int messages,
        Duration timeout,
        int eventLoops,
        int latencySampleRate,
        int warmupMessages,
        TlsVersion tlsVersion,
        String alpn,
        String cipherSuites) {

    static Config parse(String[] args) {
        Map<String, String> values = Args.parse(args);
        HostPort hostPort = HostPort.parse(values.getOrDefault("addr", "127.0.0.1:0"));
        return new Config(
                values.getOrDefault("protocol", "tcp-echo"),
                BackendKind.parse(values.get("backend")),
                hostPort.host(),
                hostPort.port(),
                Args.intValue(values, "payload", 1024),
                Args.intValue(values, "connections", 256),
                Args.intValue(values, "messages", 100000),
                durationValue(values, "timeout", Duration.ofMinutes(5)),
                Args.intValue(values, "event-loops", Runtime.getRuntime().availableProcessors()),
                Args.intValue(values, "latency-sample-rate", 0),
                Args.intValue(values, "warmup-messages", 0),
                TlsVersion.parse(values.getOrDefault("tls-version", "1.3")),
                defaultAlpn(values),
                values.getOrDefault("cipher-suites", ""));
    }

    void validate() {
        if (!Objects.equals(protocol, "tcp-echo")
                && !Objects.equals(protocol, "udp-echo")
                && !Objects.equals(protocol, "http1")
                && !Objects.equals(protocol, "https1")
                && !Objects.equals(protocol, "http2")
                && !Objects.equals(protocol, "https2")
                && !Objects.equals(protocol, "http3")) {
            throw new IllegalArgumentException("netty-bench: unsupported protocol " + protocol);
        }
        if (backend == null) {
            throw new IllegalArgumentException("netty-bench: missing backend");
        }
        if (host == null || host.isBlank()) {
            throw new IllegalArgumentException("netty-bench: empty host");
        }
        if (port < 0 || port > 65535) {
            throw new IllegalArgumentException("netty-bench: invalid port " + port);
        }
        if (payload <= 0 || connections <= 0 || messages <= 0) {
            throw new IllegalArgumentException("netty-bench: payload, connections and messages must be positive");
        }
        if (timeout == null || timeout.isZero() || timeout.isNegative()) {
            throw new IllegalArgumentException("netty-bench: timeout must be positive");
        }
        if (eventLoops <= 0) {
            throw new IllegalArgumentException("netty-bench: event-loops must be positive");
        }
        if (latencySampleRate < 0) {
            throw new IllegalArgumentException("netty-bench: latency-sample-rate must not be negative");
        }
        if (warmupMessages < 0) {
            throw new IllegalArgumentException("netty-bench: warmup-messages must not be negative");
        }
        if (Objects.equals(protocol, "https2") && tlsVersion == TlsVersion.TLS11) {
            throw new IllegalArgumentException("netty-bench: HTTP/2 over TLS requires TLS 1.2 or newer");
        }
        if (http3Family() && tlsVersion != TlsVersion.TLS13) {
            throw new IllegalArgumentException("netty-bench: HTTP/3 requires TLS 1.3");
        }
        if (http3Family() && !alpnProtocols().contains("h3")) {
            throw new IllegalArgumentException("netty-bench: HTTP/3 requires ALPN h3");
        }
        if (!tlsEnabled() && !cipherSuiteList().isEmpty()) {
            throw new IllegalArgumentException("netty-bench: cipher-suites require TLS");
        }
        if (http3Family() && !cipherSuiteList().isEmpty()) {
            throw new IllegalArgumentException("netty-bench: HTTP/3 cipher suites are provider-managed");
        }
    }

    boolean http1Family() {
        return Objects.equals(protocol, "http1") || Objects.equals(protocol, "https1");
    }

    boolean http2Family() {
        return Objects.equals(protocol, "http2") || Objects.equals(protocol, "https2");
    }

    boolean http3Family() {
        return Objects.equals(protocol, "http3");
    }

    boolean udpEcho() {
        return Objects.equals(protocol, "udp-echo");
    }

    boolean tlsEnabled() {
        return Objects.equals(protocol, "https1") || Objects.equals(protocol, "https2") || http3Family();
    }

    List<String> alpnProtocols() {
        if (alpn == null || alpn.isBlank()) {
            return List.of();
        }
        return splitNames(alpn);
    }

    List<String> cipherSuiteList() {
        if (cipherSuites == null || cipherSuites.isBlank()) {
            return List.of();
        }
        return splitNames(cipherSuites);
    }

    String cipherSuiteOutput() {
        List<String> suites = cipherSuiteList();
        if (suites.isEmpty()) {
            return "";
        }
        return String.join(",", suites);
    }

    private static List<String> splitNames(String value) {
        String[] parts = value.split("[,;:]");
        List<String> protocols = new ArrayList<>(parts.length);
        for (String part : parts) {
            String protocol = part.trim();
            if (!protocol.isEmpty()) {
                protocols.add(protocol);
            }
        }
        return protocols;
    }

    private static String defaultAlpn(Map<String, String> values) {
        String configured = values.get("alpn");
        if (configured != null) {
            return configured;
        }
        if (Objects.equals(values.get("protocol"), "https2")) {
            return "h2";
        }
        if (Objects.equals(values.get("protocol"), "http3")) {
            return "h3";
        }
        return "http/1.1";
    }

    private static Duration durationValue(Map<String, String> values, String key, Duration fallback) {
        String value = values.get(key);
        if (value == null || value.isBlank()) {
            return fallback;
        }
        if (value.endsWith("ms")) {
            return Duration.ofMillis(Long.parseLong(value.substring(0, value.length() - 2)));
        }
        if (value.endsWith("s")) {
            return Duration.ofSeconds(Long.parseLong(value.substring(0, value.length() - 1)));
        }
        if (value.endsWith("m")) {
            return Duration.ofMinutes(Long.parseLong(value.substring(0, value.length() - 1)));
        }
        return Duration.ofMillis(Long.parseLong(value));
    }
}
