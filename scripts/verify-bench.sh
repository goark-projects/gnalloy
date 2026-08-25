#!/usr/bin/env sh
set -eu

BACKENDS="${BACKENDS:-default}"
WORKERS="${WORKERS:-0}"
IOURING_ENTRIES="${IOURING_ENTRIES:-0}"
IOURING_SQPOLL="${IOURING_SQPOLL:-0}"
IOURING_MULTISHOT_ACCEPT="${IOURING_MULTISHOT_ACCEPT:-0}"
IOURING_FIXED_BUFFERS="${IOURING_FIXED_BUFFERS:-0}"
MMAP="${MMAP:-0}"
MMAP_BLOCK_SIZE="${MMAP_BLOCK_SIZE:-4096}"
MMAP_BLOCKS="${MMAP_BLOCKS:-4096}"

REPO="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "${REPO}"
export GOWORK=off

echo "== benchmarks: core packages"
go test -run '^$' -bench . -benchmem ./buffer ./channel ./codec ./queue ./timer

old_ifs="${IFS}"
IFS=','
for backend in ${BACKENDS}; do
    IFS="${old_ifs}"
    backend="$(printf '%s' "${backend}" | tr -d '[:space:]')"
    if [ -z "${backend}" ]; then
        continue
    fi
    echo "== benchmarks: transport/tcp backend=${backend}"
    GNALLOY_BENCH_BACKEND="${backend}" \
    GNALLOY_BENCH_WORKERS="${WORKERS}" \
    GNALLOY_BENCH_MMAP="${MMAP}" \
    GNALLOY_BENCH_MMAP_BLOCK_SIZE="${MMAP_BLOCK_SIZE}" \
    GNALLOY_BENCH_MMAP_BLOCKS="${MMAP_BLOCKS}" \
    GNALLOY_BENCH_IOURING_ENTRIES="${IOURING_ENTRIES}" \
    GNALLOY_BENCH_IOURING_SQPOLL="${IOURING_SQPOLL}" \
    GNALLOY_BENCH_IOURING_MULTISHOT_ACCEPT="${IOURING_MULTISHOT_ACCEPT}" \
    GNALLOY_BENCH_IOURING_FIXED_BUFFERS="${IOURING_FIXED_BUFFERS}" \
    go test -run '^$' -bench . -benchmem ./transport/tcp
    IFS=','
done
IFS="${old_ifs}"
