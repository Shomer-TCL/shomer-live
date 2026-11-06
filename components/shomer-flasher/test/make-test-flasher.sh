#!/usr/bin/env bash
# Builds a SMALL test flasher (fake 6 MB ROM, fake vbmeta) that carries the REAL
# pinned recovery images, for test/e2e.py and test/e2e_tui.py.
#   test/make-test-flasher.sh <dir with recovery2.img + stock-recovery-KEKA.img> <out dir>
# With PT_WINDOWS=<Windows platform-tools folder> it also builds shomer-flasher-test.exe
# (same fake ROM, real Windows fastboot/adb) for test/e2e_gui_windows.py.
set -euo pipefail
REC=$1; T=$2
mkdir -p "$T/vb" "$T/pt"
python3 - "$T" <<'EOF'
import os, gzip, hashlib, json, sys
t = sys.argv[1]
raw = bytearray(6 << 20); raw[1 << 20:3 << 20] = os.urandom(2 << 20)
open(t + "/super.raw.gz", "wb").write(gzip.compress(bytes(raw)))
rs = hashlib.sha256(raw).hexdigest(); open(t + "/rawsha", "w").write(rs)
shas = {}
for v in ["vbmeta", "vbmeta_system", "vbmeta_vendor"]:
    b = b"AVB0" + os.urandom(4092); open(t + "/vb/" + v, "wb").write(b); shas[v] = hashlib.sha256(b).hexdigest()
plan = json.load(open("internal/flashplan/testdata/flash-plan.json"))
for s in plan["write_order"]:
    s["sha256"] = shas.get(s["partition"], rs)
json.dump(plan, open(t + "/plan.json", "w"))
for x in ["fastboot", "adb"]:
    open(t + "/pt/" + x, "w").write("#!/bin/sh\n")
EOF
go build -o "$T/mkbundle" ./cmd/mkbundle
tags=()
if [ -n "${SHOMER_GUI_TAGS:-}" ]; then tags=(-tags "$SHOMER_GUI_TAGS"); fi
CGO_ENABLED=${SHOMER_CGO_ENABLED:-0} go build "${tags[@]}" -ldflags "-X main.version=ci-test" -o "$T/stub" ./cmd/shomer-flasher
"$T/mkbundle" -stub "$T/stub" -out "$T/shomer-flasher-test" -goos linux -tools "$T/pt" -tool-files fastboot,adb \
  -super "$T/super.raw.gz" -super-raw-sha "$(cat "$T/rawsha")" -super-raw-size 6291456 -vbmeta-dir "$T/vb" \
  -build ci-test -commit "$(git rev-parse --short=7 HEAD)" -flash-plan "$T/plan.json" \
  -dump-recovery "$REC/recovery2.img" -stock-recovery "$REC/stock-recovery-KEKA.img"
if [ -n "${PT_WINDOWS:-}" ]; then
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-X main.version=ci-test" -o "$T/stub.exe" ./cmd/shomer-flasher
  "$T/mkbundle" -stub "$T/stub.exe" -out "$T/shomer-flasher-test.exe" -goos windows -tools "$PT_WINDOWS" \
    -tool-files fastboot.exe,adb.exe,AdbWinApi.dll,AdbWinUsbApi.dll,libwinpthread-1.dll \
    -super "$T/super.raw.gz" -super-raw-sha "$(cat "$T/rawsha")" -super-raw-size 6291456 -vbmeta-dir "$T/vb" \
    -build ci-test -commit "$(git rev-parse --short=7 HEAD)" -flash-plan "$T/plan.json" \
    -dump-recovery "$REC/recovery2.img" -stock-recovery "$REC/stock-recovery-KEKA.img"
  echo "$T/shomer-flasher-test.exe"
fi
echo "$T/shomer-flasher-test"
