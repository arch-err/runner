#!/usr/bin/env bash
set -euo pipefail

mkdir -p dist/release
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  os=${target%/*}
  arch=${target#*/}
  output="dist/release/runner-${os}-${arch}"
  if [[ $os == windows ]]; then
    output+=.exe
  fi
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "$output" ./cmd/runner
done
(cd dist/release && sha256sum runner-* > SHA256SUMS)
