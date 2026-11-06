#!/bin/sh
set -eu
cd "$(dirname "$0")"
sha256sum -c PARTS.sha256
name=$(sed -n 's/^[a-f0-9]*  \(shomer-live-[A-Za-z0-9.-]*-amd64\.iso\)$/\1/p' ISO.sha256)
test -n "$name"
test ! -e "$name"
test ! -e "$name.partial"
cat "$name".part-* > "$name.partial"
expected=$(cut -d' ' -f1 ISO.sha256)
printf '%s  %s\n' "$expected" "$name.partial" | sha256sum -c -
mv "$name.partial" "$name"
printf 'Verified ISO ready: %s\n' "$name"
