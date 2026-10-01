#!/bin/sh
set -eu
version=${1:?Usage: build-release.sh VERSION [OUTPUT]}
out=${2:-dist}
case "$version" in v[0-9]*.[0-9]*.[0-9]*) ;; *) echo 'Expected a version such as v0.1.0' >&2; exit 1;; esac
commit=$(git rev-parse HEAD)
mkdir -p "$out"
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do
    os=${target%/*}; arch=${target#*/}
    name="cmdlib_${version}_${os}_${arch}"
    stage=$(mktemp -d)
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="-s -w -X main.version=$version -X main.commit=$commit" -o "$stage/cmdlib" .
    cp LICENSE THIRD_PARTY_NOTICES.md "$stage/"
    cp -R licenses "$stage/"
    tar -czf "$out/$name.tar.gz" -C "$stage" cmdlib LICENSE THIRD_PARTY_NOTICES.md licenses
    rm -rf "$stage"
done
(cd "$out"; if command -v sha256sum >/dev/null 2>&1; then sha256sum ./*.tar.gz; else shasum -a 256 ./*.tar.gz; fi) > "$out/checksums.txt"
