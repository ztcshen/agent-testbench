#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

if [ -n "${ATB_BIN:-}" ]; then
  exec "$ATB_BIN" "$@"
fi

RUNTIME_BIN="$ROOT_DIR/.runtime/bin/agent-testbench"
case "${1:-}" in
  status|doctor|version|--version|-v)
    if [ -x "$RUNTIME_BIN" ]; then
      exec "$RUNTIME_BIN" "$@"
    fi
    ;;
esac

exec go run "$ROOT_DIR/cmd/agent-testbench" "$@"
