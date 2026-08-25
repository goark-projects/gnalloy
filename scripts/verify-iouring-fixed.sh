#!/usr/bin/env sh
set -eu

if [ "$(uname -s)" != "Linux" ]; then
    echo "io_uring fixed-buffer verification requires Linux" >&2
    exit 1
fi

BACKEND=iouring \
MMAP=1 \
MMAP_BLOCK_SIZE="${MMAP_BLOCK_SIZE:-4096}" \
MMAP_BLOCKS="${MMAP_BLOCKS:-512}" \
IOURING_FIXED_BUFFERS=1 \
IOURING_MULTISHOT_ACCEPT="${IOURING_MULTISHOT_ACCEPT:-1}" \
IOURING_ENTRIES="${IOURING_ENTRIES:-256}" \
WORKERS="${WORKERS:-2}" \
CONNECTIONS="${CONNECTIONS:-32}" \
MESSAGES="${MESSAGES:-32}" \
PAYLOAD_SIZE="${PAYLOAD_SIZE:-64}" \
SCENARIO="${SCENARIO:-mixed}" \
"$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/verify-stress.sh"
