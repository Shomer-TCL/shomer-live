import importlib.machinery,importlib.util,json,pathlib,subprocess,tempfile,unittest
ROOT=pathlib.Path(__file__).resolve().parents[1]
def load(name,path):
    loader=importlib.machinery.SourceFileLoader(name,str(path));spec=importlib.util.spec_from_loader(name,loader);module=importlib.util.module_from_spec(spec);loader.exec_module(module);return module
brom=load('brom',ROOT/'runtime/shomer-brom-backup')
stage=load('stage',ROOT/'scripts/stage.py')
iso=load('iso',ROOT/'scripts/test-iso.py')

class LiveStorageTests(unittest.TestCase):
    def test_live_labels_survive_actual_ext4_creation(self):
        import re
        config=(ROOT/'scripts/configure-live.sh').read_text()
        persistence=re.search(r'persistence-label=([A-Z_]+)',config).group(1)
        helper=(ROOT/'runtime/shomer-boot-test').read_text()
        self.assertEqual(re.search(r'blkid -L ([A-Z_]+)',helper).group(1),iso.TEST_LABEL)
        for label in [persistence,iso.TEST_LABEL]:
            with tempfile.TemporaryDirectory() as d:
                image=pathlib.Path(d)/'volume.img'
                with image.open('wb') as f:f.truncate(16*1024*1024)
                subprocess.run(['mkfs.ext4','-q','-F','-L',label,str(image)],check=True,capture_output=True)
                actual=subprocess.check_output(['blkid','-p','-s','LABEL','-o','value',str(image)],text=True).strip()
                self.assertEqual(actual,label)

class VmLauncherTests(unittest.TestCase):
    def fake_qemu(self,folder,guest_failure=False):
        import sys
        p=folder/'qemu-system-x86_64'
        p.write_text('#!'+sys.executable+'\n'+'''import json,pathlib,sys
a=sys.argv[1:]
root=pathlib.Path(__file__).parent
with (root/'calls.jsonl').open('a') as f:f.write(json.dumps(a)+'\\n')
serial=pathlib.Path(a[a.index('-serial')+1].removeprefix('file:'))
if a[a.index('-accel')+1]=='kvm':
'''+(" serial.write_text('SHOMER_TEST_STARTED\\nSHOMER_TEST_FAIL\\n')\n" if guest_failure else '')+''' print('Could not access KVM kernel module: Permission denied',file=sys.stderr)
 print('qemu-system-x86_64: -accel kvm: failed to initialize kvm: Permission denied',file=sys.stderr)
 sys.exit(1)
serial.write_text('SHOMER_TEST_STARTED\\nSHOMER_OFFLINE_PASS\\n')
''')
        p.chmod(0o755)
    def test_kvm_host_refusal_repeats_identical_test_in_software(self):
        import os,unittest.mock
        with tempfile.TemporaryDirectory() as d:
            p=pathlib.Path(d);self.fake_qemu(p)
            with unittest.mock.patch.dict(os.environ,{'PATH':str(p)+os.pathsep+os.environ['PATH']}):
                result=iso.run_vm(p/'image.iso',p/'disk.raw','bios',1,p,'kvm')
            calls=[json.loads(line) for line in (p/'calls.jsonl').read_text().splitlines()]
            self.assertEqual([a[a.index('-accel')+1] for a in calls],['kvm','tcg'])
            for a in calls:self.assertEqual(a[a.index('-nic')+1],'none')
            self.assertEqual(result['acceleration'],'tcg')
    def test_guest_failure_is_never_hidden_by_acceleration_fallback(self):
        import os,unittest.mock
        with tempfile.TemporaryDirectory() as d:
            p=pathlib.Path(d);self.fake_qemu(p,guest_failure=True)
            with unittest.mock.patch.dict(os.environ,{'PATH':str(p)+os.pathsep+os.environ['PATH']}):
                with self.assertRaises(RuntimeError):iso.run_vm(p/'image.iso',p/'disk.raw','bios',1,p,'kvm')
            self.assertEqual(len((p/'calls.jsonl').read_text().splitlines()),1)

class GeometryTests(unittest.TestCase):
    def test_requires_geometry_instead_of_claiming_complete_on_zero_exit(self):
        for output in ['','EMMC USER Size: 0x1000','EMMC USER Size: 0x1000\nEMMC Boot1 Size: 0x0\nEMMC Boot2 Size: 0x20']:
            with self.assertRaises(ValueError):brom.geometry(output)
    def test_all_areas_are_recorded(self):
        self.assertEqual(brom.geometry('EMMC Boot1 Size: 0x20\nEMMC Boot2 Size: 0x20\nEMMC USER Size: 0x1000'),{'user':4096,'boot1':32,'boot2':32})
    def test_nonempty_general_purpose_areas_are_not_silently_omitted(self):
        data='EMMC Boot1 Size: 0x20\nEMMC Boot2 Size: 0x20\nEMMC USER Size: 0x1000\nEMMC GP1 Size: 0x800\nEMMC GP2 Size: 0x0'
        self.assertEqual(brom.geometry(data),{'user':4096,'boot1':32,'boot2':32,'gp1':2048})

