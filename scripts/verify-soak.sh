#!/usr/bin/env sh
set -eu

BACKEND="${BACKEND:-default}"
ADDR="${ADDR:-127.0.0.1:0}"
BOSS="${BOSS:-1}"
WORKERS="${WORKERS:-2}"
PROTOCOL="${PROTOCOL:-both}"
SCENARIO="${SCENARIO:-mixed}"
CONNECTIONS="${CONNECTIONS:-16}"
MESSAGES="${MESSAGES:-16}"
PAYLOAD_SIZE="${PAYLOAD_SIZE:-64}"
DURATION_SECONDS="${DURATION_SECONDS:-${GNALLOY_SOAK_DURATION_SECONDS:-0}}"
MIN_CYCLES="${MIN_CYCLES:-${GNALLOY_SOAK_MIN_CYCLES:-1}}"
TIMEOUT="${TIMEOUT:-30s}"
DELAY="${DELAY:-1ms}"
DRAIN_TIMEOUT="${DRAIN_TIMEOUT:-5s}"
REPORT_PATH="${REPORT_PATH:-}"
REUSEPORT="${REUSEPORT:-0}"
MMAP="${MMAP:-0}"
MMAP_BLOCK_SIZE="${MMAP_BLOCK_SIZE:-4096}"
MMAP_BLOCKS="${MMAP_BLOCKS:-4096}"
IOURING_SQPOLL="${IOURING_SQPOLL:-0}"
IOURING_MULTISHOT_ACCEPT="${IOURING_MULTISHOT_ACCEPT:-0}"
IOURING_FIXED_BUFFERS="${IOURING_FIXED_BUFFERS:-0}"
IOURING_ENTRIES="${IOURING_ENTRIES:-0}"

REPO="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/gnalloy-soak.XXXXXX")"
RESULTS_FILE="${TEMP_DIR}/cycles.jsonl"

cleanup() {
    rm -rf "${TEMP_DIR}"
}
trap cleanup EXIT INT TERM

if [ "${MIN_CYCLES}" -le 0 ]; then
    MIN_CYCLES=1
fi

now_epoch() {
    date +%s
}

json_escape() {
    printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

append_report() {
    if [ -z "${REPORT_PATH}" ]; then
        return
    fi
    {
        printf '{\n'
        printf '  "generatedAt": "%s",\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
        printf '  "backend": "%s",\n' "$(json_escape "${BACKEND}")"
        printf '  "protocol": "%s",\n' "$(json_escape "${PROTOCOL}")"
        printf '  "scenario": "%s",\n' "$(json_escape "${SCENARIO}")"
        printf '  "connections": %s,\n' "${CONNECTIONS}"
        printf '  "messages": %s,\n' "${MESSAGES}"
        printf '  "payloadSize": %s,\n' "${PAYLOAD_SIZE}"
        printf '  "durationSeconds": %s,\n' "${DURATION_SECONDS}"
        printf '  "minCycles": %s,\n' "${MIN_CYCLES}"
        printf '  "cycles": [\n'
        if [ -s "${RESULTS_FILE}" ]; then
            sed '$!s/$/,/' "${RESULTS_FILE}"
        fi
        printf '  ]\n'
        printf '}\n'
    } > "${REPORT_PATH}"
}

build_args() {
    args="-addr ${ADDR} -backend ${BACKEND} -boss ${BOSS} -workers ${WORKERS} -protocol ${PROTOCOL} -scenario ${SCENARIO} -connections ${CONNECTIONS} -messages ${MESSAGES} -payload-size ${PAYLOAD_SIZE} -timeout ${TIMEOUT} -delay ${DELAY} -drain-timeout ${DRAIN_TIMEOUT}"
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
    printf '%s' "${args}"
}

cd "${REPO}"
export GOWORK=off

go build -o "${TEMP_DIR}/stress-check" ./examples/stress-check

start="$(now_epoch)"
deadline=0
if [ "${DURATION_SECONDS}" -gt 0 ]; then
    deadline=$((start + DURATION_SECONDS))
fi

cycle=0
while :; do
    cycle=$((cycle + 1))
    cycle_start="$(now_epoch)"
    echo "== soak cycle ${cycle} backend=${BACKEND} protocol=${PROTOCOL} scenario=${SCENARIO} connections=${CONNECTIONS} messages=${MESSAGES}"
    # shellcheck disable=SC2086
    output="$("${TEMP_DIR}/stress-check" $(build_args))"
    echo "${output}"
    cycle_elapsed=$((($(now_epoch) - cycle_start) * 1000))
    if [ -n "${REPORT_PATH}" ]; then
        escaped_output="$(json_escape "${output}")"
        printf '    {"index": %s, "status": "passed", "output": "%s", "durationMs": %s}\n' "${cycle}" "${escaped_output}" "${cycle_elapsed}" >> "${RESULTS_FILE}"
    fi
    if [ "${cycle}" -ge "${MIN_CYCLES}" ]; then
        if [ "${deadline}" -eq 0 ] || [ "$(now_epoch)" -ge "${deadline}" ]; then
            break
        fi
    fi
done

append_report
elapsed=$((($(now_epoch) - start) * 1000))
echo "soak ok cycles=${cycle} elapsedMs=${elapsed} backend=${BACKEND} protocol=${PROTOCOL} scenario=${SCENARIO}"
