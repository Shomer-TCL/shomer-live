#!/usr/bin/env python3
"""Assemble a complete offline filesystem. Networking happens only during build.
Every external binary/archive is hash-checked before use; no credentials are copied.
"""
import argparse,base64,concurrent.futures,gzip,hashlib,json,os,pathlib,shutil,subprocess,tarfile,urllib.request,zipfile
ROOT=pathlib.Path(__file__).resolve().parents[1]
BUILD=ROOT/'build';FS=BUILD/'root';KIT=FS/'opt/shomer/TCL-Flip2'

def digest(path):
    h=hashlib.sha256()
    with path.open('rb') as f:
        for b in iter(lambda:f.read(1024*1024),b''):h.update(b)
    return h.hexdigest()
def checked(path,sha):
    if digest(path)!=sha:raise ValueError('Checksum mismatch: '+str(path))
def run(*args,cwd=ROOT,env=None):subprocess.run(list(map(str,args)),cwd=cwd,env=env,check=True)
def copy(src,dst):
    dst.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(src,dst)
def tree(src,dst):shutil.copytree(src,dst,dirs_exist_ok=True,ignore=shutil.ignore_patterns('.git','__pycache__','*.pyc'))
def executable(path,text):
    path.parent.mkdir(parents=True,exist_ok=True);path.write_text(text);path.chmod(0o755)
def extract(tar,dest):
    dest.mkdir(parents=True,exist_ok=True)
    with tarfile.open(tar) as t:
        for m in t.getmembers():
            target=(dest/m.name).resolve()
            if not target.is_relative_to(dest.resolve()) or m.issym() or m.islnk() or not (m.isfile() or m.isdir()):raise ValueError('Unsafe archive member: '+m.name)
        t.extractall(dest,filter='data')
def join(parts,out):
    out.parent.mkdir(parents=True,exist_ok=True)
    with out.open('wb') as f:
        for p in parts:
            with p.open('rb') as r:shutil.copyfileobj(r,f,1024*1024)
def archive_inputs(inputs):
    lock=json.loads((ROOT/'config/assets.lock.json').read_text())
    for item in lock['archives']:
        archive=BUILD/'archives'/item['name'];archive.parent.mkdir(parents=True,exist_ok=True)
        existing=inputs/item['name']
        if existing.exists():copy(existing,archive)
        else:
            chunks=[]
            for p in item['parts']:
                chunk=inputs/p['name'];checked(chunk,p['sha256']);chunks.append(chunk)
            join(chunks,archive)
        checked(archive,item['sha256']);extract(archive,BUILD/'inputs')
def tools():
    cache=BUILD/'downloads';cache.mkdir(parents=True,exist_ok=True)
    lock=json.loads((ROOT/'config/tools.lock.json').read_text())
    def download(item):
        p=cache/item['name']
        if not p.exists():
            with urllib.request.urlopen(item['url'],timeout=180) as r,p.with_suffix(p.suffix+'.partial').open('wb') as f:shutil.copyfileobj(r,f)
            p.with_suffix(p.suffix+'.partial').rename(p)
        checked(p,item['sha256']);return item,p
    for item,p in concurrent.futures.ThreadPoolExecutor(max_workers=4).map(download,lock['downloads']):
        group=item['group']
        if group.startswith('platform-'):
            dst=KIT/'Tools/Android-platform-tools'/group.removeprefix('platform-')
            with zipfile.ZipFile(p) as z:z.extractall(dst)
            for name in ['adb','fastboot']:
                f=dst/'platform-tools'/name
                if f.exists():f.chmod(0o755)
        elif group=='neutronscott':copy(p,KIT/('Images/Neutronscott' if pathlib.Path(item['original_name']).suffix in ('.img','.bin') else 'Tools/Neutronscott')/item['release']/item['original_name'])
        elif group=='avbtool':executable(KIT/'Tools/Image-tools/avbtool.py',base64.b64decode(p.read_bytes()).decode())
        elif group=='magisk':
            copy(p,KIT/'Tools/Neutronscott/Magisk-v27.0.apk')
            with zipfile.ZipFile(p) as z:
                target=KIT/'Tools/Image-tools/magiskboot';target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(z.read('lib/x86_64/libmagiskboot.so'));target.chmod(0o755)
    copy(ROOT/'config/tools.lock.json',KIT/'Checksums-and-manifests/tools.lock.json')
