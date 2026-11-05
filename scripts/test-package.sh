#!/bin/sh
set -eu
package=${1:?Usage: scripts/test-package.sh EXTRACTED_AMD64_PACKAGE}
root=$(mktemp -d)
trap 'rm -rf "$root"' EXIT
DESTDIR="$root" sh "$package/install.sh"
test ! -e "$root/etc/xray-route-guard/config.json"
test ! -e "$root/etc/rc.d"
test "$(stat -c %a "$root/etc/xray-route-guard")" = 700
test "$(stat -c %a "$root/etc/xray-route-guard/config.example.json")" = 600
"$root/usr/sbin/xray-route-guard" version
printf '%s\n' 'Staged install passed; no active configuration or autostart created.'
