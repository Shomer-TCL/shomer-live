#!/usr/bin/env python3
"""Drive the native GTK window with real X11 input and mock phone executables.
Requires Xvfb + openbox (window manager), xdotool and ImageMagick import.
"""
import hashlib,json,os,pathlib,subprocess,sys,tempfile,time
HERE=pathlib.Path(__file__).resolve().parent
BIN=str(pathlib.Path(sys.argv[1]).resolve())
CAP=pathlib.Path(sys.argv[2]);CAP.mkdir(parents=True,exist_ok=True)
def xdo(*args):
    return subprocess.run(['xdotool',*map(str,args)],capture_output=True,text=True)
def find(title,timeout=30):
    end=time.time()+timeout
    while time.time()<end:
        p=xdo('search','--onlyvisible','--name',title)
        if p.returncode==0 and p.stdout.strip():return p.stdout.strip().splitlines()[-1]
        time.sleep(.15)
    raise AssertionError('Window did not appear: '+title)
def press(window,key):
    p=xdo('windowactivate','--sync',window)
    if p.returncode:raise AssertionError(p.stderr)
    p=xdo('key','--clearmodifiers',key)
    if p.returncode:raise AssertionError(p.stderr)
    time.sleep(.2)
def scenario(accept):
    with tempfile.TemporaryDirectory(prefix='gtk-e2e-') as tmp:
        d=pathlib.Path(tmp);parts=d/'parts';parts.mkdir()
        raw=b'ro.build.fingerprint=TCL/T408DL/Gflip6_TF:11/RP1A.200720.011/KEKA:user/release-keys\n'+b'\0'*(6<<20)
        for name in ['boot','super','vbmeta','vbmeta_system','vbmeta_vendor','proinfo','nvram','nvdata','nvcfg','persist','protect1','protect2','seccfg','md1img','lk','logo','metadata','userdata','mmcblk0boot0']:
            (parts/name).write_bytes(raw if name=='super' else (name.encode()+b'\0')*100)
        state=d/'state.json';state.write_text(json.dumps({'mode':'fastboot','product':'gflip6','unlocked':True,'partdir':str(parts)}))
        env=dict(os.environ,MOCK_STATE=str(state),SHOMER_FASTBOOT=str(HERE/'mock-fastboot.py'),SHOMER_ADB=str(HERE/'mock-adb.py'),SHOMER_TEST_FAST='1')
        with (d/'gui.log').open('w') as log:
            p=subprocess.Popen([BIN,'--gui','--variant','UPCI','--workdir',str(d/'work'),'--backup-dir',str(d/'backups')],env=env,stdout=log,stderr=subprocess.STDOUT)
            try:
                win=find('Shomer Flip 2 Workbench.*Check images');time.sleep(1)
                press(win,'Return');win=find('Shomer Flip 2 Workbench.*Identify phone');time.sleep(.5)
                press(win,'Return');win=find('Shomer Flip 2 Workbench.*Back up')
                press(win,'Return');win=find('Shomer Flip 2 Workbench.*Review')
                press(win,'Return');dialog=find('Shomer.*confirm action')
                if not accept:
                    press(dialog,'Return') # Default No must perform no write.
                    win=find('Shomer Flip 2 Workbench.*Review')
                    st=json.loads(state.read_text());assert not st.get('flashed'),st
                    press(win,'alt+F4');p.wait(timeout=15);assert p.returncode==3,p.returncode
                    print('PASS final confirmation defaults to No; no partitions written')
                else:
                    press(dialog,'alt+y');win=find('Shomer Flip 2 Workbench.*Results',timeout=90)
                    subprocess.run(['import','-window',win,str(CAP/'gtk-real-mock-result.png')],check=True)
                    st=json.loads(state.read_text());assert set(st['flashed'])=={'recovery','vbmeta','vbmeta_system','vbmeta_vendor','super'},st
                    assert st['flashed']['recovery']['sha256']=='4fa8dacf08c9fe2131616d377b6c4da5b44bb504df1c67274c37aa7c38b551d3',st
                    backups=list((d/'backups').glob('backup-*.zip'));assert len(backups)==1,backups
                    calls=st['calls'];superwrite=next(i for i,c in enumerate(calls) if c[:3]==['fastboot','flash','super'])
                    assert ['fastboot','reboot','fastboot'] in calls[:superwrite]
                    press(win,'Return');p.wait(timeout=15);assert p.returncode==0,p.returncode
                    print('PASS native GUI completed mock backup, stock restore and mode-verified flash')
            except BaseException:
                print((d/'gui.log').read_text(errors='replace'))
                print(state.read_text())
                raise
            finally:
                if p.poll() is None:p.kill();p.wait()
scenario(False)
scenario(True)
