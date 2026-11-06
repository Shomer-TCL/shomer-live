#!/usr/bin/env python3
"""Boot the real ISO with no NIC, verify offline tests, then reboot retained storage."""
import argparse,json,os,pathlib,shutil,subprocess,time
TEST_LABEL='SHOMER_TEST'

def accelerator():
    if os.access('/dev/kvm',os.R_OK|os.W_OK):
        p=subprocess.Popen(['qemu-system-x86_64','-machine','q35,accel=kvm','-cpu','host','-m','128','-nodefaults','-display','none','-S'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        try:
            p.wait(timeout=1)
        except subprocess.TimeoutExpired:
            p.terminate();p.wait();return 'kvm'
    return 'tcg'

def run_vm(iso,disk,mode,phase,out,accel):
    print(f'Booting actual release ISO: {mode} phase {phase}, {accel}, no NIC',flush=True)
    log=out/f'{mode}-{phase}.serial.log';args=['qemu-system-x86_64','-m','4096','-smp','2','-accel',accel,'-cpu','host' if accel=='kvm' else 'max','-nic','none','-display','none','-no-reboot','-boot','d','-cdrom',str(iso),'-drive',f'file={disk},format=raw,if=virtio','-serial','file:'+str(log),'-fw_cfg','name=opt/shomer/selftest,string=1']
    if mode=='uefi':
        code=pathlib.Path('/usr/share/OVMF/OVMF_CODE_4M.fd');variables=pathlib.Path('/usr/share/OVMF/OVMF_VARS_4M.fd')
        if not code.exists() or not variables.exists():raise RuntimeError('OVMF firmware missing')
        var=out/f'uefi-{phase}.vars.fd';shutil.copy2(variables,var)
        args+=['-drive',f'if=pflash,format=raw,readonly=on,file={code}','-drive',f'if=pflash,format=raw,file={var}']
    with (out/f'{mode}-{phase}.qemu.log').open('w') as stderr:
        p=subprocess.Popen(args,stdout=stderr,stderr=stderr)
        started=time.monotonic()
        try:
            while p.poll() is None:
                data=log.read_text(errors='replace') if log.exists() else ''
                elapsed=time.monotonic()-started
                if 'SHOMER_TEST_FAIL' in data:raise RuntimeError('Offline guest reported a failed check: '+mode)
                if elapsed>180 and not data:raise RuntimeError('No kernel serial output; inspect the ISO boot menu: '+mode)
                if elapsed>300 and 'SHOMER_TEST_STARTED' not in data:raise RuntimeError('Offline self-test service did not start: '+mode)
                if elapsed>1500:raise RuntimeError('ISO boot/offline test timed out: '+mode)
                time.sleep(1)
        except BaseException:
            p.terminate()
            try:p.wait(timeout=10)
            except subprocess.TimeoutExpired:p.kill();p.wait()
            raise
    data=log.read_text(errors='replace') if log.exists() else ''
    expected='SHOMER_OFFLINE_PASS' if phase==1 else 'SHOMER_PERSISTENCE_PASS'
    if p.returncode!=0 or expected not in data or 'SHOMER_TEST_FAIL' in data:raise RuntimeError('ISO validation failed: '+mode+' phase '+str(phase)+'; inspect '+str(log))
    return {'mode':mode,'phase':phase,'result':'passed','network':'no NIC','acceleration':accel,'evidence':log.name}

def main():
    ap=argparse.ArgumentParser();ap.add_argument('iso',type=pathlib.Path);ap.add_argument('--out',type=pathlib.Path,default=pathlib.Path('dist/boot-tests'));a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True)
    # Boot the actual release ISO through its own bootloader. QEMU fw_cfg enables
    # the test service; there is no modified test ISO or bypassed boot chain.
    results=[];accel=accelerator()
    for mode in ['bios','uefi']:
        disk=a.out/(mode+'-backup-disk.raw')
        with disk.open('wb') as f:f.truncate(16*1024**3)
        subprocess.run(['mkfs.ext4','-F','-L',TEST_LABEL,str(disk)],check=True,stdout=subprocess.DEVNULL)
        for phase in [1,2]:results.append(run_vm(a.iso.resolve(),disk.resolve(),mode,phase,a.out.resolve(),accel))
        disk.unlink()
    report={'automated':results,'physical_rufus_usb':'not run: no physical PC/USB attached','physical_phone_flash':'not run: no phone attached','secure_boot':'not validated; BIOS/UEFI without Secure Boot tested'}
    (a.out/'test-report.json').write_text(json.dumps(report,indent=2)+'\n')
if __name__=='__main__':main()
