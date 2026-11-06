#!/usr/bin/env python3
"""End-to-end test: runs a REAL built shomer-flasher (Linux) against mock fastboot,
mock adb and a mock preloader on a pseudo-terminal. No phone is involved.

  python3 test/e2e.py dist/shomer-flasher-linux-x86_64

Scenarios: locked->unlock->flash, already unlocked, wrong product refused,
bad checksum refused (one byte flipped in the ROM inside the binary, restored after),
and backup-first (recovery2 -> read every partition into a zip -> stock recovery back -> flash).
"""
import hashlib, json, os, re, signal, subprocess, sys, tempfile, time

HERE = os.path.dirname(os.path.abspath(__file__))
BIN = os.path.abspath(sys.argv[1])
results = []
state_dir = [None]

def info():
    out = subprocess.run([BIN, "--info", "--no-pause"], capture_output=True, text=True, timeout=120).stdout
    raw = re.search(r"super: (\d+) bytes, sha256 ([0-9a-f]{64})", out)
    vb = dict(re.findall(r"avb/(vbmeta\w*)\s+\d+\s+([0-9a-f]{64})", out))
    return int(raw.group(1)), raw.group(2), vb

def run(name, state, stdin, extra_env=None, expect_code=0, args=("--no-backup",)):
    d = tempfile.mkdtemp(prefix="e2e-")
    sp = os.path.join(d, "state.json")
    json.dump(state, open(sp, "w"))
    env = dict(os.environ, MOCK_STATE=sp,
               SHOMER_FASTBOOT=os.path.join(HERE, "mock-fastboot.py"),
               SHOMER_ADB=os.path.join(HERE, "mock-adb.py"),
               SHOMER_TEST_FAST="1")
    ptyfile = os.path.join(d, "pty")
    pre = subprocess.Popen([sys.executable, os.path.join(HERE, "mock-preloader.py"), ptyfile], env=env,
                           stdout=open(os.path.join(d, "pre.log"), "w"), stderr=subprocess.STDOUT, start_new_session=True)
    for _ in range(100):
        if os.path.exists(ptyfile) and open(ptyfile).read():
            break
        time.sleep(0.05)
    env["SHOMER_PRELOADER_PORT"] = open(ptyfile).read()
    if extra_env:
        env.update(extra_env)
    t0 = time.time()
    state_dir[0] = d
    p = subprocess.run([BIN, "--no-pause", "--workdir", os.path.join(d, "work"), "--backup-dir", os.path.join(d, "backups")] + list(args), input=stdin, env=env,
                       capture_output=True, text=True, timeout=1800)
    os.killpg(pre.pid, signal.SIGKILL)
    st = json.load(open(sp))
    open(os.path.join(d, "stdout.txt"), "w").write(p.stdout)
    print("=== %s: exit %d in %.1fs (output in %s)" % (name, p.returncode, time.time() - t0, d))
    print("\n".join("    | " + l for l in p.stdout.strip().splitlines()[-6:]))
    ok = p.returncode == expect_code
    return ok, st, p.stdout

def check(name, cond, why):
    results.append((name, bool(cond), why))
    print("   %s %s" % ("PASS" if cond else "FAIL", why))

size, raw_sha, vb = info()
print("payload: super %d bytes sha %s; vbmeta %s" % (size, raw_sha, vb))

def writes(st):
    return [c for c in st["calls"] if c[0] == "fastboot" and any(x in c for x in ("flash", "erase", "flashing"))]

# 1 locked -> unlock -> flash (phone on, USB debugging on: adb reads build, powers off, preloader catch)
ok, st, out = run("locked->unlock->flash", {"mode": "adb", "model": "T408DL", "build": "KEKA", "product": "gflip6", "unlocked": False},
                  "\nUNLOCK\nFLASH\n")
