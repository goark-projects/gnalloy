package dev.goark.gnalloy.benchmarks.netty;

import io.netty.handler.ssl.ApplicationProtocolConfig;
import io.netty.handler.ssl.SslContext;
import io.netty.handler.ssl.SslContextBuilder;
import io.netty.handler.ssl.util.SelfSignedCertificate;

import javax.net.ssl.SNIHostName;
import javax.net.ssl.SSLContext;
import javax.net.ssl.SSLParameters;
import javax.net.ssl.SSLSocket;
import javax.net.ssl.TrustManager;
import javax.net.ssl.X509TrustManager;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.security.Security;
import java.security.cert.X509Certificate;
import java.util.Arrays;
import java.util.List;
import java.util.stream.Collectors;

final class SslSupport {
    private static final String SERVER_NAME = "gnalloy.local";
    private static final X509TrustManager TRUST_ALL = new X509TrustManager() {
        @Override
        public void checkClientTrusted(X509Certificate[] chain, String authType) {
        }

        @Override
        public void checkServerTrusted(X509Certificate[] chain, String authType) {
        }

        @Override
        public X509Certificate[] getAcceptedIssuers() {
            return new X509Certificate[0];
        }
    };

    private SslSupport() {
    }

    static SslContext serverContext(Config config) throws Exception {
        if (!config.tlsEnabled()) {
            return null;
        }
        enableLegacyTLSIfRequested(config.tlsVersion());
        SelfSignedCertificate certificate = new SelfSignedCertificate(SERVER_NAME);
        SslContextBuilder builder = SslContextBuilder
                .forServer(certificate.certificate(), certificate.privateKey())
                .protocols(config.tlsVersion().protocolName());
        List<String> protocols = config.alpnProtocols();
        if (!protocols.isEmpty()) {
            builder.applicationProtocolConfig(new ApplicationProtocolConfig(
                    ApplicationProtocolConfig.Protocol.ALPN,
                    ApplicationProtocolConfig.SelectorFailureBehavior.NO_ADVERTISE,
                    ApplicationProtocolConfig.SelectedListenerFailureBehavior.ACCEPT,
                    protocols));
        }
        List<String> cipherSuites = config.cipherSuiteList();
        if (!cipherSuites.isEmpty()) {
            builder.ciphers(cipherSuites);
        }
        return builder.build();
    }

    static Socket connect(InetSocketAddress address, Config config) throws Exception {
        if (!config.tlsEnabled()) {
            Socket socket = new Socket();
            socket.setTcpNoDelay(true);
            socket.connect(address, Math.toIntExact(config.timeout().toMillis()));
            socket.setSoTimeout(Math.toIntExact(config.timeout().toMillis()));
            return socket;
        }
        enableLegacyTLSIfRequested(config.tlsVersion());
        SSLContext context = SSLContext.getInstance("TLS");
        context.init(null, new TrustManager[]{TRUST_ALL}, null);
        SSLSocket socket = (SSLSocket) context.getSocketFactory().createSocket();
        socket.setUseClientMode(true);
        socket.setTcpNoDelay(true);
        socket.setEnabledProtocols(new String[]{config.tlsVersion().protocolName()});
        List<String> cipherSuites = config.cipherSuiteList();
        if (!cipherSuites.isEmpty()) {
            socket.setEnabledCipherSuites(cipherSuites.toArray(String[]::new));
        }
        configureClientParameters(socket, config.alpnProtocols());
        socket.connect(address, Math.toIntExact(config.timeout().toMillis()));
        socket.setSoTimeout(Math.toIntExact(config.timeout().toMillis()));
        socket.startHandshake();
        return socket;
    }

    static String negotiatedProtocol(Socket socket) {
        if (socket instanceof SSLSocket sslSocket) {
            return sslSocket.getApplicationProtocol();
        }
        return "";
    }

    private static void configureClientParameters(SSLSocket socket, List<String> protocols) {
        SSLParameters parameters = socket.getSSLParameters();
        parameters.setServerNames(List.of(new SNIHostName(SERVER_NAME)));
        if (!protocols.isEmpty()) {
            parameters.setApplicationProtocols(protocols.toArray(String[]::new));
        }
        socket.setSSLParameters(parameters);
    }

    private static void enableLegacyTLSIfRequested(TlsVersion version) {
        if (version != TlsVersion.TLS11) {
            return;
        }
        String disabled = Security.getProperty("jdk.tls.disabledAlgorithms");
        if (disabled == null || disabled.isBlank()) {
            return;
        }
        String updated = Arrays.stream(disabled.split(","))
                .map(String::trim)
                .filter(rule -> !rule.equalsIgnoreCase("TLSv1.1"))
                .collect(Collectors.joining(", "));
        Security.setProperty("jdk.tls.disabledAlgorithms", updated);
    }
}
