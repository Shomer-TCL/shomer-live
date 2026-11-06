# Building and releasing the flashers

## The easy way: GitHub Actions

Actions > **build-release** > Run workflow, with:

- `build_id` - the folder under `builds/` in the private `Shomer-TCL/shomer-images` repo (for example `2026-10-30_220fdd6`).
- `super_raw_sha256` - SHA-256 of the **decompressed** super partition. Get it with
  `cat super.raw.gz.part-* | gunzip | sha256sum` (or from the build notes).
- `version` - the flasher version, e.g. `1.0.1`.

The workflow:

1. runs the unit tests,
2. fetches `builds/<build_id>/super.raw.gz.part-*` and `stock/KEKA/vbmeta*` from `shomer-images` with a **read-only deploy key** (repository secret `IMAGES_DEPLOY_KEY`; its public half is a read-only deploy key on `shomer-images` named `shomer-flasher CI (read-only)`), and checks every SHA-256 (the parts' `SHA256SUMS`, the stock vbmeta `SHA256SUMS`, and the decompressed super against `super_raw_sha256`),
3. downloads Android platform-tools for Linux and Windows from Google,
4. builds both one-file flashers with `build.sh`,
5. runs `test/e2e.py` against the Linux binary (mock fastboot, mock adb, mock preloader on a pty),
6. creates a **draft** Release with both binaries and `SHA256SUMS`,
7. on a Windows runner: runs the unit tests, downloads the `.exe` from the draft and runs `--info` and `--dry-run` (full payload check and unpack on Windows),
8. publishes the Release.

Pushing a tag `v*` runs the same with the default build.

## By hand (Linux)

Needs Go 1.22+, `unzip`, Python 3 (tests).

```
# inputs
cat super.raw.gz.part-* > super.raw.gz              # from shomer-images/builds/<id>/, check SHA256SUMS
gunzip -c super.raw.gz | sha256sum                 # = SUPER_RAW_SHA
# stock KEKA vbmeta, vbmeta_system, vbmeta_vendor from shomer-images/stock/KEKA/ (check SHA256SUMS)
# platform-tools: https://dl.google.com/android/repository/platform-tools-latest-{linux,windows}.zip

SUPER_GZ=$PWD/super.raw.gz SUPER_RAW_SHA=<sha> VBMETA_DIR=$PWD/stock \
PT_LINUX=$PWD/ptl/platform-tools PT_WINDOWS=$PWD/ptw/platform-tools \
DUMP_RECOVERY=$PWD/rec/recovery2.img STOCK_RECOVERY=$PWD/rec/stock-recovery-KEKA.img \
BUILD_ID=<build_id> ROM_COMMIT=<rom commit> FLASH_PLAN=$PWD/flash-plan.json OUT=$PWD/dist ./build.sh

python3 test/e2e.py dist/shomer-flasher-linux-x86_64
```

`build.sh` refuses vbmeta files whose SHA-256 is not the stock KEKA one.

**Flash plan (since Nov 2 2026).** Every image build folder carries `flash-plan.json` (made by shomer-rom
`tools/make-flash-plan.py` from `variants/v3-strip/flash-plan.json`): the one source of truth for what a flash writes.
`mkbundle -flash-plan` (required) stops the build unless the plan lists exactly this flasher's writes, in this order
(vbmeta, vbmeta_system, vbmeta_vendor with `--disable-verity --disable-verification`, then super), with the same stock
vbmeta SHA-256s and the same decompressed super SHA-256, and lists nothing the allow-list refuses. The plan is
bundled inside the flasher as `flash-plan.json`. Tests: `internal/flashplan` (go test).

**Backup-first in the plan (since 1.4.0, Nov 3 2026).** The plan must also carry a `backup_first` section naming the
only two writes to `recovery`: step 1 `dump` = neutronscott recovery2.img (sha256 `394ad6bb...`, 25,165,824 B), step 2
`restore_stock` = genuine stock KEKA recovery (sha256 `4fa8dacf...`). `recovery` must stay in `never_written` (no general
write) and never appear in `write_order`. mkbundle stops (error, not warning) if the section is missing or differs, or if
`-dump-recovery` / `-stock-recovery` are not given or are not the pinned bytes; `build.sh` requires `DUMP_RECOVERY` and
`STOCK_RECOVERY`. A flasher without backup-first can no longer be built (the user can still skip it with `--no-backup`).

## Payload format

`[program][tar: manifest.json, tools/<os>/*, avb/vbmeta*, rom/super.raw.gz][64-byte trailer]`

Trailer: `SHOMERFL` | format version (u64 LE) | tar offset (u64) | tar length (u64) | SHA-256 of manifest.json (32 bytes).
`manifest.json` lists every file with size and SHA-256, the decompressed super's size and SHA-256, the vbmeta partition names and the accepted products/models/builds. The program refuses to run if any check fails, before it talks to a phone.

## Changing what phones are accepted

`cmd/mkbundle/main.go`: `Products` (prefix of `fastboot getvar product`), `Models` (substring of `ro.product.model`), `Builds` (substring of `ro.build.version.incremental`, or what the person types). Do not add a model without a build made for it: a regular 4058G (UPBO/UPE6/UPEJ) needs its own vendor part.
