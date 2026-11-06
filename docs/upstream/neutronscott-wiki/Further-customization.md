# Fastbootd / Faster Flashing

Fastbootd is available as described in [Moving Fastboot to Userspace](https://source.android.com/docs/core/architecture/bootloader/fastbootd). It takes only 2min to flash a 1.5G super.img and supports logical partitions, largely making my recovery modifications obsolete.

Once you enter fastboot, issue a `fastboot reboot fastboot` to be in the linux fastbootd implementation which has a lot more umph!

On Windows you will need the win-usb drivers: [Google USB driver](https://developer.android.com/studio/run/win-usb)

# Recovery Image

Modified recovery image: https://scottn.us/downloads/recovery2.img

Added utilities: su, busybox, e2fsck, tune2fs, dmctl, lpp

Android recovery skips setting up super. For this reason I made [lpp](http://scottn.us/downloads/lpp.zip) which is a modified version of [parse-android-dynparts](https://github.com/tchebb/parse-android-dynparts) to parse the super metadata into dmctl commands (vs dmsetup). You can use it like this:

```
$ su
# lpp
dmctl create product linear 0 158584 /dev/block/by-name/super 2048
dmctl create vendor linear 0 405168 /dev/block/by-name/super 161792
dmctl create system linear 0 2466944 /dev/block/by-name/super 567296
# lpp | sh
# ls -l /dev/block/mapper
total 0
drwxr-xr-x 2 root root 100 2022-09-21 06:39 by-uuid
lrwxrwxrwx 1 root root  15 2022-09-21 06:39 product -> /dev/block/dm-0
lrwxrwxrwx 1 root root  15 2022-09-21 06:39 system -> /dev/block/dm-2
lrwxrwxrwx 1 root root  15 2022-09-21 06:39 vendor -> /dev/block/dm-1
# mount /dev/block/mapper/system /mnt/system
```

`userdata` is encrypted so without root you can try taking backups of partitions using the following to send directly over adb channel:

```
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/mmcblk0p30 2>/dev/null'" > lk.dd
```

All of them for easy copy & paste in Windows because I only script bash:

<details>

```
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/boot 2>/dev/null'" > boot.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/boot_para 2>/dev/null'" > boot_para.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/cache 2>/dev/null'" > cache.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/dtbo 2>/dev/null'" > dtbo.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/efuse 2>/dev/null'" > efuse.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/expdb 2>/dev/null'" > expdb.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/flashinfo 2>/dev/null'" > flashinfo.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/frp 2>/dev/null'" > frp.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/gz1 2>/dev/null'" > gz1.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/gz2 2>/dev/null'" > gz2.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/lk 2>/dev/null'" > lk.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/lk2 2>/dev/null'" > lk2.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/loader_ext1 2>/dev/null'" > loader_ext1.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/loader_ext2 2>/dev/null'" > loader_ext2.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/logo 2>/dev/null'" > logo.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/mcupmfw 2>/dev/null'" > mcupmfw.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/md1dsp 2>/dev/null'" > md1dsp.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/md1img 2>/dev/null'" > md1img.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/md_udc 2>/dev/null'" > md_udc.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/metadata 2>/dev/null'" > metadata.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/mmcblk0boot0 2>/dev/null'" > mmcblk0boot0.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/mmcblk0boot1 2>/dev/null'" > mmcblk0boot1.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/mmcblk0rpmb 2>/dev/null'" > mmcblk0rpmb.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/nvcfg 2>/dev/null'" > nvcfg.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/nvdata 2>/dev/null'" > nvdata.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/nvram 2>/dev/null'" > nvram.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/oembin 2>/dev/null'" > oembin.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/oempersist 2>/dev/null'" > oempersist.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/otapkg 2>/dev/null'" > otapkg.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/otp 2>/dev/null'" > otp.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/para 2>/dev/null'" > para.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/persist 2>/dev/null'" > persist.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/proinfo 2>/dev/null'" > proinfo.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/protect1 2>/dev/null'" > protect1.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/protect2 2>/dev/null'" > protect2.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/recovery 2>/dev/null'" > recovery.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/sec1 2>/dev/null'" > sec1.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/seccfg 2>/dev/null'" > seccfg.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/spmfw 2>/dev/null'" > spmfw.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/super 2>/dev/null'" > super.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/swversion 2>/dev/null'" > swversion.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/tee1 2>/dev/null'" > tee1.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/tee2 2>/dev/null'" > tee2.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/vbmeta 2>/dev/null'" > vbmeta.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/vbmeta_system 2>/dev/null'" > vbmeta_system.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/vbmeta_vendor 2>/dev/null'" > vbmeta_vendor.dd
adb exec-out "su -c 'busybox stty raw; busybox dd if=/dev/block/by-name/vendor_boot 2>/dev/null'" > vendor_boot.dd
```


</details>

# Virtual mouse toggle & intercepted keys

## Overview

Ready made applications such as Macro Droid can enable/disable the included virtual mouse by changing the global settings or running the command `settings put system keyboard_pointer_enable 1`. The built-in virtual mouse was made for the Chromium browser and has hardcoded borders to keep the pointer out of the top notification area and the bottom key shortcut labels.

One issue with using an accessibility application to trigger on key presses is that the Android framework intercepts most buttons and they
cannot be detected. To fix this a Magisk module can be made, or edits to the system partition, to replace the keylayout file to rename keys.

## vMouse module

I made a Linux native app that runs in the background and turns key presses into mouse movements. It's available [here](https://github.com/neutronscott/flip2/releases/tag/v2) as a Magisk module. Copy the zip to the phone, open Magisk, and select modules. Find the zip and see what happens.

## Remapping keys using Magisk module

Making magisk module in adb shell:

```
Gflip6_TF:/ $ su
Gflip6_TF:/ # PATH=$(magisk --path)/.magisk/busybox:$PATH
Gflip6_TF:/ # mkdir -p /data/adb/modules/mymodule && cd $_
Gflip6_TF:/data/adb/modules/mymodule # vi module.prop
Gflip6_TF:/data/adb/modules/mymodule # mkdir -p system/usr/keylayout && cd $_
Gflip6_TF:/data/adb/modules/mymodule/system/usr/keylayout # cp /system/usr/keylayout/mtk-kpd.kl .
Gflip6_TF:/data/adb/modules/mymodule/system/usr/keylayout # cp /system/usr/keylayout/matrix-keypad.kl .
Gflip6_TF:/data/adb/modules/mymodule/system/usr/keylayout # vi matrix-keypad.kl
```

The contents of module.prop is described in the [Magisk Developer Guide](https://topjohnwu.github.io/Magisk/guides.html#moduleprop)

I replaced `key 138 FAVORITE_CONTACTS` with `key 138 FOCUS` which disables the built-in function of the `*` key. (You can also change `QUICK_DIAL`
in mtk-kpd.kl but I wouldn't touch the volume/power key!) Now that it does nothing we can assign it a function with an accessibility app.

# Useful stuff

## Secret Codes
- Enable adb: `*#*#DEBUG#*#*` (`*#*#33284#*#*`)
- Launcher list all apps: `*#*#217703#*#*`
- barometer calibration: `*#*#1013#*#*`
- carrier choose: `*#*#22384#*#*`
- testing settings `*#*#73884647#*#*` (Seems to do nothing unless on a particular network?)
- Enable APN editing: `*#*#9663223#*#*`

## Properties
- Enable PackageManager: `resetprop -n ro.vendor.tct.endurance true`

## Settings
- Enable virtual mouse: `settings put system keyboard_pointer_enable 1`
- Enable developer mode: `settings put global development_settings_enabled 1`
- Allow hotspot: `settings put global hotspot_entitlement_check_mode 0`

## Commands
- Disable write protection: `/vendor/bin/write_protect 0`

## Other stuff
- More logging:
```
dumpsys package log a on
dumpsys package log DEBUG_LOGGING on
dumpsys package log ENABLE_TRACE on
```

# TODO/Progress

## Customize recovery

adb has no key storage setup. setting ro.adb.secure=0 in props allows adb to run
but then it fails with `shell_service.cpp:380 Could not set SELinux context for subprocess`

2022-09-09 /data/misc/recovery or /cache/recovery may have logs to check once back in normal mode (last_log, last_kmsg, etc)
2022-09-21 made recovery2.img with su. rather have adbd run as root but shifting focus on building a proper TWRP port since this is "good enough"

## Network unlock

There are several files left in `/vendor/etc/mddb` which leads me to believe NVRAM can be modified to disable SIM lock.
I have been unsuccessful with using Modem META just as I have been with SP Flash Tools though.

## Misc

- Disable auth in preloader (**DO NOT** overwrite the 2048 byte EMMC_BOOT header - Brick #2)
- Why some have issues with mount rw