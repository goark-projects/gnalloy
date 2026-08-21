#!/usr/bin/env sh
set -eu

BACKEND="${BACKEND:-default}"
ECHO_ADDR="${ECHO_ADDR:-127.0.0.1:19000}"
LENGTH_FIELD_ADDR="${LENGTH_FIELD_ADDR:-127.0.0.1:19001}"
WORKERS="${WORKERS:-2}"
COUNT="${COUNT:-3}"
REUSEPORT="${REUSEPORT:-0}"
MMAP="${MMAP:-0}"

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
        printf '%s\n' "-mmap"
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
go build -o "${TEMP_DIR}/smoke-client" ./examples/smoke-client

run_server "${TEMP_DIR}/echo" "${ECHO_ADDR}" raw
run_server "${TEMP_DIR}/length-field" "${LENGTH_FIELD_ADDR}" length-field

echo "smoke ok backend=${BACKEND} workers=${WORKERS} count=${COUNT}"
