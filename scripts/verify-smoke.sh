#!/usr/bin/env sh
set -eu

BACKEND="${BACKEND:-default}"
ECHO_ADDR="${ECHO_ADDR:-127.0.0.1:19000}"
LENGTH_FIELD_ADDR="${LENGTH_FIELD_ADDR:-127.0.0.1:19001}"
UDP_ADDR="${UDP_ADDR:-127.0.0.1:19002}"
LINE_ADDR="${LINE_ADDR:-127.0.0.1:19003}"
FIXED_ADDR="${FIXED_ADDR:-127.0.0.1:19004}"
ICMP_TARGET="${ICMP_TARGET:-127.0.0.1}"
WORKERS="${WORKERS:-2}"
COUNT="${COUNT:-3}"
REUSEPORT="${REUSEPORT:-0}"
MMAP="${MMAP:-0}"
MMAP_BLOCK_SIZE="${MMAP_BLOCK_SIZE:-4096}"
MMAP_BLOCKS="${MMAP_BLOCKS:-4096}"
IOURING_FIXED_BUFFERS="${IOURING_FIXED_BUFFERS:-0}"
RUN_RAW="${RUN_RAW:-0}"
WRITE_HIGH_WATERMARK="${WRITE_HIGH_WATERMARK:-0}"
WRITE_LOW_WATERMARK="${WRITE_LOW_WATERMARK:-0}"

REPO="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/gnalloy-smoke.XXXXXX")"
SERVER_PID=""

cleanup() {
    if [ -n "${SERVER_PID}" ] && kill -0 "${SERVER_PID}" 2>/dev/null; then
        kill "${SERVER_PID}" 2>/dev/null || true
        wait "${SERVER_PID}" 2>/dev/null || true
    fi
    rm -rf "${TEMP_DIR}"
}
trap cleanup EXIT INT TERM

server_args() {
    printf '%s\n' "-backend" "${BACKEND}" "-workers" "${WORKERS}"
    if [ "${REUSEPORT}" = "1" ]; then
        printf '%s\n' "-reuseport"
    fi
    if [ "${MMAP}" = "1" ]; then
        printf '%s\n' "-mmap" "-mmap-block-size" "${MMAP_BLOCK_SIZE}" "-mmap-blocks" "${MMAP_BLOCKS}"
    fi
    if [ "${WRITE_HIGH_WATERMARK}" != "0" ]; then
        printf '%s\n' "-write-high-watermark" "${WRITE_HIGH_WATERMARK}"
    fi
    if [ "${WRITE_LOW_WATERMARK}" != "0" ]; then
        printf '%s\n' "-write-low-watermark" "${WRITE_LOW_WATERMARK}"
    fi
    if [ "${IOURING_FIXED_BUFFERS}" = "1" ]; then
        printf '%s\n' "-iouring-fixed-buffers"
    fi
}

run_server() {
    exe="$1"
    addr="$2"
    protocol="$3"
    "$exe" $(server_args) -addr "$addr" >"${TEMP_DIR}/server.out" 2>"${TEMP_DIR}/server.err" &
    SERVER_PID="$!"
    sleep 1
    if ! kill -0 "${SERVER_PID}" 2>/dev/null; then
        cat "${TEMP_DIR}/server.out" || true
        cat "${TEMP_DIR}/server.err" || true
        echo "server exited early" >&2
        exit 1
    fi
    "${TEMP_DIR}/smoke-client" -addr "$addr" -protocol "$protocol" -message ping -count "$COUNT"
    kill "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
    SERVER_PID=""
}

cd "${REPO}"
export GOWORK=off

go build -o "${TEMP_DIR}/echo" ./examples/echo
go build -o "${TEMP_DIR}/length-field" ./examples/length-field
go build -o "${TEMP_DIR}/udp-echo" ./examples/udp-echo
go build -o "${TEMP_DIR}/line-frame" ./examples/line-frame
go build -o "${TEMP_DIR}/fixed-length" ./examples/fixed-length
if [ "${RUN_RAW}" = "1" ]; then
    go build -o "${TEMP_DIR}/icmp-ping" ./examples/icmp-ping
fi
go build -o "${TEMP_DIR}/smoke-client" ./examples/smoke-client

run_server "${TEMP_DIR}/echo" "${ECHO_ADDR}" raw
run_server "${TEMP_DIR}/length-field" "${LENGTH_FIELD_ADDR}" length-field
run_server "${TEMP_DIR}/udp-echo" "${UDP_ADDR}" udp
run_server "${TEMP_DIR}/line-frame" "${LINE_ADDR}" line
"${TEMP_DIR}/fixed-length" $(server_args) -addr "${FIXED_ADDR}" -frame-length 4 >"${TEMP_DIR}/server.out" 2>"${TEMP_DIR}/server.err" &
SERVER_PID="$!"
sleep 1
if ! kill -0 "${SERVER_PID}" 2>/dev/null; then
    cat "${TEMP_DIR}/server.out" || true
    cat "${TEMP_DIR}/server.err" || true
    echo "server exited early" >&2
    exit 1
fi
"${TEMP_DIR}/smoke-client" -addr "${FIXED_ADDR}" -protocol fixed -message ping -count "${COUNT}"
kill "${SERVER_PID}" 2>/dev/null || true
wait "${SERVER_PID}" 2>/dev/null || true
SERVER_PID=""

if [ "${RUN_RAW}" = "1" ]; then
    "${TEMP_DIR}/icmp-ping" $(server_args) -target "${ICMP_TARGET}" -timeout 3s
fi

echo "smoke ok backend=${BACKEND} workers=${WORKERS} count=${COUNT} raw=${RUN_RAW}"