def firmware():
    for stock in ['KEKA','KEE7']:
        src=BUILD/'inputs'/('stock-'+stock);dst=KIT/'Images/Stock/T408DL'/stock;tree(src,dst)
        combined=dst/'super.gz';join(sorted(dst.glob('super.gz.part-*')),combined)
        expected=next(line.split()[0] for line in (dst/'SHA256SUMS').read_text().splitlines() if line.split()[-1]=='super.gz')
        checked(combined,expected)
        for p in dst.glob('super.gz.part-*'):p.unlink()
    current=KIT/'Images/Shomer/2026-11-05_785693c';tree(BUILD/'inputs/shomer-current',current)
    join(sorted(current.glob('super.raw.gz.part-*')),current/'super.raw.gz')
    plan=json.loads((current/'flash-plan.json').read_text());checked(current/'super.raw.gz',plan['super_raw_gz']['sha256'])
    for p in current.glob('super.raw.gz.part-*'):p.unlink()
    raw=BUILD/'super.raw'
    with gzip.open(current/'super.raw.gz','rb') as r,raw.open('wb') as f:shutil.copyfileobj(r,f,1024*1024)
    checked(raw,plan['write_order'][-1]['sha256'])
    # Explicit names prevent confusing the rooted KEKA 'recovery' dump with stock.
    keka=KIT/'Images/Stock/T408DL/KEKA'
    (keka/'recovery').rename(keka/'ROOTED-recovery2.img');(keka/'recovery-genuine').rename(keka/'STOCK-recovery-KEKA.img')
    index={'supported_shomer':['T408DL KEKA','corresponding 4058G UPCI'],'shomer_build':current.name,
           'source_commit':json.loads((ROOT/'config/assets.lock.json').read_text())['archives'][0]['source_commit'],
           'recovery2':'Stock/T408DL/KEKA/ROOTED-recovery2.img','genuine_stock_recovery':'Stock/T408DL/KEKA/STOCK-recovery-KEKA.img',
           'warnings':['KEE7 is a stock/reference source, not an approved starting build for this Shomer image.','Neutronscott boot images are version-specific reference assets; never selected automatically.','No identity/calibration partition from another handset may be restored.']}
    (KIT/'Images/INDEX.json').write_text(json.dumps(index,indent=2)+'\n')
def build_apps():
    source=ROOT/'components/shomer-flasher';out=KIT/'Shomer-Flasher';out.mkdir(parents=True,exist_ok=True)
    version=os.environ.get('VERSION','live-dev')
    run('go','build','-trimpath','-buildvcs=false','-o',BUILD/'mkbundle','./cmd/mkbundle',cwd=source)
    run('go','build','-trimpath','-buildvcs=false','-tags','gtk','-ldflags','-X main.version='+version,'-o',BUILD/'linux-stub','./cmd/shomer-flasher',cwd=source)
    pt=KIT/'Tools/Android-platform-tools/linux/platform-tools';keka=KIT/'Images/Stock/T408DL/KEKA';current=KIT/'Images/Shomer/2026-11-05_785693c'
    common=['-super',current/'super.raw.gz','-super-raw-sha','49303b828dff82aa5bdeea69f0acf8a5ceb5fdb59df1e34419cf168a407ddf0a','-super-raw-size','2147483648','-vbmeta-dir',keka,'-build',current.name,'-commit','785693cf8a4b3942eeddb068d9aad43d9089cf98','-flash-plan',current/'flash-plan.json','-dump-recovery',keka/'ROOTED-recovery2.img','-stock-recovery',keka/'STOCK-recovery-KEKA.img']
    run(BUILD/'mkbundle','-stub',BUILD/'linux-stub','-out',out/'shomer-flasher','-goos','linux','-tools',pt,'-tool-files','fastboot,adb',*common)
    env=dict(os.environ,CGO_ENABLED='0',GOOS='windows',GOARCH='amd64')
    run('go','build','-trimpath','-buildvcs=false','-ldflags','-X main.version='+version,'-o',BUILD/'windows-stub.exe','./cmd/shomer-flasher',cwd=source,env=env)
    windows=ROOT/'dist/shomer-flasher-windows-x64.exe';windows.parent.mkdir(exist_ok=True)
    run(BUILD/'mkbundle','-stub',BUILD/'windows-stub.exe','-out',windows,'-goos','windows','-tools',KIT/'Tools/Android-platform-tools/windows/platform-tools','-tool-files','fastboot.exe,adb.exe,AdbWinApi.dll,AdbWinUsbApi.dll,libwinpthread-1.dll',*common)
    fixture=BUILD/'fixture';rec=BUILD/'rec';rec.mkdir(exist_ok=True)
    copy(keka/'ROOTED-recovery2.img',rec/'recovery2.img');copy(keka/'STOCK-recovery-KEKA.img',rec/'stock-recovery-KEKA.img')
    env=dict(os.environ,SHOMER_GUI_TAGS='gtk',SHOMER_CGO_ENABLED='1',PT_WINDOWS=str(KIT/'Tools/Android-platform-tools/windows/platform-tools'))
    run('bash','test/make-test-flasher.sh',rec,fixture,cwd=source,env=env)
    tree(fixture,KIT/'Tools/Tests/fixture');tree(source/'test',KIT/'Tools/Tests/source')
