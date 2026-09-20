#!/usr/bin/env bash
set -euo pipefail

version="${1:?Version is required}"
commit="${2:?Commit is required}"
output_dir="${3:?Output directory is required}"
platform="${4:-}"
built_at=$(date -u +'%Y-%m-%dT%H:%M:%SZ')

write_build_info() {
  local platform="$1"
  local destination="$2"
  mkdir -p "$destination"
  printf 'Version: %s\nCommit: %s\nPlatform: %s\nBuilt at: %s\n' \
    "$version" "$commit" "$platform" "$built_at" > "$destination/BUILD.txt"
}

# Test builds request one platform and write directly into their package directory.
if [ -n "$platform" ]; then
  write_build_info "$platform" "$output_dir"
  exit 0
fi

# Include the universal macOS archive produced by GoReleaser.
platforms=(
  linux/amd64 linux/arm64
  windows/amd64 windows/arm64
  darwin/amd64 darwin/arm64 darwin/all
  freebsd/amd64 freebsd/arm64
)

for platform in "${platforms[@]}"; do
  destination="${output_dir}/${platform/\//_}"
  write_build_info "$platform" "$destination"
done