check("locked", ok, "exit code 0")
check("locked", ["preloader", "FASTBOOT"] in st["calls"], "preloader got FASTBOOT and answered READYTOO")
check("locked", ["fastboot", "flashing", "unlock"] in st["calls"], "ran fastboot flashing unlock")
check("locked", st["unlocked"], "phone unlocked")
check("locked", st["flashed"].get("super", {}).get("sha256") == raw_sha, "super written, sha256 matches ROM")
check("locked", all(st["flashed"].get(v, {}).get("sha256") == s and st["flashed"][v]["verity_off"] for v, s in vb.items()), "3 vbmeta written with verity+verification off, stock sha256")
check("locked", set(st["flashed"]) == {"super", "vbmeta", "vbmeta_system", "vbmeta_vendor"} and not st.get("erased"), "nothing else written or erased")
check("locked", st["mode"] == "booting", "rebooted at the end")
check("locked", "WARNING - THE PHONE" not in out and "Flash anyway" not in out, "a normal KEKA phone: no variant warning, no extra question")

# 2 already unlocked, already in fastboot, build typed, erase yes
ok, st, out = run("already-unlocked", {"mode": "fastboot", "product": "gflip6", "unlocked": True}, "KEKA\ny\nFLASH\n")
check("unlocked", ok, "exit code 0")
check("unlocked", ["fastboot", "flashing", "unlock"] not in st["calls"], "no unlock attempted")
check("unlocked", st["flashed"].get("super", {}).get("sha256") == raw_sha, "super written, sha256 matches")
check("unlocked", st.get("erased") == ["userdata"], "userdata erased (person said yes)")
check("unlocked", set(st["flashed"]) == {"super", "vbmeta", "vbmeta_system", "vbmeta_vendor"}, "only allowed partitions")
check("unlocked", "WARNING - THE PHONE" not in out and "Flash anyway" not in out, "typed KEKA on a gflip6: no variant warning")

# 3 wrong product (1.4.3, never block: a warning, then "Flash anyway as KEKA?" default No)
ok, st, out = run("wrong-product", {"mode": "fastboot", "product": "Gflip3_TMO", "unlocked": True}, "KEKA\n\n", expect_code=3)
check("wrong-product", ok, "exit code 3 (Enter = No stops)")
check("wrong-product", not writes(st), "nothing written")
check("wrong-product", "Gflip3_TMO" in out and "WARNING" in out and "Flash anyway as KEKA?" in out, "warning names the product, then the question")
ok, st, out = run("wrong-product-yes", {"mode": "fastboot", "product": "Gflip3_TMO", "unlocked": True}, "KEKA\ny\ny\nFLASH\n")
check("wrong-product-yes", ok and st["flashed"].get("super", {}).get("sha256") == raw_sha, "warning + yes: flashed as KEKA (the person's choice)")
check("wrong-product-yes", "you chose to go on as KEKA after 1 warning" in out, "the end of the run says so")
ok, st, out = run("wrong-product-closed", {"mode": "fastboot", "product": "Gflip3_TMO", "unlocked": True}, "KEKA\n", expect_code=1)
check("wrong-product-closed", ok and not writes(st) and "no more input" in out, "warning + closed input: stops, nothing written")
ok, st, out = run("product-empty-yes", {"mode": "fastboot", "product": "", "unlocked": True}, "KEKA\ny\ny\nFLASH\n")
check("product-empty-yes", ok and "could not detect what this phone is" in out and st["flashed"].get("super", {}).get("sha256") == raw_sha, "no product answer: warning, yes, flashed")
ok, st, out = run("force-variant", {"mode": "fastboot", "product": "Gflip3_TMO", "unlocked": True}, "y\nFLASH\n",
                  args=("--no-backup", "--variant", "upci", "--force-variant"))
check("force-variant", ok and "WARNING" in out and "Flash anyway" not in out and st["flashed"].get("super", {}).get("sha256") == raw_sha, "--variant UPCI --force-variant: warning shown, no question, flashed")

