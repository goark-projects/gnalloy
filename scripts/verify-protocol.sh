#!/usr/bin/env sh
set -eu

GO_CMD="${GO:-go}"
SKIP_EXTERNAL="${SKIP_EXTERNAL:-0}"
SKIP_BENCH="${SKIP_BENCH:-0}"
DOQ_ADDR="${GNALLOY_DOQ_ADDR:-}"
DOQ_SERVER_NAME="${GNALLOY_DOQ_SERVER_NAME:-}"
DOQ_QUERY="${GNALLOY_DOQ_QUERY:-example.com}"
DOQ_TYPE="${GNALLOY_DOQ_TYPE:-A}"
DOQ_TIMEOUT="${GNALLOY_DOQ_TIMEOUT:-5s}"
DOQ_INSECURE="${GNALLOY_DOQ_INSECURE:-0}"

REPO="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "${REPO}"
export GOWORK=off

run() {
    name="$1"
    shift
    echo "== ${name}"
    "$@"
}

run "protocol-tests" "${GO_CMD}" test -count=1 \
    ./protocol \
    ./transport/quic/application \
    ./resolver/dns/quic \
    ./transport/l2 \
    ./transport/l2/bpf \
    ./transport/l2/npcap \
    ./examples/doq-query \
    ./examples/protocol-exchange

run "doq dry-run" "${GO_CMD}" run ./examples/doq-query -dry-run -server dns.google:853 -name example.com -type A
run "protocol tcp dry-run" "${GO_CMD}" run ./examples/protocol-exchange -dry-run -transport tcp -addr 127.0.0.1:9000 -message ping
run "protocol udp dry-run" "${GO_CMD}" run ./examples/protocol-exchange -dry-run -transport udp -addr 127.0.0.1:9002 -message ping
run "protocol raw dry-run" "${GO_CMD}" run ./examples/protocol-exchange -dry-run -transport raw -addr 127.0.0.1 -raw-protocol 253 -message ping
run "protocol l2 dry-run" "${GO_CMD}" run ./examples/protocol-exchange -dry-run -transport l2 -addr eth0 -payload-hex 00112233445566778899aabb88b570696e67

if [ "${SKIP_BENCH}" = "1" ]; then
    echo "== parity-bench-dry-run skipped: SKIP_BENCH=1"
else
    run "parity-bench-dry-run" "${GO_CMD}" run ./examples/parity-bench -dry-run -config benchmarks/parity/baseline.json -format json
fi

if [ "${SKIP_EXTERNAL}" = "1" ]; then
    echo "== external-doq skipped: SKIP_EXTERNAL=1"
elif [ -z "${DOQ_ADDR}" ]; then
    echo "== external-doq skipped: set GNALLOY_DOQ_ADDR to run an external DoQ query"
else
    if [ -n "${DOQ_SERVER_NAME}" ]; then
        if [ "${DOQ_INSECURE}" = "1" ]; then
            run "external-doq" "${GO_CMD}" run ./examples/doq-query -server "${DOQ_ADDR}" -server-name "${DOQ_SERVER_NAME}" -name "${DOQ_QUERY}" -type "${DOQ_TYPE}" -timeout "${DOQ_TIMEOUT}" -insecure
        else
            run "external-doq" "${GO_CMD}" run ./examples/doq-query -server "${DOQ_ADDR}" -server-name "${DOQ_SERVER_NAME}" -name "${DOQ_QUERY}" -type "${DOQ_TYPE}" -timeout "${DOQ_TIMEOUT}"
        fi
    elif [ "${DOQ_INSECURE}" = "1" ]; then
        run "external-doq" "${GO_CMD}" run ./examples/doq-query -server "${DOQ_ADDR}" -name "${DOQ_QUERY}" -type "${DOQ_TYPE}" -timeout "${DOQ_TIMEOUT}" -insecure
    else
        run "external-doq" "${GO_CMD}" run ./examples/doq-query -server "${DOQ_ADDR}" -name "${DOQ_QUERY}" -type "${DOQ_TYPE}" -timeout "${DOQ_TIMEOUT}"
    fi
fi
