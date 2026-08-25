#!/usr/bin/env sh
set -eu

BACKENDS="${BACKENDS:-default}"
GROUPS="${GROUPS:-all}"
WORKERS="${WORKERS:-0}"
COUNT="${COUNT:-1}"
BENCHTIME="${BENCHTIME:-}"
IOURING_ENTRIES="${IOURING_ENTRIES:-0}"
IOURING_SQPOLL="${IOURING_SQPOLL:-0}"
IOURING_MULTISHOT_ACCEPT="${IOURING_MULTISHOT_ACCEPT:-0}"
IOURING_FIXED_BUFFERS="${IOURING_FIXED_BUFFERS:-0}"
MMAP="${MMAP:-0}"
MMAP_BLOCK_SIZE="${MMAP_BLOCK_SIZE:-4096}"
MMAP_BLOCKS="${MMAP_BLOCKS:-4096}"
WRITE_HIGH_WATERMARK="${WRITE_HIGH_WATERMARK:-0}"
WRITE_LOW_WATERMARK="${WRITE_LOW_WATERMARK:-0}"

REPO="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "${REPO}"
export GOWORK=off

has_group() {
    group_name="$1"
    groups="$(printf '%s' "${GROUPS}" | tr '[:upper:]' '[:lower:]' | tr -d '[:space:]')"
    case ",${groups}," in
        *",all,"*|*",${group_name},"*) return 0 ;;
        *) return 1 ;;
    esac
}

run_bench() {
    group_name="$1"
    bench="$2"
    shift 2
    if ! has_group "${group_name}"; then
        return 0
    fi
    echo "== benchmarks: ${group_name}"
    if [ -n "${BENCHTIME}" ]; then
        go test -run '^$' -bench "${bench}" -benchmem -count "${COUNT}" -benchtime "${BENCHTIME}" "$@"
    else
        go test -run '^$' -bench "${bench}" -benchmem -count "${COUNT}" "$@"
    fi
}

run_bench "buffer" "." ./buffer
run_bench "pipeline" "." ./channel
run_bench "codec" 'Benchmark(LengthFieldDecoder|FixedLengthFrameDecoder|LineBasedFrameDecoder|DelimiterBasedFrameDecoder|ByteToMessageListDecoder)$' ./codec
run_bench "codec-protocol" "." ./codec/icmp ./codec/ip
run_bench "queue" "." ./queue
run_bench "timer" "." ./timer
run_bench "quic" "." ./transport/quic
run_bench "udp" "." ./transport/udp
run_bench "raw" "." ./transport/raw

old_ifs="${IFS}"
IFS=','
if has_group "tcp" || has_group "tcp-echo" || has_group "length-field"; then
    for backend in ${BACKENDS}; do
        IFS="${old_ifs}"
        backend="$(printf '%s' "${backend}" | tr -d '[:space:]')"
        if [ -z "${backend}" ]; then
            IFS=','
            continue
        fi
        export GNALLOY_BENCH_BACKEND="${backend}"
        export GNALLOY_BENCH_WORKERS="${WORKERS}"
        export GNALLOY_BENCH_MMAP="${MMAP}"
        export GNALLOY_BENCH_MMAP_BLOCK_SIZE="${MMAP_BLOCK_SIZE}"
        export GNALLOY_BENCH_MMAP_BLOCKS="${MMAP_BLOCKS}"
        export GNALLOY_BENCH_WRITE_HIGH_WATERMARK="${WRITE_HIGH_WATERMARK}"
        export GNALLOY_BENCH_WRITE_LOW_WATERMARK="${WRITE_LOW_WATERMARK}"
        export GNALLOY_BENCH_IOURING_ENTRIES="${IOURING_ENTRIES}"
        export GNALLOY_BENCH_IOURING_SQPOLL="${IOURING_SQPOLL}"
        export GNALLOY_BENCH_IOURING_MULTISHOT_ACCEPT="${IOURING_MULTISHOT_ACCEPT}"
        export GNALLOY_BENCH_IOURING_FIXED_BUFFERS="${IOURING_FIXED_BUFFERS}"
        if has_group "tcp"; then
            run_bench "tcp" "." ./transport/tcp
        else
            run_bench "tcp-echo" 'BenchmarkNativeTCPEchoRoundTrip$' ./transport/tcp
            run_bench "length-field" 'BenchmarkLengthFieldTCPRoundTrip$' ./transport/tcp
        fi
        IFS=','
    done
fi
IFS="${old_ifs}"
