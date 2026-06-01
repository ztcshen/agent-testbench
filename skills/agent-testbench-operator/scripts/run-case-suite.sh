#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
ATB="$SCRIPT_DIR/atb.sh"

command -v jq >/dev/null 2>&1 || {
  echo "jq is required for summary output" >&2
  exit 127
}

STORE="${ATB_STORE:-local}"
STATUS="${ATB_CASE_STATUS:-active}"
TIMEOUT_SECONDS="${ATB_TIMEOUT_SECONDS:-30}"
RUNTIME_ROOT="${ATB_RUNTIME_ROOT:-$(pwd)/.runtime/operator-reports}"
RUN_ID="${ATB_REQUEST_ID:-case-suite-$(date -u +%Y%m%dT%H%M%SZ)}"
OUTPUT_DIR="${ATB_OUTPUT_DIR:-$RUNTIME_ROOT/$STORE-case-suite-$STATUS-$RUN_ID}"
JSON_REPORT="$OUTPUT_DIR/case-suite-report.json"

mkdir -p "$OUTPUT_DIR"

echo "STORE=$STORE"
echo "OUTPUT_DIR=$OUTPUT_DIR"

"$ATB" case suite report \
  --store "$STORE" \
  --status "$STATUS" \
  --timeout-seconds "$TIMEOUT_SECONDS" \
  --output-dir "$OUTPUT_DIR" \
  --json \
  "$@" > "$JSON_REPORT"

jq '{
  ok,
  profileId,
  counts,
  elapsedMs,
  reportUrl,
  jsonReportUrl,
  junitReportUrl,
  failuresByNode: ([
    .results[]? | select((.status // "") != "passed") | (.nodeId // "")
  ] | group_by(.) | map({nodeId: .[0], failed: length}) | sort_by(-.failed)),
  failuresByError: ([
    .results[]? | select((.status // "") != "passed") | (.error // "")
  ] | group_by(.) | map({error: .[0], failed: length}) | sort_by(-.failed))
}' "$JSON_REPORT"
