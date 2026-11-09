#!/usr/bin/env python3
"""Hash immutable toolkit files; Python bytecode caches are derived at runtime."""
import argparse
import hashlib
import pathlib


def write(root):
    root = pathlib.Path(root)
    manifest = root / 'Checksums-and-manifests/files.sha256'
    manifest.parent.mkdir(parents=True, exist_ok=True)
    with manifest.open('w') as out:
        for path in sorted(root.rglob('*')):
            relative = path.relative_to(root)
            if (not path.is_file() or path == manifest or '__pycache__' in relative.parts
                    or path.suffix in ('.pyc', '.pyo')):
                continue
            digest = hashlib.sha256()
            with path.open('rb') as source:
                for block in iter(lambda: source.read(1024 * 1024), b''):
                    digest.update(block)
            out.write(digest.hexdigest() + '  ' + str(relative) + '\n')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('root', type=pathlib.Path)
    write(parser.parse_args().root)
