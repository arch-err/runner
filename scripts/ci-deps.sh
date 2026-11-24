#!/usr/bin/env bash
set -euo pipefail

root=$(pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cp go.mod go.sum "$tmp/"
cp -r cmd internal "$tmp/"
(cd "$tmp" && go mod vendor)
mkdir -p dist/ci-deps
tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner -C "$tmp" -czf dist/ci-deps/vendor.tar.gz vendor
sha256sum dist/ci-deps/vendor.tar.gz | cut -d ' ' -f1 > .forgejo/vendor.sha256
version=$(sha256sum go.sum | cut -d ' ' -f1)
printf 'Upload %s/dist/ci-deps/vendor.tar.gz to:\nhttps://git.3rr.dev/api/packages/arch-err/generic/runner-deps/%s/vendor.tar.gz\n' "$root" "$version"