# 3b wrong model over adb: warning, the person's choice, default No
ok, st, out = run("wrong-model", {"mode": "adb", "model": "4058W", "build": "LF91", "product": "gflip6", "unlocked": False}, "UPCI\n\n", expect_code=3)
check("wrong-model", ok and not writes(st) and st["mode"] == "adb" and "4058W" in out and "WARNING" in out, "4058W: warning, No stops before touching the phone")
ok, st, out = run("wrong-model-yes", {"mode": "adb", "model": "4058W", "build": "LF91", "product": "gflip6", "unlocked": False}, "UPCI\ny\n\nUNLOCK\nFLASH\n")
check("wrong-model-yes", ok and st["flashed"].get("super", {}).get("sha256") == raw_sha, "4058W + yes as UPCI: unlocked and flashed")

# 4 bad checksum: flip one byte inside the ROM in the binary, run, restore
before = hashlib.sha256(open(BIN, "rb").read()).hexdigest()
sz = os.path.getsize(BIN)
off = sz - 64 - 1024 - 4096 - 100000  # inside rom/super.raw.gz (last payload file)
with open(BIN, "r+b") as f:
    f.seek(off); b = f.read(1); f.seek(off); f.write(bytes([b[0] ^ 0xFF]))
try:
    ok, st, out = run("bad-sha", {"mode": "fastboot", "product": "gflip6", "unlocked": True}, "KEKA\ny\nFLASH\n", expect_code=3)
finally:
    with open(BIN, "r+b") as f:
        f.seek(off); f.write(b)
after = hashlib.sha256(open(BIN, "rb").read()).hexdigest()
check("bad-sha", ok, "exit code 3 (refused)")
check("bad-sha", "CHECKSUM MISMATCH" in out, "says CHECKSUM MISMATCH")
check("bad-sha", not [c for c in st["calls"] if c[0] in ("fastboot", "preloader")] , "phone never contacted")
check("bad-sha", before == after, "binary restored byte-for-byte")

# 5 backup-first (default mode): locked phone -> unlock -> backup zip -> stock recovery back -> flash
import random, zipfile
pd = tempfile.mkdtemp(prefix="e2e-parts-")
rng = random.Random(7)
names = ["boot", "lk", "lk2", "nvram", "nvdata", "proinfo", "vbmeta", "vbmeta_system", "vbmeta_vendor", "super", "recovery", "userdata", "seccfg",
         "nvcfg", "persist", "protect1", "protect2", "md1img", "logo", "metadata", "mmcblk0boot0"]
for n in names:
    open(os.path.join(pd, n), "wb").write(bytes(rng.getrandbits(8) for _ in range(4096 * (2 + len(n) % 3))))
sums = {n: hashlib.sha256(open(os.path.join(pd, n), "rb").read()).hexdigest() for n in names}
ok, st, out = run("backup-first", {"mode": "adb", "model": "T408DL", "build": "KEKA", "product": "gflip6", "unlocked": False, "partdir": pd},
                  "\nUNLOCK\n\nFLASH\n", args=())
check("backup", ok, "exit code 0")
fl = [c for c in st["calls"] if c[0] == "fastboot" and "flash" in c]
check("backup", [c[2] for c in fl[:2]] == ["recovery", "recovery"] and fl[2][-2] == "vbmeta", "recovery written twice BEFORE vbmeta/super")
check("backup", st["flashed"]["recovery"]["sha256"] == "4fa8dacf08c9fe2131616d377b6c4da5b44bb504df1c67274c37aa7c38b551d3", "stock KEKA recovery is what stays on the phone")
check("backup", set(st["flashed"]) == {"super", "vbmeta", "vbmeta_system", "vbmeta_vendor", "recovery"}, "nothing else written")
zips = [f for f in os.listdir(os.path.join(state_dir[0], "backups")) if f.endswith(".zip")]
check("backup", len(zips) == 1 and zips[0].startswith("backup-MOCK0001-"), "one backup-<serial>-<date>.zip")
if zips:
    z = zipfile.ZipFile(os.path.join(state_dir[0], "backups", zips[0]))
    good = all(hashlib.sha256(z.read(n + ".img")).hexdigest() == s for n, s in sums.items() if n != "userdata")
    check("backup", good, "every partition in the zip, byte-identical")
    check("backup", "userdata.img" not in z.namelist(), "no userdata (unlock had wiped it)")
    check("backup", z.testzip() is None and "SHA256SUMS" in z.namelist() and "backup-info.json" in z.namelist(), "zip reads clean, has SHA256SUMS + backup-info.json")
