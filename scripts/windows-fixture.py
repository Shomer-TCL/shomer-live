#!/usr/bin/env python3
"""Prepare hash-checked Windows GUI fixtures without Actions artifact storage."""
import importlib.util
import json
import pathlib
import shutil
import urllib.request
import zipfile

root = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('stage', root / 'scripts/stage.py')
stage = importlib.util.module_from_spec(spec)
spec.loader.exec_module(stage)
build = root / 'build'
lock = json.loads((root / 'config/assets.lock.json').read_text())
stock = next(a for a in lock['archives'] if a['name'] == 'stock-KEKA.tar.gz')
parts = []
for item in stock['parts']:
    path = build / 'input-chunks' / item['name']
    stage.checked(path, item['sha256'])
    parts.append(path)
archive = build / stock['name']
stage.join(parts, archive)
stage.checked(archive, stock['sha256'])
stage.extract(archive, build / 'stock')
rec = build / 'rec'
rec.mkdir()
shutil.copyfile(build / 'stock/stock-KEKA/recovery', rec / 'recovery2.img')
shutil.copyfile(build / 'stock/stock-KEKA/recovery-genuine', rec / 'stock-recovery-KEKA.img')
tools = json.loads((root / 'config/tools.lock.json').read_text())
tool = next(t for t in tools['downloads'] if t['group'] == 'platform-windows')
download = build / tool['name']
with urllib.request.urlopen(tool['url'], timeout=180) as source, download.open('wb') as target:
    shutil.copyfileobj(source, target)
stage.checked(download, tool['sha256'])
with zipfile.ZipFile(download) as archive:
    archive.extractall(build / 'windows-tools')
print('Verified stock recovery and Windows platform-tools fixtures')
