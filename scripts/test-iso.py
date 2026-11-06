#!/usr/bin/env python3
"""Boot the real ISO with no NIC, verify offline tests, then reboot retained storage."""
import argparse,json,pathlib,shutil,subprocess,time

def run_vm(iso,disk,mode,phase,out):
    log=out/f'{mode}-{phase}.serial.log';args=['qemu-system-x86_64','-m','4096','-smp','2','-accel','tcg','-nic','none','-display','none','-no-reboot','-boot','d','-cdrom',str(iso),'-drive',f'file={disk},format=raw,if=virtio','-serial','file:'+str(log),'-fw_cfg','name=opt/shomer/selftest,string=1']
    if mode=='uefi':
        code=pathlib.Path('/usr/share/OVMF/OVMF_CODE_4M.fd');variables=pathlib.Path('/usr/share/OVMF/OVMF_VARS_4M.fd')
        if not code.exists() or not variables.exists():raise RuntimeError('OVMF firmware missing')
        var=out/f'uefi-{phase}.vars.fd';shutil.copy2(variables,var)
        args+=['-drive',f'if=pflash,format=raw,readonly=on,file={code}','-drive',f'if=pflash,format=raw,file={var}']
    with (out/f'{mode}-{phase}.qemu.log').open('w') as stderr:
        p=subprocess.Popen(args,stdout=stderr,stderr=stderr)
        try:
            p.wait(timeout=1500)
        except subprocess.TimeoutExpired:p.kill();p.wait();raise RuntimeError('ISO boot/offline test timed out: '+mode)
    data=log.read_text(errors='replace') if log.exists() else ''
    expected='SHOMER_OFFLINE_PASS' if phase==1 else 'SHOMER_PERSISTENCE_PASS'
    if p.returncode!=0 or expected not in data or 'SHOMER_TEST_FAIL' in data:raise RuntimeError('ISO validation failed: '+mode+' phase '+str(phase)+'; inspect '+str(log))
    return {'mode':mode,'phase':phase,'result':'passed','network':'no NIC','evidence':log.name}

def main():
    ap=argparse.ArgumentParser();ap.add_argument('iso',type=pathlib.Path);ap.add_argument('--out',type=pathlib.Path,default=pathlib.Path('dist/boot-tests'));a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True)
    # Boot the actual release ISO through its own bootloader. QEMU fw_cfg enables
    # the test service; there is no modified test ISO or bypassed boot chain.
    results=[]
    for mode in ['bios','uefi']:
        disk=a.out/(mode+'-backup-disk.raw')
        with disk.open('wb') as f:f.truncate(16*1024**3)
        subprocess.run(['mkfs.ext4','-F','-L','SHOMER_TEST_BACKUPS',str(disk)],check=True,stdout=subprocess.DEVNULL)
        for phase in [1,2]:results.append(run_vm(a.iso.resolve(),disk.resolve(),mode,phase,a.out.resolve()))
        disk.unlink()
    report={'automated':results,'physical_rufus_usb':'not run: no physical PC/USB attached','physical_phone_flash':'not run: no phone attached','secure_boot':'not validated; BIOS/UEFI without Secure Boot tested'}
    (a.out/'test-report.json').write_text(json.dumps(report,indent=2)+'\n')
if __name__=='__main__':main()
