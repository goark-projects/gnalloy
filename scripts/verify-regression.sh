#!/usr/bin/env sh
set -eu

BENCHTIME="${BENCHTIME:-100ms}"
COUNT="${COUNT:-1}"
FUZZTIME="${FUZZTIME:-2s}"
GO_CMD="${GO:-go}"
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
REPO="$(dirname "$SCRIPT_DIR")"
OLD_GOWORK="${GOWORK-}"

restore_env() {
    if [ -z "$OLD_GOWORK" ]; then
        unset GOWORK
    else
        GOWORK="$OLD_GOWORK"
        export GOWORK
    fi
}

trap restore_env EXIT

cd "$REPO"
GOWORK=off
export GOWORK

run_fuzz_smoke() {
    name="$1"
    target="$2"
    package="$3"
    if [ -z "$FUZZTIME" ]; then
        return 0
    fi
    echo "== fuzz: ${name}"
    "$GO_CMD" test -run '^$' -fuzz "$target" -fuzztime "$FUZZTIME" "$package"
}

echo "== tests: full suite"
"$GO_CMD" test ./... -count="$COUNT"

run_fuzz_smoke "codec length-field" "FuzzLengthFieldBasedFrameDecoder" "./codec"
run_fuzz_smoke "http1 request" "FuzzHTTP1RequestDecoder" "./codec/http1"
run_fuzz_smoke "websocket frame" "FuzzWebSocketFrameDecoder" "./codec/websocket"
run_fuzz_smoke "mqtt frame pipeline" "FuzzMQTTFramePipeline" "./codec/mqtt"
run_fuzz_smoke "dns message" "FuzzDNSParseMessage" "./codec/dns"
run_fuzz_smoke "redis frame pipeline" "FuzzRedisFramePipeline" "./codec/redis"
run_fuzz_smoke "http2 frame pipeline" "FuzzHTTP2FramePipeline" "./codec/http2"
run_fuzz_smoke "http3 frame pipeline" "FuzzHTTP3FramePipeline" "./codec/http3"

echo "== benchmarks: hot path allocation guards"
"$GO_CMD" test ./buffer ./channel/pool ./codec ./codec/http2 ./handler/ipfilter ./handler/pcap ./queue ./timer ./observability \
    -run '^$' \
    -bench 'Benchmark(HeapAllocatorAcquireRelease|PooledAllocatorAcquireRelease|MmapAllocatorAcquireRelease|FixedPoolGetPut|ChannelPoolMapGet|FixedLengthFrameDecoder|LineBasedFrameDecoder|DelimiterBasedFrameDecoder|ByteToMessageListDecoder|StreamMultiplexerReadData|IPFilterAllowedDatagram|PCAPCaptureByteBuf|MPSCOfferPoll|WheelScheduleAdvance|AtomicChannelRecorderRead|PrometheusExporter)$' \
    -benchmem \
    -benchtime "$BENCHTIME" \
    -count="$COUNT"

if [ "$(uname -s)" = "Linux" ]; then
    echo "== tests: io_uring fixed buffers"
    "$GO_CMD" test ./transport/poller/iouring -run 'Test(RegisterBuffersStatsAndUnregister|RegisterMmapAllocatorFixedBuffers)' -count="$COUNT"
fi
