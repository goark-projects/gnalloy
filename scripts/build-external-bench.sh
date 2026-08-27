#!/usr/bin/env sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
EXTERNAL="$ROOT/benchmarks/external"
BIN="$EXTERNAL/bin"
GO_BIN="${GO:-go}"
MAVEN_BIN="${MAVEN:-mvn}"

mkdir -p "$BIN"

# 外部 harness 必须隔离 go.work，避免把对标依赖注入 gnalloy 根模块。
export GOWORK=off
export GOTOOLCHAIN=local

(cd "$EXTERNAL/gnalloy-bench" && "$GO_BIN" mod download && "$GO_BIN" build -trimpath -o "$BIN/gnalloy-bench" .)
(cd "$EXTERNAL/gnet-bench" && "$GO_BIN" mod download && "$GO_BIN" build -trimpath -o "$BIN/gnet-bench" .)
(cd "$EXTERNAL/netpoll-bench" && "$GO_BIN" mod download && "$GO_BIN" build -trimpath -o "$BIN/netpoll-bench" .)

if [ "${SKIP_NETTY:-0}" != "1" ]; then
    "$MAVEN_BIN" -q -f "$EXTERNAL/netty-bench/pom.xml" -DskipTests package
    cp "$EXTERNAL/netty-bench/target/netty-bench.jar" "$BIN/netty-bench.jar"
fi

printf 'external benchmark harnesses built under %s\n' "$BIN"
