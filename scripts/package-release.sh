#!/usr/bin/env bash

set -Eeuo pipefail
export COPYFILE_DISABLE=1

TEMP_DIR=""

usage() {
  cat <<'EOF'
Usage: scripts/package-release.sh VERSION ARCH [OUTPUT_DIR]

Build a self-contained Linux RepoForge release archive.

Arguments:
  VERSION      Release version, for example v0.1.0
  ARCH         Go architecture: amd64 or arm64
  OUTPUT_DIR   Archive output directory, defaults to dist
EOF
}

log_info() {
  printf '[INFO] %s\n' "$*"
}

die() {
  printf '[ERROR] %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "Required command not found: $1"
}

cleanup() {
  if [[ -n "$TEMP_DIR" ]]; then
    rm -rf -- "$TEMP_DIR"
  fi
}

main() {
  [[ $# -ge 2 && $# -le 3 ]] || {
    usage
    exit 2
  }

  local version="$1"
  local arch="$2"
  local output_dir="${3:-dist}"
  local commit
  local build_date
  local archive

  [[ "$arch" == "amd64" || "$arch" == "arm64" ]] || die "Unsupported architecture: $arch"
  require_command go
  require_command git
  require_command tar

  commit="$(git rev-parse --short HEAD)"
  build_date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  TEMP_DIR="$(mktemp -d)"

  mkdir -p "$TEMP_DIR/repoforge/bin" "$output_dir"
  log_info "Building RepoForge $version for linux/$arch"
  GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -X github.com/fanhuadesenlinnn/RepoForge/internal/version.Version=$version -X github.com/fanhuadesenlinnn/RepoForge/internal/version.Commit=$commit -X github.com/fanhuadesenlinnn/RepoForge/internal/version.Date=$build_date" \
    -o "$TEMP_DIR/repoforge/bin/repoforge" .

  cp README.md CHANGELOG.md repoforge_codex_execution_plan.md "$TEMP_DIR/repoforge/"
  archive="$output_dir/repoforge_${version}_linux_${arch}.tar.gz"
  tar -C "$TEMP_DIR" -czf "$archive" repoforge
  log_info "Created $archive"
}

trap cleanup EXIT
main "$@"
