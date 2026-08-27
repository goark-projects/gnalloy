#!/usr/bin/env sh
set -eu

GO_CMD="${GO:-go}"
ALLOW_SKIP="${ALLOW_SKIP:-0}"
RUN_IOURING="${RUN_IOURING:-0}"
RAW_BIND="${GNALLOY_RAW_BIND:-0.0.0.0}"
RAW_PROTOCOL="${GNALLOY_RAW_PROTOCOL:-1}"
L2_INTERFACE="${GNALLOY_L2_INTERFACE:-}"
L2_ETHERTYPE="${GNALLOY_L2_ETHERTYPE:-0}"
DOQ_ADDR="${GNALLOY_DOQ_ADDR:-}"
QUIC_INTEROP_ADDR="${GNALLOY_QUIC_INTEROP_ADDR:-}"

REPO="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "${REPO}"
export GOWORK=off

skip_or_fail() {
    name="$1"
    reason="$2"
    if [ "${ALLOW_SKIP}" = "1" ]; then
        echo "== ${name} skipped: ${reason}"
        return 0
    fi
    echo "FAILED ${name}: ${reason}" >&2
    exit 1
}

run_gate() {
    name="$1"
    shift
    echo "== ${name}"
    "$@"
}

if [ "$(uname -s)" != "Linux" ]; then
    skip_or_fail "privileged-runtime" "requires Linux"
    exit 0
fi

if [ "$(id -u)" != "0" ] && ! grep -q "CapEff:.*[1-9a-fA-F]" /proc/self/status 2>/dev/null; then
    skip_or_fail "privileged-runtime" "requires root or effective capabilities such as CAP_NET_RAW"
    exit 0
fi

GNALLOY_RAW_PRIVILEGED=1 \
GNALLOY_RAW_BIND="${RAW_BIND}" \
GNALLOY_RAW_PROTOCOL="${RAW_PROTOCOL}" \
run_gate "raw-socket-open" "${GO_CMD}" test -count=1 ./transport/raw -run TestPrivilegedRawSocketOpen

if [ -z "${L2_INTERFACE}" ]; then
    skip_or_fail "af-packet-open" "set GNALLOY_L2_INTERFACE"
else
    GNALLOY_L2_INTERFACE="${L2_INTERFACE}" \
    GNALLOY_L2_ETHERTYPE="${L2_ETHERTYPE}" \
    run_gate "af-packet-open" "${GO_CMD}" test -count=1 ./transport/l2 -run TestPrivilegedAFPacketOpen
fi

if [ "${RUN_IOURING}" = "1" ]; then
    run_gate "iouring-sqpoll" ./scripts/verify-iouring-sqpoll.sh
    run_gate "iouring-fixed" ./scripts/verify-iouring-fixed.sh
else
    echo "== iouring-runtime skipped: set RUN_IOURING=1"
fi

if [ -n "${DOQ_ADDR}" ]; then
    run_gate "external-doq" ./scripts/verify-protocol.sh
else
    echo "== external-doq skipped: set GNALLOY_DOQ_ADDR"
fi

if [ -n "${QUIC_INTEROP_ADDR}" ]; then
    run_gate "external-quic-interop" "${GO_CMD}" test -count=1 ./transport/quic/rfc9000 -run TestExternalInteropHandshake
else
    echo "== external-quic-interop skipped: set GNALLOY_QUIC_INTEROP_ADDR"
fi
