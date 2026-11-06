#!/usr/bin/env bash
# Builds the two one-file flashers. Needs: Go 1.22+, the ROM image, stock KEKA vbmeta files,
# Android platform-tools for Linux and Windows. See docs/BUILDING.md.
set -euo pipefail
cd "$(dirname "$0")"

: "${SUPER_GZ:?path to super.raw.gz (Shomer image)}"
: "${SUPER_RAW_SHA:?sha256 of the decompressed super}"
: "${SUPER_RAW_SIZE:=2147483648}"
: "${VBMETA_DIR:?folder with stock KEKA vbmeta, vbmeta_system, vbmeta_vendor}"
: "${PT_LINUX:?Linux platform-tools folder}"
: "${PT_WINDOWS:?Windows platform-tools folder}"
: "${BUILD_ID:?image build id, e.g. 2026-10-30_220fdd6}"
: "${ROM_COMMIT:?ROM repo commit}"
: "${FLASH_PLAN:?flash-plan.json from the image build folder (shomer-images builds/<id>/flash-plan.json)}"
VERSION="${VERSION:-1.0.0}"
# backup-first (required since 1.4.0; the flash plan's backup_first section names both):
# neutronscott recovery2.img + genuine stock KEKA recovery (tools/make-stock-recovery.py)
: "${DUMP_RECOVERY:?recovery2.img (shomer-images stock/KEKA/recovery, sha256 394ad6bb...)}"
: "${STOCK_RECOVERY:?genuine stock KEKA recovery (tools/make-stock-recovery.py or shomer-images stock/KEKA/recovery-genuine, sha256 4fa8dacf...)}"
RECFLAGS=(-dump-recovery "$DUMP_RECOVERY" -stock-recovery "$STOCK_RECOVERY")
TARGETS="${TARGETS:-linux windows}"
OUT="${OUT:-dist}"
# stock KEKA vbmeta checksums (neutronscott dump, shomer-images stock/KEKA/SHA256SUMS)
VBSHA="vbmeta=0f1c398d3a7699701616b19a8d572c99a574d8dfdcd0285ed6e767872d14d0f8,vbmeta_system=c482fe430b276d3fa26d7d08a9de8f6080baf276f1953609c9850946c886aa92,vbmeta_vendor=b77ed5c44d6046122ac5e9305ba7553499aa2eb27eecc4dfcca9ceb9f02ddb7c"

mkdir -p "$OUT" build
go test ./...
go build -o build/mkbundle ./cmd/mkbundle
for t in $TARGETS; do
  case $t in
    linux)
      CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o build/stub-linux ./cmd/shomer-flasher
      build/mkbundle -stub build/stub-linux -out "$OUT/shomer-flasher-linux-x86_64" -goos linux \
        -tools "$PT_LINUX" -tool-files fastboot,adb \
        -super "$SUPER_GZ" -super-raw-sha "$SUPER_RAW_SHA" -super-raw-size "$SUPER_RAW_SIZE" \
        -vbmeta-dir "$VBMETA_DIR" -vbmeta-sha "$VBSHA" -build "$BUILD_ID" -commit "$ROM_COMMIT" -flash-plan "$FLASH_PLAN" "${RECFLAGS[@]}"
      ;;
    windows)
      CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o build/stub-windows.exe ./cmd/shomer-flasher
      build/mkbundle -stub build/stub-windows.exe -out "$OUT/shomer-flasher-windows-x64.exe" -goos windows \
        -tools "$PT_WINDOWS" -tool-files fastboot.exe,adb.exe,AdbWinApi.dll,AdbWinUsbApi.dll,libwinpthread-1.dll \
        -super "$SUPER_GZ" -super-raw-sha "$SUPER_RAW_SHA" -super-raw-size "$SUPER_RAW_SIZE" \
        -vbmeta-dir "$VBMETA_DIR" -vbmeta-sha "$VBSHA" -build "$BUILD_ID" -commit "$ROM_COMMIT" -flash-plan "$FLASH_PLAN" "${RECFLAGS[@]}"
      ;;
  esac
done
(cd "$OUT" && sha256sum shomer-flasher-* > SHA256SUMS.part && cat SHA256SUMS.part)
