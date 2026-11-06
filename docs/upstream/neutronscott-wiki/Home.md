TCL Flip 2 (T408DL, 4058G, 4058L, etc)

# Overview

The vendor framework on the phone prohibits APK installs unless a read-only property (`ro.vendor.tct.endurance`) is set. One way to overcome this is to modify only the boot partition, and have it reset the property. If doing a full ROM install, this could be unnecessary. Thus the minimal way is having Magisk installed on the boot partition and have it set the property to true during bootup.

# Root / Enable APK installation

**Credits**
- Finding APK unlock: Apps4Flip-Admin ([forum post](https://forums.apps4flip.com/d/596-alcatel-tcl-flip-2-t408dl/84))
- boot.img with Magisk: Biden2020prez & EnriqueMcquade
- Fastboot method & using Magisk's overlay.d: darth
- First to put it all together: neutronscott (that's me!)

## Prerequisites
**Phone**
- Modified boot.img - neutron.img from the latest [flip2 releases](https://github.com/neutronscott/flip2/releases)

**Windows**
- adb - [Android SDK platform-tools](https://developer.android.com/tools/releases/platform-tools)
- Tool to enter fastboot mode - autobooter.exe from the latest [flip2 releases](https://github.com/neutronscott/flip2/releases)
- USB Drivers - [mtkdrivers.exe](https://scottn.us/downloads/mtkdrivers.exe)

**Linux**
- adb - Ubuntu/Debian `sudo apt install -y adb` or [Android SDK platform-tools](https://developer.android.com/tools/releases/platform-tools)
- Tool to enter fastboot mode - [mtk-bootseq.py](https://gist.github.com/neutronscott/2e4179af74c2fadec101a184fbb6a89e)
- No drivers

## Installation
- **Windows** Install drivers
- Open a shell / command prompt window where you've placed the files
- Enter fastboot mode on the phone
  - Power off the phone (I remove the battery to be sure)
  - **Windows**: Execute `autobooter.exe`
  - **Linux**: Execute `python3 mtk-bootseq.py FASTBOOT /dev/tty.usbmodem14200`\
    To find the port necessary you could watch `dmesg -w` output while rebooting the phone
- Connect phone to USB port
- Unlock the bootloader to allow modified boot partition
  - `fastboot flashing unlock`
- Flash boot partition
  - `fastboot flash boot neutron.img`
- Reboot phone
  - `fastboot reboot`
- Install Magisk
  - Dial `*#*#217703#*#*` to bring up list of apps
  - Select Magisk and allow to download full version.
  - Dial `*#*#217703#*#*` again
  - Select Magisk and allow to update and reboot.

****NOTE****: Magisk may not show up on the first boot or sometimes you cannot install APKs later. It's a timing issue with `ro.vendor.tct.endurance` getting set. Just reboot.

# Customization

You can enable ADB access simply by dialing `*#*#DEBUG#*#*` (`*#*#33284#*#*`). This is available without rooting the phone.

You should set a higher resolution. In `adb shell` execute `wm size 321x428` which seems like a good compromise.

You can disable the Power-On write protection enforced by the bootloader. This is **very risky**. Open an `adb shell` and execute `su`. Magisk should popup a dialog asking to grant superuser access. Select grant. Finally run `/vendor/bin/write_protect 0` and reboot. You can then allow Magisk to update itself with the direct-install method. Modifications to any other partition will cause problems unless you disable verity checks.