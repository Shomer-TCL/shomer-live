#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
docker run --rm --privileged --volume "$PWD:/src" --workdir /src \
  debian:trixie-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2 \
  bash -euc '
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      live-build debootstrap debian-archive-keyring squashfs-tools xorriso \
      isolinux syslinux-common syslinux-utils dosfstools mtools grub-pc-bin \
      grub-efi-amd64-bin grub-efi-amd64-signed shim-signed ca-certificates \
      apt-utils cpio file rsync
    bash scripts/configure-live.sh
    cd build/live
    lb build
  '
test -s build/live/live-image-amd64.hybrid.iso