check("backup", st["flashed"].get("super", {}).get("sha256") == raw_sha, "super written after the backup")

# 6 backup fails (nvram cannot be read), the person answers No (default): nothing flashed, stock recovery back, INCOMPLETE zip kept
ok, st, out = run("backup-fails-no", {"mode": "fastboot", "product": "gflip6", "unlocked": True, "partdir": pd, "corrupt": ["nvram"]},
                  "KEKA\n\n\n", args=(), expect_code=3)
check("backup-fails-no", ok, "exit code 3 (stopped)")
check("backup-fails-no", "Backup failed: continue flashing WITHOUT a backup? [y/N]" in out, "asked whether to flash without a backup")
fl = [c for c in st["calls"] if c[0] == "fastboot" and "flash" in c]
check("backup-fails-no", [c[2] for c in fl] == ["recovery", "recovery"] and st["flashed"]["recovery"]["sha256"].startswith("4fa8dacf"), "only recovery2 then stock recovery written; stock stays")
zips = os.listdir(os.path.join(state_dir[0], "backups"))
check("backup-fails-no", len(zips) == 1 and zips[0].endswith("-INCOMPLETE.zip"), "partial backup kept, named -INCOMPLETE.zip")
if zips:
    info = json.loads(zipfile.ZipFile(os.path.join(state_dir[0], "backups", zips[0])).read("backup-info.json"))
    check("backup-fails-no", info["complete"] is False and info["missing_critical"] == ["nvram"], "backup-info.json says incomplete, names nvram")

# 7 same failure, unattended: --continue-without-backup flashes without asking, stock recovery first
ok, st, out = run("backup-fails-continue", {"mode": "fastboot", "product": "gflip6", "unlocked": True, "partdir": pd, "corrupt": ["nvram"]},
                  "KEKA\n\nn\nFLASH\n", args=("--continue-without-backup",))
check("backup-fails-continue", ok, "exit code 0")
check("backup-fails-continue", "continue flashing WITHOUT a backup?" not in out, "no question with --continue-without-backup")
fl = [c[2] if c[1] == "flash" else c[-2] for c in st["calls"] if c[0] == "fastboot" and "flash" in c]
check("backup-fails-continue", fl[:3] == ["recovery", "recovery", "vbmeta"] and st["flashed"]["recovery"]["sha256"].startswith("4fa8dacf"), "stock recovery back BEFORE vbmeta/super")
check("backup-fails-continue", st["flashed"].get("super", {}).get("sha256") == raw_sha, "super written")

# ---------- 1.4.3 ----------
import shutil

def zip_info(name):
    zs = os.listdir(os.path.join(state_dir[0], "backups"))
    if len(zs) != 1:
        return zs, None
    return zs, json.loads(zipfile.ZipFile(os.path.join(state_dir[0], "backups", zs[0])).read("backup-info.json"))

# 8 (bug 1) phone data asked for, its copy fails: INCOMPLETE, and erasing it needs an explicit yes (Enter = No = stop)
ok, st, out = run("userdata-fails", {"mode": "fastboot", "product": "gflip6", "unlocked": True, "partdir": pd, "corrupt": ["userdata"]},
                  "KEKA\n\ny\n\n", args=("--backup-userdata",), expect_code=3)
