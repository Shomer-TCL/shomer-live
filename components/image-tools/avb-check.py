#!/usr/bin/env python3
"""avb-check.py — read-only AVB check of a built image against the stock vbmeta files.

Usage: tools/avb-check.py --vbmeta stock/KEKA/vbmeta --vbmeta stock/KEKA/vbmeta_system --vbmeta stock/KEKA/vbmeta_vendor \
         --part system=system.img --part product=product.img --part vendor=vendor.img
Prints each vbmeta's flags and descriptors, then recomputes every partition's dm-verity root digest
(sha256, salted, 4096-byte blocks, exactly as AVB's hashtree descriptor defines it) and compares it to the
descriptor in the stock vbmeta. A MISMATCH means: with stock vbmeta (flags 0) that partition fails dm-verity
on the phone, so vbmeta has to be flashed with --disable-verity --disable-verification (unlocked phone only).
Writes nothing. Partition images come from tools/superkit.py unpack (or the streaming extract in the review).
Measured 2026-10-30 on builds/2026-10-30_220fdd6: vendor MATCH, system and product MISMATCH (docs/IMAGE-REVIEW-2026-10-30.md).
"""
import argparse, hashlib, struct, sys

HDR = '>IIQQIQQQQQQQQQQQII'

def parse_vbmeta(b):
    if b[:4] != b'AVB0':
        raise SystemExit('not a vbmeta image (no AVB0 magic)')
    (maj, mnr, auth_sz, aux_sz, alg, h_off, h_sz, s_off, s_sz, pk_off, pk_sz, pkm_off, pkm_sz,
     d_off, d_sz, rb_idx, flags, rb_loc) = struct.unpack(HDR, b[4:4 + struct.calcsize(HDR)])
    aux = b[256 + auth_sz:256 + auth_sz + aux_sz]
    pk = aux[pk_off:pk_off + pk_sz]
    out = dict(algorithm=alg, flags=flags, rollback_index=rb_idx,
               public_key_sha256=hashlib.sha256(pk).hexdigest() if pk else None, descriptors=[])
    d = aux[d_off:d_off + d_sz]; o = 0
    while o < len(d):
        tag, nb = struct.unpack('>QQ', d[o:o + 16]); body = d[o + 16:o + 16 + nb]
        if tag == 1:  # hashtree
            f = '>IQQQIIIQQ'; p = struct.calcsize(f)
            dmv, img_sz, t_off, t_sz, dbs, hbs, fr, fec_off, fec_sz = struct.unpack(f, body[:p])
            halg = body[p:p + 32].split(b'\0')[0].decode()
            pnl, sl, rdl, fl = struct.unpack('>IIII', body[p + 32:p + 48]); q = p + 48 + 60
            out['descriptors'].append(dict(type='hashtree', partition=body[q:q + pnl].decode(), image_size=img_sz,
                data_block=dbs, hash_block=hbs, hash_alg=halg, salt=body[q + pnl:q + pnl + sl].hex(),
                root_digest=body[q + pnl + sl:q + pnl + sl + rdl].hex(), flags=fl))
        elif tag == 2:  # hash
            img_sz, = struct.unpack('>Q', body[:8]); pnl, sl, dl, fl = struct.unpack('>IIII', body[40:56]); q = 116
            out['descriptors'].append(dict(type='hash', partition=body[q:q + pnl].decode(), image_size=img_sz))
        elif tag == 4:  # chain
            rbl, pnl, pkl = struct.unpack('>III', body[:12]); q = 76
            out['descriptors'].append(dict(type='chain', partition=body[q:q + pnl].decode(), rollback_location=rbl,
                public_key_sha256=hashlib.sha256(body[q + pnl:q + pnl + pkl]).hexdigest()))
        elif tag == 0:
            kl, vl = struct.unpack('>QQ', body[:16])
            out['descriptors'].append(dict(type='property', key=body[16:16 + kl].decode(),
                                           value=body[17 + kl:17 + kl + vl].decode(errors='replace')))
        o += 16 + nb
    return out

def root_digest(path, size, salt_hex, bs=4096):
    salt = bytes.fromhex(salt_hex); level = bytearray()
    with open(path, 'rb') as f:
        for _ in range(size // bs):
            blk = f.read(bs)
            if len(blk) != bs:
                raise SystemExit(f'{path}: shorter than the descriptor image size {size}')
            level += hashlib.sha256(salt + blk).digest()
    while True:
        if len(level) % bs:
            level += b'\0' * (bs - len(level) % bs)
        if len(level) == bs:
            return hashlib.sha256(salt + bytes(level)).hexdigest()
        level = bytearray(b''.join(hashlib.sha256(salt + bytes(level[i:i + bs])).digest()
                                   for i in range(0, len(level), bs)))

def main():
    ap = argparse.ArgumentParser(); ap.add_argument('--vbmeta', action='append', default=[])
    ap.add_argument('--part', action='append', default=[], help='name=path')
    a = ap.parse_args(); trees = {}; bad = 0
    for v in a.vbmeta:
        m = parse_vbmeta(open(v, 'rb').read())
        print(f"{v}: flags={m['flags']} ({'verification ON' if m['flags'] == 0 else 'flags set'}) "
              f"alg={m['algorithm']} rollback={m['rollback_index']} key={str(m['public_key_sha256'])[:16]}")
        for d in m['descriptors']:
            print('   ', d)
            if d['type'] == 'hashtree':
                trees[d['partition']] = d
    for spec in a.part:
        name, path = spec.split('=', 1)
        if name not in trees:
            print(f'{name}: no hashtree descriptor in the given vbmeta files'); continue
        d = trees[name]; got = root_digest(path, d['image_size'], d['salt'])
        ok = got == d['root_digest']; bad += not ok
        print(f"{name}: computed {got} vbmeta {d['root_digest']} {'MATCH' if ok else 'MISMATCH'}")
    if bad:
        print(f'{bad} partition(s) differ from the stock hashtree: flash vbmeta with '
              '--disable-verity --disable-verification (unlocked phone only, never relock).')
    return 1 if bad else 0

if __name__ == '__main__':
    sys.exit(main())
