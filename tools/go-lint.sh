#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage:
  tools/go-lint.sh [--base REF] [--head REF] [--patch PATH] [--full] [--] [golangci-lint args...]

Default mode mirrors pull-request CI: lint only issues introduced by the
current branch patch against the base branch. Use --full for a full repository
lint sweep.
USAGE
}

root_dir=$(git rev-parse --show-toplevel 2>/dev/null)
cd "$root_dir"

lint_bin=${GOLANGCI_LINT_BIN:-golangci-lint}
mode=pr
base_ref=${AGENT_TESTBENCH_LINT_BASE_REF:-}
head_ref=${AGENT_TESTBENCH_LINT_HEAD_REF:-HEAD}
patch_file=""
extra_args=()

while (($# > 0)); do
  case "$1" in
    --base)
      if (($# < 2)); then
        echo "--base requires a ref" >&2
        exit 2
      fi
      base_ref=$2
      shift 2
      ;;
    --head)
      if (($# < 2)); then
        echo "--head requires a ref" >&2
        exit 2
      fi
      head_ref=$2
      shift 2
      ;;
    --patch)
      if (($# < 2)); then
        echo "--patch requires a file path" >&2
        exit 2
      fi
      patch_file=$2
      shift 2
      ;;
    --full)
      mode=full
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    --)
      shift
      extra_args+=("$@")
      break
      ;;
    *)
      extra_args+=("$1")
      shift
      ;;
  esac
done

if ! command -v "$lint_bin" >/dev/null 2>&1; then
  go_lint_bin="$(go env GOPATH 2>/dev/null)/bin/golangci-lint"
  if [[ -x "$go_lint_bin" ]]; then
    lint_bin=$go_lint_bin
  else
    echo "$lint_bin is required for Go lint. Install golangci-lint or set GOLANGCI_LINT_BIN." >&2
    exit 127
  fi
fi

if [[ "$mode" == "full" ]]; then
  if ((${#extra_args[@]} > 0)); then
    exec "$lint_bin" run "${extra_args[@]}"
  fi
  exec "$lint_bin" run
fi

if [[ -z "$patch_file" ]]; then
  if [[ -z "$base_ref" && -n "${GITHUB_BASE_REF:-}" ]]; then
    for candidate in "origin/${GITHUB_BASE_REF}" "github/${GITHUB_BASE_REF}" "$GITHUB_BASE_REF"; do
      if git rev-parse --verify --quiet "$candidate^{commit}" >/dev/null; then
        base_ref=$candidate
        break
      fi
    done
  fi
  if [[ -z "$base_ref" ]]; then
    for candidate in github/main origin/main main master; do
      if git rev-parse --verify --quiet "$candidate^{commit}" >/dev/null; then
        base_ref=$candidate
        break
      fi
    done
  fi
  if [[ -z "$base_ref" ]]; then
    echo "could not resolve lint base ref; fetch main or pass --base REF" >&2
    exit 2
  fi

  merge_base=$(git merge-base "$base_ref" "$head_ref")
  patch_file=$(mktemp "${TMPDIR:-/tmp}/agent-testbench-go-lint.XXXXXX")
  trap 'rm -f "$patch_file"' EXIT
  git diff --diff-filter=ACMRT --binary "$merge_base"..."$head_ref" >"$patch_file"
  echo "Go lint base: $base_ref ($merge_base)" >&2
fi

if [[ ! -s "$patch_file" ]]; then
  echo "No lint patch changes detected." >&2
  exit 0
fi

if ((${#extra_args[@]} > 0)); then
  exec "$lint_bin" run --new-from-patch "$patch_file" "${extra_args[@]}"
fi
exec "$lint_bin" run --new-from-patch "$patch_file"
