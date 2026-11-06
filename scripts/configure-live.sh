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
  --bootappend-live 'boot=live components console=tty0 console=ttyS0,115200 username=shomer hostname=shomer-live locales=en_US.UTF-8 keyboard-layouts=us persistence persistence-label=SHOMER_PERSISTENCE'
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
cat > /etc/skel/.config/xfce4/xfconf/xfce-perchannel-xml/thunar.xml <<'XML'
<?xml version="1.0" encoding="UTF-8"?>
<channel name="thunar" version="1.0"><property name="misc-volume-management" type="bool" value="false"/></channel>
XML
cp /usr/share/applications/shomer-workbench.desktop /etc/skel/Desktop/
chmod +x /etc/skel/Desktop/shomer-workbench.desktop
# Debian's LXPolKit autostart excludes Xfce, whose full desktop normally supplies
# another agent. This minimal desktop deliberately uses the lightweight LX agent.
sed -i '/^NotShowIn=/d' /etc/xdg/autostart/lxpolkit.desktop
ln -sf /opt/shomer/TCL-Flip2 /etc/skel/TCL-Flip2
mkdir -p /etc/systemd/system
for service in ModemManager.service apt-daily.service apt-daily-upgrade.service NetworkManager.service systemd-networkd.service; do ln -sf /dev/null /etc/systemd/system/$service; done
mkdir -p /etc/modules-load.d
printf 'cdc_acm\nusbserial\nftdi_sio\ncp210x\nch341\nqemu_fw_cfg\n' > /etc/modules-load.d/shomer.conf
mkdir -p /etc/live/config.conf.d
printf 'LIVE_USER_DEFAULT_GROUPS="audio cdrom video plugdev dialout sudo"\n' > /etc/live/config.conf.d/20-shomer.conf
# Fully install the hash-locked Python environment OFFLINE inside the final root,
# so venv shebangs point at their actual runtime location rather than a build path.
kit=/opt/shomer/TCL-Flip2
python3 -m venv "$kit/Tools/MTKClient/venv"
"$kit/Tools/MTKClient/venv/bin/pip" install --no-index --require-hashes --no-compile --find-links="$kit/Tools/MTKClient/wheels" -r "$kit/Tools/MTKClient/requirements.txt"
"$kit/Tools/MTKClient/venv/bin/pip" check
"$kit/Tools/MTKClient/venv/bin/pip" freeze --all > "$kit/Checksums-and-manifests/python-packages.txt"
# Inputs originate in private checkouts with restrictive modes. Make only the
# public-to-this-live-system toolkit readable; no private keys are build inputs.
chmod -R a+rX "$kit"
dpkg-query -W -f='${Package}\t${Version}\n' > /opt/shomer/TCL-Flip2/Checksums-and-manifests/debian-packages.tsv
cd "$kit"
find . -type f ! -path './Checksums-and-manifests/files.sha256' -print0 | sort -z | xargs -0 sha256sum > Checksums-and-manifests/files.sha256
HOOK
chmod +x config/hooks/live/0500-shomer.hook.chroot
