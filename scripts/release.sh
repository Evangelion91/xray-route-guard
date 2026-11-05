#!/bin/sh
set -eu
version=${1:?Usage: scripts/release.sh VERSION}
case "$version" in *[!a-zA-Z0-9._-]*) echo 'Invalid version' >&2; exit 1;; esac
mkdir -p dist
for arch in amd64 arm64 arm; do
    case "$arch" in arm) label=armv7;; *) label=$arch;; esac
    stage="dist/xray-route-guard-$version-linux-$label"
    mkdir -p "$stage"
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" GOARM=7 go build -trimpath -buildvcs=false -ldflags "-s -w -X main.version=$version" -o "$stage/xray-route-guard" ./cmd/xray-route-guard
    cp packaging/install.sh packaging/uninstall.sh packaging/openwrt.init config.example.json LICENSE README.md "$stage/"
    cp -R docs "$stage/"
    chmod 755 "$stage/xray-route-guard" "$stage/install.sh" "$stage/uninstall.sh" "$stage/openwrt.init"
    tar -C "$stage" -czf "$stage.tar.gz" .
done
(cd dist && sha256sum ./*.tar.gz > SHA256SUMS)
