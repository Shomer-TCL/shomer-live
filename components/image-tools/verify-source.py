#!/usr/bin/env python3
"""Verify neutronscott's nonstandard SHA256SUMS.txt lines: SHA256  name  (N bytes)."""
import argparse,hashlib,pathlib,re,sys
p=argparse.ArgumentParser();p.add_argument('folder',type=pathlib.Path);a=p.parse_args()
root=a.folder;lines=(root/'SHA256SUMS.txt').read_text().splitlines();bad=[]
for line in lines:
 m=re.fullmatch(r'([0-9a-f]{64})  (.+?)  \((\d+) bytes\)',line)
 if m is None: bad.append(('bad manifest line',line));continue
 digest,name,size=m.groups();f=root/name
 if not f.is_file():bad.append(('missing file',name));continue
 if f.stat().st_size!=int(size):bad.append(('wrong byte size',name));continue
 h=hashlib.sha256()
 with f.open('rb') as fd:
  for chunk in iter(lambda: fd.read(1024*1024),b''):h.update(chunk)
 if h.hexdigest()!=digest:bad.append(('wrong sha256',name))
print(f'{root}: {len(lines)-len(bad)}/{len(lines)} files verified')
for issue in bad:print(*issue,file=sys.stderr)
sys.exit(bool(bad))
