#!/usr/bin/env sh
set -eu

BENCHTIME="${BENCHTIME:-100ms}"
COUNT="${COUNT:-1}"
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

echo "== tests: full suite"
"$GO_CMD" test ./... -count="$COUNT"

echo "== benchmarks: hot path allocation guards"
"$GO_CMD" test ./buffer ./codec ./queue ./timer \
    -run '^$' \
    -bench 'Benchmark(HeapAllocatorAcquireRelease|MmapAllocatorAcquireRelease|FixedLengthFrameDecoder|LineBasedFrameDecoder|DelimiterBasedFrameDecoder|ByteToMessageListDecoder|MPSCOfferPoll|WheelScheduleAdvance)$' \
    -benchmem \
    -benchtime "$BENCHTIME" \
    -count="$COUNT"

if [ "$(uname -s)" = "Linux" ]; then
    echo "== tests: io_uring fixed buffers"
    "$GO_CMD" test ./transport/poller/iouring -run 'Test(RegisterBuffersStatsAndUnregister|RegisterMmapAllocatorFixedBuffers)' -count="$COUNT"
fi
