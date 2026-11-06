#!/usr/bin/env python3
import argparse,hashlib,json,pathlib,re,shutil
def sha(p):
    h=hashlib.sha256()
    with p.open('rb') as f:
        for b in iter(lambda:f.read(1024*1024),b''):h.update(b)
    return h.hexdigest()
def main():
    ap=argparse.ArgumentParser();ap.add_argument('iso',type=pathlib.Path);ap.add_argument('--version',required=True);ap.add_argument('--out',type=pathlib.Path,default=pathlib.Path('dist/release'));a=ap.parse_args()
    if not re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?',a.version):raise ValueError('Invalid release version')
    a.out.mkdir(parents=True,exist_ok=True);name=f'shomer-live-{a.version}-amd64.iso'
    manifest={'image':name,'sha256':sha(a.iso),'size':a.iso.stat().st_size,'parts':[]}
    with a.iso.open('rb') as f:
        index=0
        while True:
            data=f.read(1_800_000_000)
            if not data:break
            p=a.out/(name+f'.part-{index:03d}');p.write_bytes(data)
            manifest['parts'].append({'name':p.name,'sha256':sha(p),'size':len(data)});index+=1
    (a.out/'download-manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
    (a.out/'PARTS.sha256').write_text(''.join(p['sha256']+'  '+p['name']+'\n' for p in manifest['parts']))
    (a.out/'ISO.sha256').write_text(manifest['sha256']+'  '+name+'\n')
    root=pathlib.Path(__file__).resolve().parents[1]
    shutil.copy2(root/'scripts/Join-ISO.ps1',a.out/'Join-ISO.ps1')
    shutil.copy2(root/'scripts/join-iso.sh',a.out/'join-iso.sh')
    (a.out/'DOWNLOAD-INSTRUCTIONS.txt').write_text('Download every .part-* file, download-manifest.json and Join-ISO.ps1 into one folder.\nRun Windows PowerShell: powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\\Join-ISO.ps1\nThe script checks every part and the reconstructed ISO before reporting success.\nSelect the reconstructed .iso in Rufus; choose DD mode. Writing the USB erases that USB.\nThe split is solely to fit GitHub release-asset limits; no compilation is needed.\n')
if __name__=='__main__':main()