class ArchiveTests(unittest.TestCase):
    def test_archive_traversal_and_links_are_rejected(self):
        import io,tarfile
        for name,kind in [('../outside',tarfile.REGTYPE),('symlink',tarfile.SYMTYPE)]:
            with tempfile.TemporaryDirectory() as d:
                p=pathlib.Path(d);archive=p/'bad.tar'
                with tarfile.open(archive,'w') as t:
                    m=tarfile.TarInfo(name);m.type=kind;m.size=0;m.linkname='/etc/passwd';t.addfile(m,io.BytesIO())
                with self.assertRaises(ValueError):stage.extract(archive,p/'unpacked')

class StockPackageTests(unittest.TestCase):
    def test_current_image_checksums_cover_joined_files(self):
        with tempfile.TemporaryDirectory() as d:
            p=pathlib.Path(d)
            (p/'super.raw.gz').write_bytes(b'joined-image')
            (p/'SHA256SUMS').write_text('obsolete split-part paths\n')
            stage.normalize_current(p)
            result=subprocess.run(['sha256sum','-c','SHA256SUMS'],cwd=p,capture_output=True,text=True)
            self.assertEqual(result.returncode,0,result.stdout+result.stderr)
            self.assertEqual((p/'Source-provenance/SHA256SUMS.original').read_text(),'obsolete split-part paths\n')
    def test_packaged_checksums_follow_renames_and_exclude_device_artifacts(self):
        for release,expected in [('KEKA','ROOTED-recovery2.img'),('KEE7','STOCK-recovery-KEE7.img')]:
            with tempfile.TemporaryDirectory() as d:
                p=pathlib.Path(d)
                for name in ['recovery','boot','frp','metadata','SHA256SUMS','README.md']:
                    (p/name).write_bytes(('fixture-'+name).encode())
                excluded=stage.normalize_stock(p,release)
                self.assertEqual(excluded,['frp','metadata'])
                self.assertTrue((p/expected).exists());self.assertFalse((p/'recovery').exists())
                self.assertFalse((p/'frp').exists());self.assertFalse((p/'metadata').exists())
                result=subprocess.run(['sha256sum','-c','SHA256SUMS'],cwd=p,capture_output=True,text=True)
                self.assertEqual(result.returncode,0,result.stdout+result.stderr)
                self.assertEqual((p/'Source-provenance/SHA256SUMS.original').read_bytes(),b'fixture-SHA256SUMS')

class ReleaseTests(unittest.TestCase):
    def test_actual_split_and_linux_reassembly_preserve_bytes(self):
        import shutil
        with tempfile.TemporaryDirectory() as d:
            p=pathlib.Path(d);source=p/'input.iso';source.write_bytes(b'actual iso fixture\0'*1000);out=p/'out'
            subprocess.run(['python3',str(ROOT/'scripts/package-release.py'),str(source),'--version','0.1.0-test','--out',str(out)],check=True)
            subprocess.run(['sh',str(out/'join-iso.sh')],check=True)
            m=json.loads((out/'download-manifest.json').read_text());self.assertEqual(source.read_bytes(),(out/m['image']).read_bytes())
    def test_corrupted_part_is_not_accepted(self):
        with tempfile.TemporaryDirectory() as d:
            p=pathlib.Path(d);source=p/'input.iso';source.write_bytes(b'fixture');out=p/'out'
            subprocess.run(['python3',str(ROOT/'scripts/package-release.py'),str(source),'--version','0.1.0-test','--out',str(out)],check=True)
            next(out.glob('*.part-*')).write_bytes(b'corrupted')
            result=subprocess.run(['sh',str(out/'join-iso.sh')],capture_output=True)
            self.assertNotEqual(result.returncode,0);self.assertFalse(any(out.glob('*.iso')))

class InventoryTests(unittest.TestCase):
    def test_firmware_and_tool_pins_are_complete(self):
        a=json.loads((ROOT/'config/assets.lock.json').read_text());self.assertEqual(len(a['git_ref']),40)
        for archive in a['archives']:
            self.assertEqual(sum(p['size'] for p in archive['parts']),archive['size'])
            for p in archive['parts']:self.assertRegex(p['sha256'],r'^[a-f0-9]{64}$')
        for p in json.loads((ROOT/'config/tools.lock.json').read_text())['downloads']:
            self.assertTrue(p['url'].startswith('https://'));self.assertRegex(p['sha256'],r'^[a-f0-9]{64}$')
    def test_brom_sequence_has_no_storage_write_command(self):
        source=(ROOT/'runtime/shomer-brom-backup').read_text()
        self.assertIn("['rf',str(path),*options]",source)
        for command in ["['wf'","['w'","['wl'","['e'","['da','seccfg'"]:
            self.assertNotIn(command,source)

if __name__=='__main__':unittest.main()
