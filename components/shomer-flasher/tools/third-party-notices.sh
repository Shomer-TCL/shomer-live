#!/usr/bin/env bash
# Writes cmd/shomer-flasher/THIRD_PARTY_LICENSES.txt: the licence of every Go module
# compiled into the Linux and Windows flashers (shown by --licenses). Run after changing go.mod.
set -euo pipefail
out=cmd/shomer-flasher/THIRD_PARTY_LICENSES.txt
mods=$( (go list -deps -f '{{if not .Standard}}{{.Module}}{{end}}' ./cmd/shomer-flasher
         GOOS=windows go list -deps -f '{{if not .Standard}}{{.Module}}{{end}}' ./cmd/shomer-flasher) \
       | grep -v '^github.com/Shomer-TCL/' | sort -u)
{
  echo "shomer-flasher includes these open-source Go modules. Their licences follow."
  echo "(Fastboot/adb, recovery2.img, Magisk and BusyBox notices: see README.md.)"
  echo
  while read -r m v; do
    d=$(go list -m -f '{{.Dir}}' "$m")
    echo "================================================================================"
    echo "$m $v"
    echo "================================================================================"
    f=$(ls "$d" | grep -i -E '^(licen[cs]e|copying)' | head -1 || true)
    if [ -n "$f" ]; then cat "$d/$f"; else
      echo "(no licence file in this module version; its README says: $(grep -i -A2 '^## licen' "$d/README.md" | tail -n +2 | tr -s '\n' ' '))"
      echo "MIT License - Copyright (c) Yasuhiro Matsumoto (as stated by the project)"
    fi
    echo
  done <<< "$mods"
} > "$out"
echo "wrote $out ($(wc -c < "$out") bytes, $(echo "$mods" | wc -l) modules)"
