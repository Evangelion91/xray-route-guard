#!/bin/sh
set -eu
[ "$(id -u)" = 0 ] || { echo 'Run as root.' >&2; exit 1; }
if [ -x /etc/init.d/xray-route-guard ]; then
    /etc/init.d/xray-route-guard stop
    /etc/init.d/xray-route-guard disable
fi
rm -f /usr/sbin/xray-route-guard /etc/init.d/xray-route-guard
echo 'Removed executable and service. Configuration, recovery records and your active Xray configuration are retained.'
