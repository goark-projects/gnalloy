#!/usr/bin/env sh
set -eu

BACKEND="${BACKEND:-default}"
ADDR="${ADDR:-127.0.0.1:0}"
BOSS="${BOSS:-1}"
WORKERS="${WORKERS:-2}"
CONNECTIONS="${CONNECTIONS:-32}"
MESSAGES="${MESSAGES:-32}"
PAYLOAD_SIZE="${PAYLOAD_SIZE:-64}"
SCENARIO="${SCENARIO:-mixed}"
PROTOCOL="${PROTOCOL:-both}"
REUSEPORT="${REUSEPORT:-0}"
MMAP="${MMAP:-0}"
MMAP_BLOCK_SIZE="${MMAP_BLOCK_SIZE:-4096}"
MMAP_BLOCKS="${MMAP_BLOCKS:-4096}"
IOURING_SQPOLL="${IOURING_SQPOLL:-0}"
IOURING_MULTISHOT_ACCEPT="${IOURING_MULTISHOT_ACCEPT:-0}"
IOURING_FIXED_BUFFERS="${IOURING_FIXED_BUFFERS:-0}"
IOURING_ENTRIES="${IOURING_ENTRIES:-0}"

REPO="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/gnalloy-stress.XXXXXX")"

cleanup() {
    rm -rf "${TEMP_DIR}"
}
trap cleanup EXIT INT TERM

cd "${REPO}"
export GOWORK=off

go build -o "${TEMP_DIR}/stress-check" ./examples/stress-check

args="-addr ${ADDR} -backend ${BACKEND} -boss ${BOSS} -workers ${WORKERS} -protocol ${PROTOCOL} -scenario ${SCENARIO} -connections ${CONNECTIONS} -messages ${MESSAGES} -payload-size ${PAYLOAD_SIZE}"
if [ "${REUSEPORT}" = "1" ]; then
    args="${args} -reuseport"
fi
if [ "${MMAP}" = "1" ]; then
    args="${args} -mmap -mmap-block-size ${MMAP_BLOCK_SIZE} -mmap-blocks ${MMAP_BLOCKS}"
fi
if [ "${IOURING_SQPOLL}" = "1" ]; then
    args="${args} -iouring-sqpoll"
fi
if [ "${IOURING_MULTISHOT_ACCEPT}" = "1" ]; then
    args="${args} -iouring-multishot-accept"
fi
if [ "${IOURING_FIXED_BUFFERS}" = "1" ]; then
    args="${args} -iouring-fixed-buffers"
fi
if [ "${IOURING_ENTRIES}" != "0" ]; then
    args="${args} -iouring-entries ${IOURING_ENTRIES}"
fi

# shellcheck disable=SC2086
"${TEMP_DIR}/stress-check" ${args}
echo "stress ok backend=${BACKEND} workers=${WORKERS} connections=${CONNECTIONS} messages=${MESSAGES} scenario=${SCENARIO} leaks=0"
