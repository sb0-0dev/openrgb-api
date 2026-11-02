#!/bin/sh
set -eu

# OpenRGB API uninstaller for Linux (systemd-based distributions)
if [ "$(id -u)" -ne 0 ]; then
    echo "Run this uninstaller as root (for example: sudo sh uninstall.sh)." >&2
    exit 1
fi

purge=false
if [ "${1:-}" = "--purge" ]; then
    purge=true
    shift
fi
if [ "$#" -ne 0 ]; then
    echo "Usage: uninstall.sh [--purge]" >&2
    exit 2
fi

# Stop and disable the service if systemd is available.
if command -v systemctl >/dev/null 2>&1; then
    systemctl disable --now openrgb-api.service 2>/dev/null || true
fi
# Remove the systemd unit and binary.
rm -f /etc/systemd/system/openrgb-api.service /usr/local/bin/openrgb-api
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
fi

# Remove configuration and service account if --purge is specified.
if [ "$purge" = true ]; then
    rm -rf /etc/openrgb-api
    if command -v userdel >/dev/null 2>&1 && getent passwd openrgb-api >/dev/null 2>&1; then
        userdel openrgb-api
    fi
    echo "OpenRGB API removed, including configuration and paired devices."
else
    echo "OpenRGB API removed. Configuration in /etc/openrgb-api was kept; use --purge to remove it."
fi
