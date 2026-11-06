# preloader info

from [MTK-bypass](https://github.com/MTK-bypass/bypass_utility). Device does not crash to brom.

```
Waiting for device
Found device = 0e8d:2000
Device hw code: 0x699
Device hw sub code: 0x8a00
Device hw version: 0xcb00
Device sw version: 0x2
Device secure boot: True
Device serial link authorization: True
Device download agent authorization: True
Found device in preloader mode, trying to crash...
status is 7024
```

# fastboot getvar all

<details>

```
(bootloader) max-download-size: 0x8000000
(bootloader) variant:
(bootloader) logical-block-size: 0x200
(bootloader) erase-block-size: 0x80000
(bootloader) hw-revision: cb00
(bootloader) battery-soc-ok: yes
(bootloader) battery-voltage: 4295mV
(bootloader) partition-size:sgpt: 8000
(bootloader) partition-type:sgpt: raw data
(bootloader) partition-size:flashinfo: 1000000
(bootloader) partition-type:flashinfo: raw data
(bootloader) partition-size:otp: 2b00000
(bootloader) partition-type:otp: raw data
(bootloader) partition-size:userdata: 1264f8000
(bootloader) partition-type:userdata: f2fs
(bootloader) partition-size:cache: 7000000
(bootloader) partition-type:cache: ext4
(bootloader) partition-size:super: 80000000
(bootloader) partition-type:super: raw data
(bootloader) partition-size:tee2: 980000
(bootloader) partition-type:tee2: raw data
(bootloader) partition-size:tee1: 300000
(bootloader) partition-type:tee1: raw data
(bootloader) partition-size:vbmeta_vendor: 100000
(bootloader) partition-type:vbmeta_vendor: raw data
(bootloader) partition-size:vbmeta_system: 100000
(bootloader) partition-type:vbmeta_system: raw data
(bootloader) partition-size:vbmeta: 100000
(bootloader) partition-type:vbmeta: raw data
(bootloader) partition-size:dtbo: 800000
(bootloader) partition-type:dtbo: raw data
(bootloader) partition-size:logo: 800000
(bootloader) partition-type:logo: raw data
(bootloader) partition-size:vendor_boot: 4000000
(bootloader) partition-type:vendor_boot: raw data
(bootloader) partition-size:boot: 1800000
(bootloader) partition-type:boot: raw data
(bootloader) partition-size:lk2: 200000
(bootloader) partition-type:lk2: raw data
(bootloader) partition-size:lk: 200000
(bootloader) partition-type:lk: raw data
(bootloader) partition-size:gz2: 1000000
(bootloader) partition-type:gz2: raw data
(bootloader) partition-size:gz1: 1000000
(bootloader) partition-type:gz1: raw data
(bootloader) partition-size:mcupmfw: 100000
(bootloader) partition-type:mcupmfw: raw data
(bootloader) partition-size:spmfw: 100000
(bootloader) partition-type:spmfw: raw data
(bootloader) partition-size:md1dsp: 1000000
(bootloader) partition-type:md1dsp: raw data
(bootloader) partition-size:md1img: 4000000
(bootloader) partition-type:md1img: raw data
(bootloader) partition-size:efuse: 80000
(bootloader) partition-type:efuse: raw data
(bootloader) partition-size:sec1: 200000
(bootloader) partition-type:sec1: raw data
(bootloader) partition-size:seccfg: 800000
(bootloader) partition-type:seccfg: raw data
(bootloader) partition-size:md_udc: 1780000
(bootloader) partition-type:md_udc: ext4
(bootloader) partition-size:protect2: 800000
(bootloader) partition-type:protect2: ext4
(bootloader) partition-size:protect1: 800000
(bootloader) partition-type:protect1: ext4
(bootloader) partition-size:metadata: 2000000
(bootloader) partition-type:metadata: raw data
(bootloader) partition-size:nvdata: 2000000
(bootloader) partition-type:nvdata: ext4
(bootloader) partition-size:nvcfg: 800000
(bootloader) partition-type:nvcfg: ext4
(bootloader) partition-size:frp: 100000
(bootloader) partition-type:frp: raw data
(bootloader) partition-size:expdb: 1400000
(bootloader) partition-type:expdb: raw data
(bootloader) partition-size:para: 80000
(bootloader) partition-type:para: raw data
(bootloader) partition-size:recovery: 1800000
(bootloader) partition-type:recovery: raw data
(bootloader) partition-size:boot_para: 100000
(bootloader) partition-type:boot_para: raw data
(bootloader) partition-size:otapkg: 1400000
(bootloader) partition-type:otapkg: ext4
(bootloader) partition-size:oempersist: 1e00000
(bootloader) partition-type:oempersist: ext4
(bootloader) partition-size:oembin: 800000
(bootloader) partition-type:oembin: ext4
(bootloader) partition-size:persist: 3000000
(bootloader) partition-type:persist: ext4
(bootloader) partition-size:swversion: 2e0000
(bootloader) partition-type:swversion: raw data
(bootloader) partition-size:loader_ext2: 10000
(bootloader) partition-type:loader_ext2: raw data
(bootloader) partition-size:loader_ext1: 10000
(bootloader) partition-type:loader_ext1: raw data
(bootloader) partition-size:nvram: 500000
(bootloader) partition-type:nvram: raw data
(bootloader) partition-size:proinfo: 7f8000
(bootloader) partition-type:proinfo: raw data
(bootloader) partition-size:pgpt: 8000
(bootloader) partition-type:pgpt: raw data
(bootloader) partition-size:preloader: 40000
(bootloader) partition-type:preloader: raw data
(bootloader) serialno: 4PPZMJ69IRRSKB6H
(bootloader) off-mode-charge: 1
(bootloader) warranty: no
(bootloader) unlocked: yes
(bootloader) secure: no
(bootloader) kernel: lk
(bootloader) product: gflip6
(bootloader) is-userspace: no
(bootloader) slot-count: 0
(bootloader) version-baseband: MOLY.LR12A.R3.MP.V179.5.P11
(bootloader) version-bootloader: gflip6-38928b3-20220304182100-202203152
(bootloader) version-preloader:
(bootloader) version: 0.5
all: Done!!
Finished. Total time: 0.292s
```
</details>

# /proc/cpuinfo

```
Processor       : ARMv7 Processor rev 4 (v7l)
model name      : ARMv7 Processor rev 4 (v7l)
BogoMIPS        : 28.64
Features        : half thumb fastmult vfp edsp neon vfpv3 tls vfpv4 idiva idivt vfpd32 lpae evtstrm aes pmull sha1 sha2 crc32
CPU implementer : 0x41
CPU architecture: 7
CPU variant     : 0x0
CPU part        : 0xd03
CPU revision    : 4

Hardware        : MT6739CH
Revision        : 0000
Serial          : 0000000000000000
```

# MMC Partition Layout
<details>

```
Disk /dev/block/mmcblk0: 15269888 sectors, 7.3 GiB
Sector size (logical/physical): 512/512 bytes
Disk identifier (GUID): 00000000-0000-0000-0000-000000000000
Partition table holds up to 128 entries
Main partition table begins at sector 2 and ends at sector 33
First usable sector is 34, last usable sector is 15269854
Partitions will be aligned on 64-sector boundaries
Total free space is 61 sectors (30.5 KiB)

Number  Start (sector)    End (sector)  Size       Code  Name
   1              64           16383   8.0 MiB     0700  proinfo
   2           16384           26623   5.0 MiB     0700  nvram
   3           26624           26751   64.0 KiB    0700  loader_ext1
   4           26752           26879   64.0 KiB    0700  loader_ext2
   5           26880           32767   2.9 MiB     0700  swversion
   6           32768          131071   48.0 MiB    0700  persist
   7          131072          147455   8.0 MiB     0700  oembin
   8          147456          208895   30.0 MiB    0700  oempersist
   9          208896          249855   20.0 MiB    0700  otapkg
  10          249856          251903   1024.0 KiB  0700  boot_para
  11          251904          301055   24.0 MiB    0700  recovery
  12          301056          302079   512.0 KiB   0700  para
  13          302080          343039   20.0 MiB    0700  expdb
  14          343040          345087   1024.0 KiB  0700  frp
  15          345088          361471   8.0 MiB     0700  nvcfg
  16          361472          427007   32.0 MiB    0700  nvdata
  17          427008          492543   32.0 MiB    0700  metadata
  18          492544          508927   8.0 MiB     0700  protect1
  19          508928          525311   8.0 MiB     0700  protect2
  20          525312          573439   23.5 MiB    0700  md_udc
  21          573440          589823   8.0 MiB     0700  seccfg
  22          589824          593919   2.0 MiB     0700  sec1
  23          593920          594943   512.0 KiB   0700  efuse
  24          594944          726015   64.0 MiB    0700  md1img
  25          726016          758783   16.0 MiB    0700  md1dsp
  26          758784          760831   1024.0 KiB  0700  spmfw
  27          760832          762879   1024.0 KiB  0700  mcupmfw
  28          762880          795647   16.0 MiB    0700  gz1
  29          795648          828415   16.0 MiB    0700  gz2
  30          828416          832511   2.0 MiB     0700  lk
  31          832512          836607   2.0 MiB     0700  lk2
  32          836608          885759   24.0 MiB    0700  boot
  33          885760         1016831   64.0 MiB    0700  vendor_boot
  34         1016832         1033215   8.0 MiB     0700  logo
  35         1033216         1049599   8.0 MiB     0700  dtbo
  36         1049600         1051647   1024.0 KiB  0700  vbmeta
  37         1051648         1053695   1024.0 KiB  0700  vbmeta_system
  38         1053696         1055743   1024.0 KiB  0700  vbmeta_vendor
  39         1055744         1061887   3.0 MiB     0700  tee1
  40         1061888         1081343   9.5 MiB     0700  tee2
  41         1081344         5275647   2.0 GiB     0700  super
  42         5275648         5505023   112.0 MiB   0700  cache
  43         5505024        15148991   4.6 GiB     0700  userdata
  44        15148992        15237055   43.0 MiB    0700  otp
  45        15237056        15269823   16.0 MiB    0700  flashinfo
```
</details>

# MMC Write Protection Groups

<details>

```
Write Protect Group size in blocks/bytes: 16384/8388608
Write Protect Groups 0-1 (Blocks 0-32767), Power-on Write Protection
Write Protect Groups 2-35 (Blocks 32768-589823), No Write Protection
Write Protect Groups 36-321 (Blocks 589824-5275647), Power-on Write Protection
Write Protect Groups 322-924 (Blocks 5275648-15155199), No Write Protection
Write Protect Groups 925-925 (Blocks 15155200-15171583), Permanent Write Protection
Write Protect Groups 926-931 (Blocks 15171584-15269887), No Write Protection
```
</details>

# super partition layout

<details>
  
  ```
  Slot 0:
Metadata version: 10.0
Metadata size: 516 bytes
Metadata max size: 65536 bytes
Metadata slot count: 2
Header flags: none
Partition table:
------------------------
  Name: product
  Group: main
  Attributes: readonly
  Extents:
    0 .. 158583 linear super 2048
------------------------
  Name: vendor
  Group: main
  Attributes: readonly
  Extents:
    0 .. 405167 linear super 161792
------------------------
  Name: system
  Group: main
  Attributes: readonly
  Extents:
    0 .. 2466943 linear super 567296
------------------------
Super partition layout:
------------------------
super: 2048 .. 160632: product (158584 sectors)
super: 161792 .. 566960: vendor (405168 sectors)
super: 567296 .. 3034240: system (2466944 sectors)
------------------------
Block device table:
------------------------
  Partition name: super
  First sector: 2048
  Size: 2147483648 bytes
  Flags: none
------------------------
Group table:
------------------------
  Name: default
  Maximum size: 0 bytes
  Flags: none
------------------------
  Name: main
  Maximum size: 2145386496 bytes
  Flags: none
------------------------
```
</details>

# vbmeta info

<details>
  <summary>vbmeta</summary>

```
Minimum libavb version:   1.0
Header Block:             256 bytes
Authentication Block:     320 bytes
Auxiliary Block:          2752 bytes
Public key (sha1):        fb86b6a7475f8ae82ee4f731a1c8e86ee27a3399
Algorithm:                SHA256_RSA2048
Rollback Index:           2
Flags:                    0
Rollback Index Location:  0
Release String:           'avbtool 1.1.0'
Descriptors:
    Chain Partition descriptor:
      Partition Name:          boot
      Rollback Index Location: 3
      Public key (sha1):       090c5e80cc8d61dc5378625c74af4daa2033ab23
    Chain Partition descriptor:
      Partition Name:          vbmeta_system
      Rollback Index Location: 2
      Public key (sha1):       8c5586ce094272bc59ede02e91e6d3132dab523d
    Chain Partition descriptor:
      Partition Name:          vbmeta_vendor
      Rollback Index Location: 4
      Public key (sha1):       3b11d91d311f5eb26cd538705f88dcad85da9d9d
    Prop: com.android.build.dtbo.fingerprint -> 'TCL/T408DL/Gflip6_TF:11/RP1A.200720.011/KEE7:user/release-keys'
    Hash descriptor:
      Image Size:            53964 bytes
      Hash Algorithm:        sha256
      Partition Name:        dtbo
      Salt:                  c83987f11437da6abe42c27f0f93c4c6d501d8c68cfe6ed1820c5f484a51a3fc
      Digest:                00d78e0a2e99189cf563fd21b2db631cdc1baab0c0cf66eb4a679f4b00c1505c
      Flags:                 0
```
</details>
<details>
  <summary>vbmeta_system</summary>

```
Minimum libavb version:   1.0
Header Block:             256 bytes
Authentication Block:     320 bytes
Auxiliary Block:          1664 bytes
Public key (sha1):        8c5586ce094272bc59ede02e91e6d3132dab523d
Algorithm:                SHA256_RSA2048
Rollback Index:           2
Flags:                    0
Rollback Index Location:  0
Release String:           'avbtool 1.1.0'
Descriptors:
    Prop: com.android.build.system.fingerprint -> 'TCL/T408DL/Gflip6_TF:11/RP1A.200720.011/KEE7:user/release-keys'
    Prop: com.android.build.system.os_version -> '11'
    Prop: com.android.build.system.security_patch -> '2021-09-05'
    Prop: com.android.build.product.fingerprint -> 'TCL/T408DL/Gflip6_TF:11/RP1A.200720.011/KEE7:user/release-keys'
    Prop: com.android.build.product.os_version -> '11'
    Prop: com.android.build.product.security_patch -> '2021-09-05'
    Hashtree descriptor:
      Version of dm-verity:  1
      Image Size:            79826944 bytes
      Tree Offset:           79826944
      Tree Size:             638976 bytes
      Data Block Size:       4096 bytes
      Hash Block Size:       4096 bytes
      FEC num roots:         2
      FEC offset:            80465920
      FEC size:              638976 bytes
      Hash Algorithm:        sha256
      Partition Name:        product
      Salt:                  0bc79aa78b8e9fad8b1d848cfab2af66d1f6d1036e1a3ee2048df5997fd123f3
      Root Digest:           34c6629a5317551570027cacc3fb2f3141083a6fb14d426725ba9bc7fa241750
      Flags:                 0
    Hashtree descriptor:
      Version of dm-verity:  1
      Image Size:            1243062272 bytes
      Tree Offset:           1243062272
      Tree Size:             9793536 bytes
      Data Block Size:       4096 bytes
      Hash Block Size:       4096 bytes
      FEC num roots:         2
      FEC offset:            1252855808
      FEC size:              9904128 bytes
      Hash Algorithm:        sha256
      Partition Name:        system
      Salt:                  d4bf80951e85766297fa3a6e33a425617b8386bd3f10c484feec823ed8e1bb09
      Root Digest:           0756f015c5a0f1ac28e0ada2f160becc37c157e22012fc4b9a45c06b846fcd8b
      Flags:                 0
```
</details>
<details>
  <summary>vbmeta_vendor</summary>

```
Minimum libavb version:   1.0
Header Block:             256 bytes
Authentication Block:     320 bytes
Auxiliary Block:          1088 bytes
Public key (sha1):        3b11d91d311f5eb26cd538705f88dcad85da9d9d
Algorithm:                SHA256_RSA2048
Rollback Index:           2
Flags:                    0
Rollback Index Location:  0
Release String:           'avbtool 1.1.0'
Descriptors:
    Prop: com.android.build.vendor.fingerprint -> 'TCL/T408DL/Gflip6_TF:11/RP1A.200720.011/KEE7:user/release-keys'
    Prop: com.android.build.vendor.os_version -> '11'
    Prop: com.android.build.vendor.security_patch -> '2021-09-05'
    Hashtree descriptor:
      Version of dm-verity:  1
      Image Size:            204083200 bytes
      Tree Offset:           204083200
      Tree Size:             1617920 bytes
      Data Block Size:       4096 bytes
      Hash Block Size:       4096 bytes
      FEC num roots:         2
      FEC offset:            205701120
      FEC size:              1630208 bytes
      Hash Algorithm:        sha256
      Partition Name:        vendor
      Salt:                  a29aed9494169872d1d8da98b26cb02cc34fa6889f0143e4e5bbc709c64ec3e4
      Root Digest:           222cd00119fbe739f81aab0a2dc092d16d268dfe8d7270d347e24b08de73b319
      Flags:                 0
```
</details>
<details>
  <summary>recovery</summary>

```
Footer version:           1.0
Image size:               25165824 bytes
Original image size:      15650816 bytes
VBMeta offset:            15650816
VBMeta size:              1472 bytes
--
Minimum libavb version:   1.0
Header Block:             256 bytes
Authentication Block:     320 bytes
Auxiliary Block:          896 bytes
Public key (sha1):        0decd6779dab1f9ba3cc1f58c2097efd7fce34a6
Algorithm:                SHA256_RSA2048
Rollback Index:           2
Flags:                    0
Rollback Index Location:  0
Release String:           'avbtool 1.1.0'
Descriptors:
    Hash descriptor:
      Image Size:            15650816 bytes
      Hash Algorithm:        sha256
      Partition Name:        recovery
      Salt:                  68b533368c668a25534e34acc870a736ee49639f9fb716771d32b4988e7cd57d
      Digest:                87fb02ef69b5448b81a03cef8b1c670e2849f2928a930d9530572794ceb86bd3
      Flags:                 0
    Prop: com.android.build.recovery.fingerprint -> 'TCL/T408DL/Gflip6_TF:11/RP1A.200720.011/KEE7:user/release-keys'
```
</details>
<details>
  <summary>neutron.img</summary>

This is the modified boot.img captured from an OTA update

```
Footer version:           1.0
Image size:               25165824 bytes
Original image size:      9129984 bytes
VBMeta offset:            9129984
VBMeta size:              1600 bytes
--
Minimum libavb version:   1.0
Header Block:             256 bytes
Authentication Block:     320 bytes
Auxiliary Block:          1024 bytes
Public key (sha1):        b7e7220dc3459af3f6dd0a678cfa83c2891b7f63
Algorithm:                SHA256_RSA2048
Rollback Index:           2
Flags:                    0
Rollback Index Location:  0
Release String:           'avbtool 1.1.0'
Descriptors:
    Hash descriptor:
      Image Size:            8896512 bytes
      Hash Algorithm:        sha256
      Partition Name:        boot
      Salt:                  1e502e6723b4f5fd8e7c528b652caed17dbdcab9d2449e3e02359198afee149d
      Digest:                88e334eeb3111ba7afd81a67d2ef7261cdc5be2fe63fbf0c10dbe81b9e54b7d2
      Flags:                 0
    Prop: com.android.build.boot.fingerprint -> 'TCL/4058R/Gflip6_ATT:11/RP1A.200720.011/NH6B:user/release-keys'
    Prop: com.android.build.boot.os_version -> '11'
    Prop: com.android.build.boot.security_patch -> '2019-06-06'
```
</details>

# input devices

<details>
  
```
Event Hub State:
  BuiltInKeyboardId: 4
  Devices:
    -1: Virtual
      Classes: 0x40000023
      Path: <virtual>
      Enabled: true
      Descriptor: a718a782d34bc767f4689c232d64d527998ea7fd
      Location:
      ControllerNumber: 0
      UniqueId: <virtual>
      Identifier: bus=0x0000, vendor=0x0000, product=0x0000, version=0x0000
      KeyLayoutFile: /system/usr/keylayout/Generic.kl
      KeyCharacterMapFile: /system/usr/keychars/Virtual.kcm
      ConfigurationFile:
      HaveKeyboardLayoutOverlay: false
      VideoDevice: <none>
    1: ACCDET
      Classes: 0x00000081
      Path: /dev/input/event0
      Enabled: true
      Descriptor: 1c78f7e0d16d4dbc8d3ab93943523f379203f90b
      Location:
      ControllerNumber: 0
      UniqueId:
      Identifier: bus=0x0019, vendor=0x0000, product=0x0000, version=0x0000
      KeyLayoutFile: /system/usr/keylayout/Generic.kl
      KeyCharacterMapFile: /system/usr/keychars/Generic.kcm
      ConfigurationFile:
      HaveKeyboardLayoutOverlay: false
      VideoDevice: <none>
    2: gpio_keys
      Classes: 0x00000001
      Path: /dev/input/event3
      Enabled: true
      Descriptor: 485d69228e24f5e46da1598745890b214130dbc4
      Location: gpio-keys/input0
      ControllerNumber: 0
      UniqueId:
      Identifier: bus=0x0019, vendor=0x0001, product=0x0001, version=0x0100
      KeyLayoutFile: /system/usr/keylayout/gpio_keys.kl
      KeyCharacterMapFile: /system/usr/keychars/Generic.kcm
      ConfigurationFile:
      HaveKeyboardLayoutOverlay: false
      VideoDevice: <none>
    3: mtk-kpd
      Classes: 0x00000001
      Path: /dev/input/event1
      Enabled: true
      Descriptor: f0d2e427e7a05eb6d316f5e14800c5ac7b6aee79
      Location:
      ControllerNumber: 0
      UniqueId:
      Identifier: bus=0x0019, vendor=0x2454, product=0x6500, version=0x0010
      KeyLayoutFile: /system/usr/keylayout/mtk-kpd.kl
      KeyCharacterMapFile: /system/usr/keychars/Generic.kcm
      ConfigurationFile:
      HaveKeyboardLayoutOverlay: false
      VideoDevice: <none>
    4: matrix-keypad (aka device 0 - built-in keyboard)
      Classes: 0x00000021
      Path: /dev/input/event2
      Enabled: true
      Descriptor: b6451db489e135ebd47fcf80eb6a5dc913c7cf38
      Location:
      ControllerNumber: 0
      UniqueId:
      Identifier: bus=0x0018, vendor=0x0000, product=0x0000, version=0x0000
      KeyLayoutFile: /system/usr/keylayout/matrix-keypad.kl
      KeyCharacterMapFile: /system/usr/keychars/Generic.kcm
      ConfigurationFile:
      HaveKeyboardLayoutOverlay: false
      VideoDevice: <none>
```
</details>

# key layout

<details>
  
  ```
gpio_keys.kl      key  252  CLAMSHELL
mtk-kpd.kl        key  114  VOLUME_DOWN
mtk-kpd.kl        key  115  VOLUME_UP
mtk-kpd.kl        key  116  POWER
mtk-kpd.kl        key  212  QUICK_DIAL
matrix-keypad.kl  key  2    1
matrix-keypad.kl  key  3    2
matrix-keypad.kl  key  4    3
matrix-keypad.kl  key  5    4
matrix-keypad.kl  key  6    5
matrix-keypad.kl  key  7    6
matrix-keypad.kl  key  8    7
matrix-keypad.kl  key  9    8
matrix-keypad.kl  key  10   9
matrix-keypad.kl  key  11   0
matrix-keypad.kl  key  139  SOFT_LEFT
matrix-keypad.kl  key  48   SOFT_RIGHT
matrix-keypad.kl  key  103  DPAD_UP
matrix-keypad.kl  key  108  DPAD_DOWN
matrix-keypad.kl  key  105  DPAD_LEFT
matrix-keypad.kl  key  106  DPAD_RIGHT
matrix-keypad.kl  key  158  BACK
matrix-keypad.kl  key  28   DPAD_CENTER
matrix-keypad.kl  key  522  STAR
matrix-keypad.kl  key  523  POUND
matrix-keypad.kl  key  231  CALL
matrix-keypad.kl  key  30   MESSENGER
matrix-keypad.kl  key  138  FAVORITE_CONTACTS
matrix-keypad.kl  key  59   SPEAKER
```

**Where is the speaker button...?**
</details>
