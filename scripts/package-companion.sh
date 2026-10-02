#!/bin/sh
set -eu
repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_dir"
for target in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64; do
  target_os=${target%-*}
  target_arch=${target#*-}
  output="$repo_dir/dist/companion-$target"
  mkdir -p "$output"
  (cd companion && CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -ldflags='-s -w' -o "$output/ddev-manager" .)
  cp scripts/install.sh scripts/uninstall.sh "$output/"
  cp README.md "$output/README.md"
  tar -czf "dist/ddev-manager-$target.tar.gz" -C "$output" .
done