check("userdata-fails", ok, "exit code 3 (stopped)")
check("userdata-fails", "Erase the phone data anyway? (n = stop here" in out, "asked explicitly before erasing unsaved phone data")
check("userdata-fails", not st.get("erased") and "super" not in st.get("flashed", {}), "nothing erased, Shomer not written")
zs, inf = zip_info("userdata-fails")
check("userdata-fails", inf is not None and zs[0].endswith("-INCOMPLETE.zip") and not inf["complete"] and "userdata" in inf["missing_critical"], "backup INCOMPLETE, names userdata")

# 9 (bug 1) same with --continue-without-backup: still asked; input closed = never erased
ok, st, out = run("userdata-fails-unattended", {"mode": "fastboot", "product": "gflip6", "unlocked": True, "partdir": pd, "corrupt": ["userdata"]},
                  "KEKA\n\n", args=("--backup-userdata", "--continue-without-backup"), expect_code=1)
check("userdata-fails-unattended", ok and "Erase the phone data anyway?" in out and not st.get("erased") and "super" not in st.get("flashed", {}), "unattended run never erases unsaved phone data")

# 10 (bug 2) the phone's list leaves out the IMEI partitions: INCOMPLETE, asked, No = nothing flashed
ok, st, out = run("imei-not-listed", {"mode": "fastboot", "product": "gflip6", "unlocked": True, "partdir": pd, "drop": ["nvram", "proinfo", "nvdata"]},
                  "KEKA\n\n\n", args=(), expect_code=3)
zs, inf = zip_info("imei-not-listed")
check("imei-not-listed", ok and "super" not in st.get("flashed", {}), "exit 3, nothing flashed")
check("imei-not-listed", inf is not None and not inf["complete"] and set(["nvram", "proinfo", "nvdata"]) <= set(inf["missing_critical"]), "INCOMPLETE, names nvram/proinfo/nvdata")

# 11 (bug 3) the boot area cannot be read: INCOMPLETE
ok, st, out = run("bootarea-fails", {"mode": "fastboot", "product": "gflip6", "unlocked": True, "partdir": pd, "corrupt": ["mmcblk0boot0"]},
                  "KEKA\n\n\n", args=(), expect_code=3)
zs, inf = zip_info("bootarea-fails")
check("bootarea-fails", ok and inf is not None and "mmcblk0boot0" in inf["missing_critical"], "boot area failure makes the backup INCOMPLETE")

# 12 (bug 4) --workdir with the person's own tools folder: nothing of theirs deleted (dry run)
wdir = tempfile.mkdtemp(prefix="e2e-wd-")
os.makedirs(os.path.join(wdir, "tools", "myproject"))
open(os.path.join(wdir, "tools", "my-notes.txt"), "w").write("mine")
open(os.path.join(wdir, "tools", "myproject", "a.c"), "w").write("int x;")
p = subprocess.run([BIN, "--dry-run", "--no-pause", "--workdir", wdir], capture_output=True, text=True, timeout=600)
check("workdir", p.returncode == 0, "dry run exit 0")
check("workdir", os.path.exists(os.path.join(wdir, "tools", "my-notes.txt")) and os.path.exists(os.path.join(wdir, "tools", "myproject", "a.c")), "the person's tools folder is untouched")
check("workdir", not os.path.exists(os.path.join(wdir, "shomer-flasher-work")), "the flasher's own subfolder is gone afterwards")

# 13 (bug 5) a second copy refuses to start while the first one waits at a question
d2 = tempfile.mkdtemp(prefix="e2e-lock-")
sp2 = os.path.join(d2, "state.json"); json.dump({"mode": "off", "product": "gflip6", "unlocked": False}, open(sp2, "w"))
env2 = dict(os.environ, MOCK_STATE=sp2, SHOMER_FASTBOOT=os.path.join(HERE, "mock-fastboot.py"), SHOMER_ADB=os.path.join(HERE, "mock-adb.py"), SHOMER_TEST_FAST="1")
c1 = subprocess.Popen([BIN, "--cli", "--no-pause", "--no-backup", "--workdir", os.path.join(d2, "w1")], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                      stderr=subprocess.STDOUT, env=env2)
seen = ""
t0 = time.time()
while "Type the 4 letters" not in seen and time.time() - t0 < 300:
    chunk = os.read(c1.stdout.fileno(), 4096)  # the question has no newline: read what is there
    if not chunk:
        break
    seen += chunk.decode("utf-8", "replace")
