#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
SKILL_DIR="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
REPO_DIR="${ATB_REPO_DIR:-$(CDPATH= cd -- "$SKILL_DIR/../.." && pwd)}"

export ATB_STORE="${ATB_STORE:-local}"

if [[ -n "${ATB_BIN:-}" ]]; then
  exec "$ATB_BIN" "$@"
fi

REPO_RUNTIME="$REPO_DIR/.runtime/bin/agent-testbench"
if [[ -x "$REPO_RUNTIME" ]]; then
  exec "$REPO_RUNTIME" "$@"
fi

PATH_BIN="$(command -v agent-testbench || true)"
if [[ -n "$PATH_BIN" && -x "$PATH_BIN" ]]; then
  exec "$PATH_BIN" "$@"
fi

REPO_WRAPPER="$REPO_DIR/bin/agent-testbench.sh"
if [[ -x "$REPO_WRAPPER" ]]; then
  exec "$REPO_WRAPPER" "$@"
fi

echo "agent-testbench binary not found. Run: $REPO_DIR/bin/agent-testbench.sh onboard --repo $REPO_DIR --store local --build-runtime --install-shell" >&2
exit 127
