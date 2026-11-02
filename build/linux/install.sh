#!/bin/sh
set -eu

# OpenRGB API installer for Linux (systemd-based distributions)
if [ "$(id -u)" -ne 0 ]; then
    echo "Run this installer as root (for example: sudo sh install.sh)." >&2
    exit 1
fi

# Determine installation mode (local or LAN) based on command-line arguments.
mode=local
if [ "${1:-}" = "--lan" ]; then
    mode=lan
    shift
fi
if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    echo "Usage: install.sh [--lan]"
    exit 0
fi
if [ "$#" -ne 0 ]; then
    echo "Usage: install.sh [--lan]" >&2
    exit 2
fi

# Determine the directory containing this script and the source files for installation.
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
binary_source="$script_dir/openrgb-api"
unit_source="$script_dir/openrgb-api.service"
config_source="$script_dir/config-$mode.toml"
config_dir=/etc/openrgb-api
config_file="$config_dir/config.toml"

for required_file in "$binary_source" "$unit_source" "$config_source"; do
    if [ ! -f "$required_file" ]; then
        echo "Required installer file not found: $required_file" >&2
        exit 1
    fi
done
# Ensure systemd is available.
if ! command -v systemctl >/dev/null 2>&1; then
    echo "systemctl was not found; this installer requires a systemd-based distribution." >&2
    exit 1
fi

# Create the openrgb-api system user if it does not exist.
if ! getent passwd openrgb-api >/dev/null 2>&1; then
    if command -v useradd >/dev/null 2>&1; then
        nologin_shell=$(command -v nologin || echo /usr/sbin/nologin)
        useradd --system --no-create-home --shell "$nologin_shell" openrgb-api
    else
        echo "useradd is required to create the openrgb-api service account." >&2
        exit 1
    fi
fi

# Stop the service if it is currently running.
if systemctl is-active --quiet openrgb-api.service; then
    systemctl stop openrgb-api.service
fi
# Install the binary, configuration, and systemd unit files.
install -D -o root -g root -m 0755 "$binary_source" /usr/local/bin/openrgb-api
install -d -o openrgb-api -g openrgb-api -m 0750 "$config_dir"
if [ ! -e "$config_file" ]; then
    install -o openrgb-api -g openrgb-api -m 0600 "$config_source" "$config_file"
fi
# Set the correct ownership and permissions for the configuration file.
chown openrgb-api:openrgb-api "$config_file"
chmod 0600 "$config_file"
install -D -o root -g root -m 0644 "$unit_source" /etc/systemd/system/openrgb-api.service

# Reload systemd, enable and start the service.
systemctl daemon-reload
systemctl enable --now openrgb-api.service
echo "OpenRGB API installed. Check it with: systemctl status openrgb-api"
