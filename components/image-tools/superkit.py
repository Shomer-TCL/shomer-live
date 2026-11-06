#!/usr/bin/env python3
"""Minimal liblp v10 raw single-block-device super unpack/repack helper for KEKA ONLY.
Refuses unknown layouts. Never writes to a device. Test a stock round-trip before changes.
For production use Google's lpunpack/lpmake from the matching Android build tools; this
script is an independent diagnostic, not a substitute for on-device verification.
"""
from pathlib import Path
import argparse, hashlib, struct, sys
GEOM=4096; HEADER=12288; SLOT=65536; SECTOR=512; ALIGN=2048
MAGIC=0x414c5030

def parse(fp):
    with fp.open('rb') as f:
        f.seek(GEOM); geom=f.read(52)
        assert struct.unpack_from('<I',geom)[0]==0x616c4467, 'invalid geometry magic'
        maxmeta,slots,blocksize=struct.unpack_from('<III',geom,40)
        assert (maxmeta,slots,blocksize)==(65536,2,4096), 'unexpected geometry'
        f.seek(HEADER); meta=bytearray(f.read(SLOT))
        assert struct.unpack_from('<IHHI',meta)==(MAGIC,10,0,128),'unexpected liblp metadata'
        hs=128; size=struct.unpack_from('<I',meta,44)[0]
        assert hashlib.sha256(meta[:12]+b'\0'*32+meta[44:hs]).digest()==meta[12:44], 'header checksum'
        table=bytearray(meta[hs:hs+size]); assert hashlib.sha256(table).digest()==meta[48:80], 'table checksum'
        desc=[struct.unpack_from('<III',meta,80+i*12) for i in range(4)]
        def ent(i):
            o,n,s=desc[i]; return [table[o+j*s:o+(j+1)*s] for j in range(n)]
        assert all(struct.unpack_from('<I',p,36)[0]==1 for p in ent(0)),'unexpected partition attributes'
        assert len(ent(3))==1,'multi-block-device unsupported'
        bdev=ent(3)[0]; first,align,offset,bd_size=struct.unpack_from('<QIIQ',bdev)
        assert (first,align,offset,bd_size)==(2048,1048576,0,2147483648),'unknown super device layout'
        ps=[]
        for p in ent(0):
            name=p[:36].split(b'\0')[0].decode('ascii'); attrs,ex,nex,group=struct.unpack_from('<IIII',p,36)
            assert nex==1 and name in ('product','vendor','system') and group==1,'unexpected partition'
            eo,en,esz=desc[1]; extent=table[eo+ex*esz:eo+(ex+1)*esz]
            sectors,typ,target,source=struct.unpack_from('<QIQI',extent)
            assert typ==0 and source==0 and target>=first,'nonlinear extent'
            ps.append((name,sectors,target,ex))
        assert len(ps)==3 and len(set(p[0] for p in ps))==3,'unexpected partitions'
        return meta,table,desc,ps,bd_size

def copy_region(fin,fout,sz,offset):
    fin.seek(offset)
    remaining=sz
    while remaining:
        data=fin.read(min(1<<20,remaining))
        if not data: raise EOFError('truncated image')
        if not any(data): fout.seek(len(data),1)
        else: fout.write(data)
        remaining-=len(data)

def unpack(src,dst):
    with src.open('rb') as f:
        _,_,_,ps,_=parse(src)
        dst.mkdir(parents=True,exist_ok=True)
        for name,sec,target,_ in ps:
            with (dst/(name+'.img')).open('wb') as out:
                copy_region(f,out,sec*SECTOR,target*SECTOR)
                out.truncate(sec*SECTOR)
            print(name,sec*SECTOR)

def pack(src,dst,imgs):
    meta,table,desc,ps,bd_size=parse(src)
    original={p[0]:p for p in ps}; order=sorted(ps,key=lambda x:x[2])
    nextsec=2048; new={}
    for name,oldsz,_,ex in order:
        p=imgs/(name+'.img'); assert p.is_file(),f'missing {p}'
        sz=p.stat().st_size; assert sz%4096==0,f'{name} must be 4096-byte aligned'
        if nextsec%ALIGN: nextsec+=ALIGN-nextsec%ALIGN
        new[name]=(sz//SECTOR,nextsec,ex)
        nextsec+=sz//SECTOR
    go,gn,gs=desc[2]
    assert gn==2 and table[go+gs:go+gs+36].split(b'\0')[0]==b'main','unknown partition group'
    group_limit=struct.unpack_from('<Q',table,go+gs+40)[0]
    assert sum(v[0]*SECTOR for v in new.values())<=group_limit,'main group limit exceeded'
    assert nextsec*SECTOR<=bd_size,'block device size exceeded'
    eo,en,esz=desc[1]
    for name,(sectors,target,ex) in new.items():
        struct.pack_into('<QIQI',table,eo+ex*esz,sectors,0,target,0)
    meta[128:128+len(table)]=table
    meta[48:80]=hashlib.sha256(table).digest()
    meta[12:44]=b'\0'*32
    meta[12:44]=hashlib.sha256(meta[:128]).digest()
    with src.open('rb') as orig,dst.open('wb') as out:
        # preserve geometry and reserved metadata area; overwrite all four slot copies
        out.write(orig.read(1048576))
        for i in range(4):
            out.seek(HEADER+i*SLOT); out.write(meta)
        for name,_,_,_ in order:
            sectors,target,_=new[name]
            out.seek(target*SECTOR)
            with (imgs/(name+'.img')).open('rb') as f:
                left=sectors*SECTOR
                while left:
                    b=f.read(min(1<<20,left)); assert b,'truncated partition'
                    if not any(b): out.seek(len(b),1)
                    else: out.write(b)
                    left-=len(b)
        out.truncate(bd_size)
    print('packed', dst, {k:(v[0]*SECTOR,v[1]*SECTOR) for k,v in new.items()})

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('mode',choices=['info','unpack','pack']);p.add_argument('super',type=Path);p.add_argument('out',nargs='?',type=Path);p.add_argument('--images',type=Path)
    a=p.parse_args()
    if a.mode=='info': print(parse(a.super)[3])
    elif a.mode=='unpack': unpack(a.super,a.out)
    else:
        assert a.images and a.out
        pack(a.super,a.out,a.images)