c2 = subprocess.run([BIN, "--dry-run", "--no-pause", "--workdir", os.path.join(d2, "w2")], capture_output=True, text=True, timeout=120, env=env2)
check("two-copies", "Type the 4 letters" in seen, "copy 1 is waiting at the software-version question")
check("two-copies", c2.returncode == 2 and "already running" in c2.stdout, "copy 2 refused: another copy is already running")
check("two-copies", os.path.exists(os.path.join(d2, "w1", "shomer-flasher-work", "super.raw")), "copy 1's unpacked ROM is still there")
c1.stdin.write(b"q\n"); c1.stdin.flush()
rest, _ = c1.communicate(timeout=120)
check("two-copies", c1.returncode == 3, "copy 1 stops normally when told (q)")
p = subprocess.run([BIN, "--dry-run", "--no-pause", "--workdir", os.path.join(d2, "w3")], capture_output=True, text=True, timeout=600)
check("two-copies", p.returncode == 0, "after copy 1 ended, a new copy starts")

# 14 (variant) the phone's own software, read out of super during the backup, is another variant: refused before Shomer is written
pdv = tempfile.mkdtemp(prefix="e2e-parts-v-")
for n in names:
    shutil.copy(os.path.join(pd, n), os.path.join(pdv, n))
sup = bytearray(open(os.path.join(pdv, "super"), "rb").read())
props = b"\x00ro.build.version.incremental=LF91\nro.build.fingerprint=TCL/4058W/Gflip6_TMO:11/RP1A.200720.011/LF91:user/release-keys\n\x00"
sup[100:100 + len(props)] = props
open(os.path.join(pdv, "super"), "wb").write(bytes(sup))
ok, st, out = run("variant-foreign", {"mode": "fastboot", "product": "gflip6", "unlocked": True, "partdir": pdv},
                  "KEKA\n\n\n", args=(), expect_code=3)
check("variant-foreign", ok and "not T408DL KEKA" in out and "Flash anyway as KEKA?" in out, "warning: the phone's own files say another variant; Enter = No stops")
check("variant-foreign", "super" not in st.get("flashed", {}) and st["flashed"]["recovery"]["sha256"].startswith("4fa8dacf"), "Shomer not written; stock recovery back")
ok, st, out = run("variant-foreign-yes", {"mode": "fastboot", "product": "gflip6", "unlocked": True, "partdir": pdv},
                  "KEKA\n\ny\nn\nFLASH\n", args=())
check("variant-foreign-yes", ok and st["flashed"].get("super", {}).get("sha256") == raw_sha, "warning + yes: backup made, then flashed")

# 15 (variant) exact product: "gflip65" is another phone
ok, st, out = run("product-gflip65", {"mode": "fastboot", "product": "gflip65", "unlocked": True}, "KEKA\nn\n", expect_code=3)
check("product-gflip65", ok and not writes(st) and "gflip65" in out and "WARNING" in out, "gflip65: warning, No stops, nothing written")

# 16 (bug 10) adb error while reading the model is never shown as the model
ok, st, out = run("adb-error", {"mode": "adb", "model": "T408DL", "build": "KEKA", "product": "gflip6", "unlocked": False, "adb_error": "error: closed"}, "\n", expect_code=3)
check("adb-error", ok and "lost the connection to the phone" in out and 'this phone is a "error' not in out, "says the connection was lost, not a wrong phone")

fails = [r for r in results if not r[1]]
print("\n%d checks, %d failed" % (len(results), len(fails)))
sys.exit(1 if fails else 0)
