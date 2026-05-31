#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT_DIR"

DENYLIST="tools/guardrails/source-domain-terms.txt"

if [[ -e team-configs ]]; then
  echo "core repo must not contain bundled team configuration; keep team data outside this repository" >&2
  exit 1
fi

if [[ ! -f "$DENYLIST" ]]; then
  echo "missing source-domain denylist: $DENYLIST" >&2
  exit 1
fi

existing=()
scan_args=("$@")

collect_git_files() {
  local source=$1
  shift
  while IFS= read -r -d '' path; do
    case "$path" in
      .git/*|.idea/*|.runtime/*|node_modules/*)
        continue
        ;;
      .scratch/*|.understand-anything/*)
        if [[ "$source" == "untracked" ]]; then
          continue
        fi
        ;;
      docs/progress/*|docs/plans/*)
        continue
        ;;
      package-lock.json|tools/guardrails/source-domain-terms.txt)
        continue
        ;;
    esac
    if [[ -f "$path" ]]; then
      existing+=("$path")
    fi
  done < <("$@")
}

if [[ ${#scan_args[@]} -gt 0 ]]; then
  collect_git_files cached git ls-files --cached -z -- "${scan_args[@]}"
  collect_git_files untracked git ls-files --others --exclude-standard -z -- "${scan_args[@]}"
else
  collect_git_files cached git ls-files --cached -z
  collect_git_files untracked git ls-files --others --exclude-standard -z
fi

if [[ ${#existing[@]} -eq 0 ]]; then
  echo "no core paths to scan"
  exit 0
fi

if rg -n -i -f "$DENYLIST" "${existing[@]}"; then
  echo "core contains source-domain terms; move them into private validation data" >&2
  exit 1
fi

echo "core scan passed"
