#!/usr/bin/env sh
set -eu

if [ "$(uname -s)" != "Linux" ]; then
    echo "io_uring SQPOLL verification requires Linux" >&2
    exit 1
fi

BACKEND=iouring \
MMAP="${MMAP:-1}" \
IOURING_SQPOLL=1 \
IOURING_MULTISHOT_ACCEPT="${IOURING_MULTISHOT_ACCEPT:-1}" \
IOURING_ENTRIES="${IOURING_ENTRIES:-256}" \
WORKERS="${WORKERS:-4}" \
CONNECTIONS="${CONNECTIONS:-64}" \
MESSAGES="${MESSAGES:-64}" \
PAYLOAD_SIZE="${PAYLOAD_SIZE:-64}" \
SCENARIO="${SCENARIO:-mixed}" \
"$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/verify-stress.sh"
