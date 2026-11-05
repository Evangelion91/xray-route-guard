#!/bin/sh
set -eu
cd "$(dirname "$0")"
prefix=${DESTDIR:-}
if [ -z "$prefix" ]; then
    [ "$(id -u)" = 0 ] || { echo 'Run as root.' >&2; exit 1; }
    [ -f /etc/openwrt_release ] || { echo 'This installer supports OpenWrt. See docs for other Linux hosts.' >&2; exit 1; }
    if [ -x /etc/init.d/xray-route-guard ] && /etc/init.d/xray-route-guard running >/dev/null 2>&1; then
        echo 'Stop xray-route-guard before upgrading so any pending rollback can finish.' >&2
        exit 1
    fi
fi
[ -f xray-route-guard ] && [ -f openwrt.init ] && [ -f config.example.json ] || { echo 'Extract the complete release archive first.' >&2; exit 1; }
umask 077
mkdir -p "$prefix/usr/sbin" "$prefix/etc/init.d" "$prefix/etc/xray-route-guard"
chmod 700 "$prefix/etc/xray-route-guard"
cp xray-route-guard "$prefix/usr/sbin/xray-route-guard.new"
chmod 755 "$prefix/usr/sbin/xray-route-guard.new"
mv "$prefix/usr/sbin/xray-route-guard.new" "$prefix/usr/sbin/xray-route-guard"
cp openwrt.init "$prefix/etc/init.d/xray-route-guard"
chmod 755 "$prefix/etc/init.d/xray-route-guard"
cp config.example.json "$prefix/etc/xray-route-guard/config.example.json"
chmod 600 "$prefix/etc/xray-route-guard/config.example.json"
echo 'Installed, disabled. Configure the example, run check, then explicitly start/enable the service.'