def runtime():
    for f in (ROOT/'runtime').iterdir():
        if f.is_file():copy(f,FS/'usr/local/bin'/f.name);(FS/'usr/local/bin'/f.name).chmod(0o755)
    copy(ROOT/'config/51-shomer-flip.rules',FS/'etc/udev/rules.d/51-shomer-flip.rules')
    copy(ROOT/'config/shomer-workbench.desktop',FS/'usr/share/applications/shomer-workbench.desktop')
    docs=KIT/'START-HERE';docs.mkdir(parents=True,exist_ok=True)
    for f in (ROOT/'docs/offline').glob('*'):copy(f,docs/f.name)
    tree(ROOT/'docs',KIT/'Documentation')
    for name in ['mtkclient','neutronscott','image-tools']:
        dst={'mtkclient':'Tools/MTKClient/source','neutronscott':'Tools/Neutronscott/source','image-tools':'Tools/Image-tools/source'}[name]
        tree(ROOT/'components'/name,KIT/dst)
    tree(ROOT/'components/shomer-flasher',KIT/'Licenses-and-source/shomer-flasher')
    copy(ROOT/'config/mtk-requirements.txt',KIT/'Tools/MTKClient/requirements.txt')
    executable(FS/'usr/local/bin/mtk','#!/bin/sh\nexec /opt/shomer/TCL-Flip2/Tools/MTKClient/venv/bin/python -u /opt/shomer/TCL-Flip2/Tools/MTKClient/source/mtk.py "$@"\n')
    executable(FS/'usr/local/bin/mtk-gui','#!/bin/sh\nexec /opt/shomer/TCL-Flip2/Tools/MTKClient/venv/bin/python /opt/shomer/TCL-Flip2/Tools/MTKClient/source/mtk_gui.py "$@"\n')
    for name,target in [('adb','Tools/Android-platform-tools/linux/platform-tools/adb'),('fastboot','Tools/Android-platform-tools/linux/platform-tools/fastboot'),('avbtool','Tools/Image-tools/avbtool.py'),('magiskboot','Tools/Image-tools/magiskboot')]:
        executable(FS/'usr/local/bin'/name,'#!/bin/sh\nexec /opt/shomer/TCL-Flip2/'+target+' "$@"\n')
    executable(FS/'usr/local/bin/superkit','#!/bin/sh\nexec python3 /opt/shomer/TCL-Flip2/Tools/Image-tools/source/superkit.py "$@"\n')
    executable(FS/'usr/local/bin/lpunpack','#!/bin/sh\nexec python3 /opt/shomer/TCL-Flip2/Tools/Image-tools/source/lpunpack/lpunpack.py "$@"\n')
    for name in ['shomer-boot-test.service']:
        copy(ROOT/'config'/name,FS/'etc/systemd/system'/name)
    wants=FS/'etc/systemd/system/multi-user.target.wants';wants.mkdir(parents=True,exist_ok=True)
    link=wants/'shomer-boot-test.service'
    if not link.exists():link.symlink_to('../shomer-boot-test.service')
    manifest=KIT/'Checksums-and-manifests';manifest.mkdir(parents=True,exist_ok=True)
    for name in ['assets.lock.json','sources.lock.json']:copy(ROOT/'config'/name,manifest/name)
    # Explicit deny patterns guard accidental inclusion of production/private data.
    forbidden=['shomer-prod','test-secrets.json','.dev.vars','.p12','.pass','.pk8']
    for p in FS.rglob('*'):
        if p.is_file() and any(x in p.name for x in forbidden):raise ValueError('Forbidden private material in ISO: '+str(p))
def inventory():
    out=KIT/'Checksums-and-manifests/files.sha256'
    with out.open('w') as f:
        for p in sorted(KIT.rglob('*')):
            if p.is_file() and p!=out:f.write(digest(p)+'  '+str(p.relative_to(KIT))+'\n')

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--inputs',required=True,type=pathlib.Path);args=ap.parse_args()
    if FS.exists():raise ValueError('Staging root exists; preserve or explicitly remove the previous generated build/root first')
    KIT.mkdir(parents=True);(KIT/'Checksums-and-manifests').mkdir()
    archive_inputs(args.inputs);tools();firmware();build_apps();runtime()
    wheelhouse=KIT/'Tools/MTKClient/wheels';wheelhouse.mkdir(parents=True)
    run('python3','-m','pip','download','--require-hashes','--only-binary=:all:','--python-version','3.13','--dest',wheelhouse,'-r',ROOT/'config/mtk-requirements.txt')
    inventory()
    print('Offline filesystem staged:',FS)
if __name__=='__main__':main()
