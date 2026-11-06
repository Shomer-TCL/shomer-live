#!/usr/bin/env python3
"""make-stock-recovery.py: rebuild the GENUINE stock TCL T408DL KEKA recovery, exactly the way
the phone rebuilds it itself at every boot (/vendor/bin/install-recovery.sh):

    applypatch --patch /vendor/recovery-from-boot.p
               --source boot   (25165824 bytes, sha1 d55e064c07cf4fcbc4f2870bbea70f0833653cba)
               --target recovery (25165824 bytes, sha1 40e9d0d4bc1e10efa308e9a279f70b2300ff9200)

Why: the "stock/KEKA/recovery" file in shomer-images is NOT stock. It is byte-identical to
neutronscott's recovery2.img (sha256 394ad6bb...), because his public KEKA dump came from a
phone that had it flashed. The flasher needs the real stock recovery to put back after a backup.

  make-stock-recovery.py <stock KEKA boot> <super.raw.gz with stock KEKA vendor> <out.img>

Needs: python3, bsdiff4 (pip install bsdiff4), debugfs (e2fsprogs). Fails unless the result
has the pinned sha1 AND sha256. The vendor inside every Shomer build is byte-identical to stock KEKA.
"""
import hashlib, os, shutil, struct, subprocess, sys, tempfile, zlib
import bsdiff4

DEBUGFS = shutil.which("debugfs") or next((p for p in ("/usr/sbin/debugfs", "/sbin/debugfs") if os.path.exists(p)), "debugfs")
BOOT_SHA1 = "d55e064c07cf4fcbc4f2870bbea70f0833653cba"
REC_SHA1 = "40e9d0d4bc1e10efa308e9a279f70b2300ff9200"
REC_SHA256 = "4fa8dacf08c9fe2131616d377b6c4da5b44bb504df1c67274c37aa7c38b551d3"
SIZE = 25165824


def vendor_from_super_gz(path, out):
    """Stream-decompress super, read the LP metadata, keep only the vendor extents."""
    d = zlib.decompressobj(16 + zlib.MAX_WBITS)
    buf, pos, extents, vf = bytearray(), 0, None, open(out, "wb")
    with open(path, "rb") as fh:
        while True:
            c = fh.read(1 << 20)
            if not c:
                break
            buf += d.decompress(c)
            if extents is None:
                if len(buf) < 12288 + 65536 + 4096:
                    continue
                assert struct.unpack_from("<I", buf, 4096)[0] == 0x616C4467, "no LP geometry"
                mh = 12288  # 4096 reserved + 2 x 4096 geometry
                assert struct.unpack_from("<I", buf, mh)[0] == 0x414C5030, "no LP metadata"
                hsize = struct.unpack_from("<I", buf, mh + 8)[0]
                o = mh + 4 + 2 + 2 + 4 + 32 + 4 + 32
                po, pn, ps = struct.unpack_from("<III", buf, o)
                eo, en, es = struct.unpack_from("<III", buf, o + 12)
                tb = mh + hsize
                parts = {}
                for i in range(pn):
                    e = tb + po + i * ps
                    name = bytes(buf[e:e + 36]).split(b"\0")[0].decode()
                    parts[name] = struct.unpack_from("<II", buf, e + 40)
                fe, ne = parts["vendor"]
                extents = []
                for i in range(fe, fe + ne):
                    nsec, _t, tdata, _dev = struct.unpack_from("<QIQI", buf, tb + eo + i * es)
                    extents.append((tdata * 512, nsec * 512))
            for start, ln in extents:
                a, b = max(start, pos), min(start + ln, pos + len(buf))
                if a < b:
                    vf.write(buf[a - pos:b - pos])
            pos += len(buf)
            buf = bytearray()
            if pos >= max(s + l for s, l in extents):
                break
    vf.close()


def apply_imgdiff(src, p):
    assert p[:8] == b"IMGDIFF2", "not an imgdiff patch"
    n = struct.unpack_from("<i", p, 8)[0]
    o, out = 12, bytearray()
    for _ in range(n):
        t = struct.unpack_from("<i", p, o)[0]
        o += 4
        if t == 0:  # CHUNK_NORMAL
            ss, sl, po = struct.unpack_from("<qqq", p, o)
            o += 24
            out += bsdiff4.patch(src[ss:ss + sl], p[po:])
        elif t == 2:  # CHUNK_DEFLATE
            ss, sl, po, sel, tel = struct.unpack_from("<qqqqq", p, o)
            o += 40
            level, method, wb, ml, strat = struct.unpack_from("<iiiii", p, o)
            o += 20
            exp = zlib.decompressobj(-15).decompress(src[ss:ss + sl])
            assert len(exp) == sel
            tgt = bsdiff4.patch(exp, p[po:])
            assert len(tgt) == tel
            c = zlib.compressobj(level, method, wb, ml, strat)
            out += c.compress(tgt) + c.flush()
        elif t == 3:  # CHUNK_RAW
            ln = struct.unpack_from("<i", p, o)[0]
            o += 4
            out += p[o:o + ln]
            o += ln
        else:
            raise SystemExit("unsupported imgdiff chunk type %d" % t)
    return bytes(out)


def main():
    boot_path, super_gz, out = sys.argv[1:4]
    boot = open(boot_path, "rb").read()
    if len(boot) != SIZE or hashlib.sha1(boot).hexdigest() != BOOT_SHA1:
        sys.exit("boot is not stock KEKA boot (sha1 %s)" % hashlib.sha1(boot).hexdigest())
    with tempfile.TemporaryDirectory() as td:
        v = os.path.join(td, "vendor.img")
        vendor_from_super_gz(super_gz, v)
        script = subprocess.run([DEBUGFS, "-R", "cat /bin/install-recovery.sh", v], capture_output=True, text=True).stdout
        if BOOT_SHA1 not in script or REC_SHA1 not in script:
            sys.exit("vendor install-recovery.sh does not name the expected boot/recovery hashes:\n" + script)
        pf = os.path.join(td, "rfb.p")
        subprocess.run([DEBUGFS, "-R", "dump /recovery-from-boot.p " + pf, v], check=True, capture_output=True)
        rec = apply_imgdiff(boot, open(pf, "rb").read())
    s1, s256 = hashlib.sha1(rec).hexdigest(), hashlib.sha256(rec).hexdigest()
    if len(rec) != SIZE or s1 != REC_SHA1 or s256 != REC_SHA256:
        sys.exit("result does not match the pinned stock recovery: %d bytes sha1 %s sha256 %s" % (len(rec), s1, s256))
    open(out, "wb").write(rec)
    print("stock KEKA recovery OK: %s  %d bytes  sha1 %s  sha256 %s" % (out, len(rec), s1, s256))


if __name__ == "__main__":
    main()
