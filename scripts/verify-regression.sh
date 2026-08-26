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
run_fuzz_smoke "quic frame scanner" "FuzzQUICFrameScanner" "./transport/quic"

echo "== benchmarks: hot path allocation guards"
"$GO_CMD" test ./buffer ./codec ./queue ./timer ./observability \
    -run '^$' \
    -bench 'Benchmark(HeapAllocatorAcquireRelease|MmapAllocatorAcquireRelease|FixedLengthFrameDecoder|LineBasedFrameDecoder|DelimiterBasedFrameDecoder|ByteToMessageListDecoder|MPSCOfferPoll|WheelScheduleAdvance|AtomicChannelRecorderRead)$' \
    -benchmem \
    -benchtime "$BENCHTIME" \
    -count="$COUNT"

if [ "$(uname -s)" = "Linux" ]; then
    echo "== tests: io_uring fixed buffers"
    "$GO_CMD" test ./transport/poller/iouring -run 'Test(RegisterBuffersStatsAndUnregister|RegisterMmapAllocatorFixedBuffers)' -count="$COUNT"
fi
