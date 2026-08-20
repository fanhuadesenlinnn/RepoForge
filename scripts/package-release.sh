#!/usr/bin/env bash

set -Eeuo pipefail
export COPYFILE_DISABLE=1

TEMP_DIR=""

usage() {
  cat <<'EOF'
Usage: scripts/package-release.sh VERSION OS ARCH [OUTPUT_DIR]

Build a self-contained RepoForge release archive.

Arguments:
  VERSION      Release version, for example v0.1.0
  OS           Target OS: linux, darwin, or windows
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
  [[ $# -ge 3 && $# -le 4 ]] || {
    usage
    exit 2
  }

  local version="$1"
  local goos="$2"
  local arch="$3"
  local output_dir="${4:-dist}"
  local commit
  local build_date
  local archive
  local binary="repoforge"
  local ext="tar.gz"

  [[ "$goos" == "linux" || "$goos" == "darwin" || "$goos" == "windows" ]] ||
    die "Unsupported OS: $goos (expected linux, darwin, or windows)"
  [[ "$arch" == "amd64" || "$arch" == "arm64" ]] || die "Unsupported architecture: $arch"
  require_command go
  require_command git
  require_command tar

  if [[ "$goos" == "windows" ]]; then
    binary="repoforge.exe"
    ext="zip"
  fi

  commit="$(git rev-parse --short HEAD)"
  build_date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  TEMP_DIR="$(mktemp -d)"

  mkdir -p "$TEMP_DIR/repoforge/bin" "$output_dir"
  log_info "Building RepoForge $version for $goos/$arch"
  GOOS="$goos" GOARCH="$arch" CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -X github.com/fanhuadesenlinnn/RepoForge/internal/version.Version=$version -X github.com/fanhuadesenlinnn/RepoForge/internal/version.Commit=$commit -X github.com/fanhuadesenlinnn/RepoForge/internal/version.Date=$build_date" \
    -o "$TEMP_DIR/repoforge/bin/$binary" .

  cp README.md CHANGELOG.md "$TEMP_DIR/repoforge/"
  archive="$(cd "$output_dir" && pwd)/repoforge_${version}_${goos}_${arch}.${ext}"
  if [[ "$goos" == "windows" ]]; then
    if command -v zip >/dev/null 2>&1; then
      (cd "$TEMP_DIR" && zip -qr "$archive" repoforge)
    elif command -v python3 >/dev/null 2>&1; then
      (cd "$TEMP_DIR" && python3 -c "import shutil; shutil.make_archive('repoforge', 'zip', '.', 'repoforge')" && mv repoforge.zip "$archive")
    else
      die "Neither 'zip' nor 'python3' found; cannot create Windows archive"
    fi
  else
    tar -C "$TEMP_DIR" -czf "$archive" repoforge
  fi
  log_info "Created $archive"
}

trap cleanup EXIT
main "$@"
