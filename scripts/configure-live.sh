#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p build/live
cd build/live
lb config --mode debian --distribution trixie --architectures amd64 \
  --archive-areas 'main contrib non-free non-free-firmware' \
  --binary-images iso-hybrid --bootloaders 'syslinux grub-efi' \
  --debian-installer false --apt-recommends false \
  --iso-application 'Shomer TCL Flip 2 Offline Workbench' \
  --iso-volume SHOMER_LIVE --memtest none \
  --bootappend-live 'boot=live components username=shomer hostname=shomer-live locales=en_US.UTF-8 keyboard-layouts=us persistence persistence-label=SHOMER_PERSISTENCE'
cp ../../config/packages.list config/package-lists/shomer.list.chroot
mkdir -p config/includes.chroot/opt/shomer config/hooks/live
cp -a ../root/. config/includes.chroot/
cat > config/hooks/live/0500-shomer.hook.chroot <<'HOOK'
#!/bin/sh
set -eu
printf 'shomer-live\n' > /etc/hostname
mkdir -p /etc/lightdm/lightdm.conf.d /etc/skel/Desktop /etc/xdg/autostart
printf '[Seat:*]\nautologin-user=shomer\nautologin-session=xfce\n' > /etc/lightdm/lightdm.conf.d/50-shomer.conf
# No host disks are automatically mounted. Explicit file-manager mounts remain possible.
mkdir -p /etc/skel/.config/xfce4/xfconf/xfce-perchannel-xml
cat > /etc/skel/.config/xfce4/xfconf/xfce-perchannel-xml/thunar-volman.xml <<'XML'
<?xml version="1.0" encoding="UTF-8"?>
<channel name="thunar-volman" version="1.0"><property name="automount-drives" type="bool" value="false"/><property name="automount-media" type="bool" value="false"/><property name="autobrowse" type="bool" value="false"/><property name="autorun" type="bool" value="false"/></channel>
XML
cp /usr/share/applications/shomer-workbench.desktop /etc/skel/Desktop/
chmod +x /etc/skel/Desktop/shomer-workbench.desktop
ln -sf /opt/shomer/TCL-Flip2 /etc/skel/TCL-Flip2
mkdir -p /etc/systemd/system
for service in ModemManager.service apt-daily.service apt-daily-upgrade.service NetworkManager.service systemd-networkd.service; do ln -sf /dev/null /etc/systemd/system/$service; done
mkdir -p /etc/modules-load.d
printf 'cdc_acm\nusbserial\nftdi_sio\ncp210x\nch341\n' > /etc/modules-load.d/shomer.conf
mkdir -p /etc/live/config.conf.d
printf 'LIVE_USER_DEFAULT_GROUPS="audio cdrom video plugdev dialout sudo"\n' > /etc/live/config.conf.d/20-shomer.conf
dpkg-query -W -f='${Package}\t${Version}\n' > /opt/shomer/TCL-Flip2/Checksums-and-manifests/debian-packages.tsv
HOOK
chmod +x config/hooks/live/0500-shomer.hook.chroot
