#!/usr/bin/env bash
# One-time host packages for UMKIM v0.2 hub install (requires root).
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root: sudo $0" >&2
  exit 1
fi

dnf install -y \
  ansible-core \
  git \
  python3-pip \
  python3-libselinux \
  httpd-tools \
  podman \
  skopeo \
  wireguard-tools \
  kernel-modules-extra \
  nftables

modprobe br_netfilter || true
echo br_netfilter > /etc/modules-load.d/fleet-br-netfilter.conf

echo "Host prerequisites installed."
