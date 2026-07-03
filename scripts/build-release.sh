#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR"

output_dir="dist"
version=""
revision=""
targets=()

usage() {
  cat <<'USAGE'
Usage: scripts/build-release.sh [--version VERSION] [--revision REVISION] [--output-dir DIR] [--target GOOS/GOARCH]

Build compressed AgentTestBench CLI release assets. When no --target is given,
the current GOOS/GOARCH pair is used.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      version="${2:-}"
      shift 2
      ;;
    --revision)
      revision="${2:-}"
      shift 2
      ;;
    --output-dir)
      output_dir="${2:-}"
      shift 2
      ;;
    --target)
      targets+=("${2:-}")
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "unknown build-release option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -z "$version" ]]; then
  if exact_tag=$(git describe --tags --exact-match 2>/dev/null); then
    version="${exact_tag#v}"
  else
    version=$(node -e "process.stdout.write(JSON.parse(require('fs').readFileSync('package.json', 'utf8')).version)")
  fi
fi

if [[ -z "$revision" ]]; then
  revision=$(git rev-parse HEAD)
fi

if [[ ${#targets[@]} -eq 0 ]]; then
  targets+=("$(go env GOOS)/$(go env GOARCH)")
fi

if [[ -z "$output_dir" || "$output_dir" == "/" || "$output_dir" == "." ]]; then
  echo "--output-dir must point at a dedicated artifact directory, got: ${output_dir:-<empty>}" >&2
  exit 1
fi

rm -rf "$output_dir"
mkdir -p "$output_dir"

for target in "${targets[@]}"; do
  if [[ ! "$target" =~ ^[^/]+/[^/]+$ ]]; then
    echo "--target must use GOOS/GOARCH, got: $target" >&2
    exit 1
  fi
  goos=${target%/*}
  goarch=${target#*/}
  work_dir="$output_dir/agent-testbench_${version}_${goos}_${goarch}"
  binary_name="agent-testbench"
  if [[ "$goos" == "windows" ]]; then
    binary_name="agent-testbench.exe"
  fi

  mkdir -p "$work_dir"
  env GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -X main.version=$version -X main.buildRevision=$revision" \
    -o "$work_dir/$binary_name" \
    ./cmd/agent-testbench
  cp LICENSE README.md "$work_dir/"
  if [[ -f NOTICE ]]; then
    cp NOTICE "$work_dir/"
  fi

  archive="$output_dir/agent-testbench_${version}_${goos}_${goarch}.tar.gz"
  tar -C "$output_dir" -czf "$archive" "$(basename "$work_dir")"
  rm -rf "$work_dir"
  echo "$archive"
done
