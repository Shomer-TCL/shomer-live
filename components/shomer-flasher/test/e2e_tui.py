#!/usr/bin/env python3
"""End-to-end test of the Linux TUI: runs a REAL built shomer-flasher in a pseudo-terminal
(no --cli, no --no-pause), presses keys like a person, and reads the screen with the pyte
terminal emulator. Mock fastboot/adb/preloader; no phone is involved.

  pip install pyte==0.8.2
  python3 test/e2e_tui.py <flasher> [capture dir]

With a capture dir, each screen is saved as text (the README's TUI captures)."""
import fcntl, glob, hashlib, json, os, pty, random, select, signal, struct, subprocess, sys, tempfile, termios, time
import pyte

HERE = os.path.dirname(os.path.abspath(__file__))
BIN = os.path.abspath(sys.argv[1])
CAP = sys.argv[2] if len(sys.argv) > 2 else None
COLS, ROWS = 100, 34
results = []

def check(name, cond, why):
    results.append((name, bool(cond), why))
    print("   %s %s" % ("PASS" if cond else "FAIL", why))

class Session:
    last = None

    def __init__(self, name, state, args=()):
        # one flasher at a time (1.4.3 refuses a second copy): end a session a failed check left running
        if Session.last is not None:
            try:
                os.kill(Session.last.pid, signal.SIGKILL)
                os.waitpid(Session.last.pid, 0)
            except (OSError, ChildProcessError):
                pass
        Session.last = self
        self.name = name
        self.d = tempfile.mkdtemp(prefix="tui-")
        self.sp = os.path.join(self.d, "state.json")
        json.dump(state, open(self.sp, "w"))
        env = dict(os.environ, MOCK_STATE=self.sp, SHOMER_FASTBOOT=os.path.join(HERE, "mock-fastboot.py"),
                   SHOMER_ADB=os.path.join(HERE, "mock-adb.py"), SHOMER_TEST_FAST="1", TERM="xterm-256color", COLORTERM="truecolor")
        ptyfile = os.path.join(self.d, "pty")
        self.pre = subprocess.Popen([sys.executable, os.path.join(HERE, "mock-preloader.py"), ptyfile], env=env,
                                    stdout=open(os.path.join(self.d, "pre.log"), "w"), stderr=subprocess.STDOUT, start_new_session=True)
        for _ in range(100):
            if os.path.exists(ptyfile) and open(ptyfile).read():
                break
            time.sleep(0.05)
        env["SHOMER_PRELOADER_PORT"] = open(ptyfile).read()
        self.backups = os.path.join(self.d, "backups")
        pid, fd = pty.fork()
        if pid == 0:
            os.execve(BIN, [BIN, "--tui", "--workdir", os.path.join(self.d, "work"), "--backup-dir", self.backups] + list(args), env)
        self.pid, self.fd = pid, fd
        fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))
        os.kill(pid, signal.SIGWINCH)
        self.screen = pyte.Screen(COLS, ROWS)
        self.stream = pyte.ByteStream(self.screen)
        self.raw = b""
        self.code = None

    def pump(self, t=0.05):
        r, _, _ = select.select([self.fd], [], [], t)
        if r:
            try:
                b = os.read(self.fd, 65536)
            except OSError:
                b = b""
            if not b:
                return False
            self.raw += b
            self.stream.feed(b)
            return True
        return False

    def text(self):
        return "\n".join(l.rstrip() for l in self.screen.display)

    def wait(self, s, timeout=30):
        end = time.time() + timeout
        while time.time() < end:
            self.pump()
            if s in self.text():
                time.sleep(0.15)
                while self.pump(0.05):
                    pass
                return True
            if self.exited():
                break
        print("---- screen while waiting for %r ----\n%s\n----" % (s, self.text()))
        return False

    def key(self, k, n=1):
        for _ in range(n):
            os.write(self.fd, k.encode())
            time.sleep(0.12)
            while self.pump(0.05):
                pass

    def save(self, name):
        if CAP:
            os.makedirs(CAP, exist_ok=True)
            open(os.path.join(CAP, "tui-%s.txt" % name), "w").write(self.text().rstrip() + "\n")

    def exited(self, timeout=0):
        if self.code is not None:
            return True
        end = time.time() + timeout
        while True:
            p, st = os.waitpid(self.pid, os.WNOHANG)
            if p:
                self.code = os.waitstatus_to_exitcode(st)
                return True
            if time.time() >= end:
                return False
            self.pump(0.1)

    def finish(self):
        self.exited(20)
        while self.pump(0.05):
            pass
        os.killpg(self.pre.pid, signal.SIGKILL)
        return json.load(open(self.sp))

DOWN, ENTER = "\x1b[B", "\r"

def parts():
    pd = tempfile.mkdtemp(prefix="tui-parts-")
    rng = random.Random(7)
    for n in ["boot", "lk", "lk2", "nvram", "nvdata", "proinfo", "vbmeta", "vbmeta_system", "vbmeta_vendor", "super", "recovery", "userdata", "seccfg",
              "nvcfg", "persist", "protect1", "protect2", "md1img", "logo", "metadata", "mmcblk0boot0"]:
        open(os.path.join(pd, n), "wb").write(bytes(rng.getrandbits(8) for _ in range(4096 * (2 + len(n) % 3))))
    return pd

STOCK = "4fa8dacf08c9fe2131616d377b6c4da5b44bb504df1c67274c37aa7c38b551d3"
pd = parts()

# 1 locked phone, backup on (default): checks -> phone -> options -> confirm -> writing -> done
print("=== tui: locked phone, backup on")
s = Session("locked", {"mode": "adb", "model": "T408DL", "build": "KEKA", "product": "gflip6", "unlocked": False, "partdir": pd})
check("tui", s.wait("Continue: find the phone"), "checks screen, ROM checked, Continue offered")
check("tui", "ROM inside this program" in s.text() and "Free disk for the ROM" in s.text(), "checks list ROM and disk")
s.save("1-checks")
s.key(ENTER)
check("tui", s.wait("Done - press Enter"), "phone screen asks to power off + unplug (instructions in a box)")
check("tui", "UNPLUG" in s.text(), "instructions shown with the question")
s.save("2-phone-catch")
s.key(ENTER)
check("tui", s.wait("Phone found"), "phone found screen")
check("tui", "MOCK0001" in s.text() and "LOCKED" in s.text() and "T408DL" in s.text(), "shows model, serial and LOCKED")
s.save("3-phone-found")
s.key(ENTER)
check("tui", s.wait("Backup the phone first"), "options screen")
check("tui", "[x] ON" in s.text(), "backup ON by default")
s.save("4-options")
s.key(DOWN, 4)
s.key(ENTER)
check("tui", s.wait("FLASH NOW"), "confirm screen")
t = s.text()
check("tui", "fastboot flashing unlock" in t and "Backup 1/3" in t and "ERASES" in t, "confirm lists unlock + backup steps")
s.save("5-confirm")
s.key(ENTER)
check("tui", s.wait("Unlock the bootloader (this ERASES"), "final yes/no, default No")
s.key("y")
check("tui", s.wait("Finished", 60) and s.wait("DONE - Shomer OS has been written", 60), "done screen")
check("tui", "backup-MOCK0001-" in s.text(), "done screen shows the backup zip path")
s.save("7-done")
s.key(ENTER)
st = s.finish()
check("tui", s.code == 0, "exit code 0 (got %s)" % s.code)
fl = [c for c in st["calls"] if c[0] == "fastboot" and "flash" in c]
check("tui", ["fastboot", "flashing", "unlock"] in st["calls"], "unlocked (no typed UNLOCK needed: confirmed on screen)")
check("tui", [c[2] for c in fl[:2]] == ["recovery", "recovery"] and st["flashed"]["recovery"]["sha256"] == STOCK, "recovery2 then stock recovery, stock stays")
check("tui", set(st["flashed"]) == {"super", "vbmeta", "vbmeta_system", "vbmeta_vendor", "recovery"}, "nothing else written")
zips = os.listdir(s.backups) if os.path.isdir(s.backups) else []
check("tui", len(zips) == 1 and zips[0].endswith(".zip") and "INCOMPLETE" not in zips[0], "one complete backup zip")
check("tui", b"backup-MOCK0001-" in s.raw.split(b"\x1b[?1049l")[-1], "result left in the terminal after closing")

# 2 unlocked phone in fastboot, backup fails (nvram unreadable) -> the TUI asks, person says yes -> flashed
print("=== tui: backup fails, continue without it")
s = Session("fails", {"mode": "fastboot", "product": "gflip6", "unlocked": True, "partdir": pd, "corrupt": ["nvram"]})
check("tui-fail", s.wait("Continue: find the phone"), "checks")
s.key(ENTER)
check("tui-fail", s.wait("Type the 4 letters"), "asks the software version (phone already in fastboot)")
s.key("KEKA")
s.key(ENTER)
check("tui-fail", s.wait("Phone found") and "UNLOCKED" in s.text(), "phone found, UNLOCKED")
s.key(ENTER)
check("tui-fail", s.wait("Erase phone data after flashing"), "options: erase offered (already unlocked)")
s.key(DOWN, 4)
s.key(ENTER)
check("tui-fail", s.wait("FLASH NOW") and "fastboot flashing unlock" not in s.text(), "confirm: no unlock step")
s.key(ENTER)
s.key("y")
check("tui-fail", s.wait("Backup failed: continue flashing WITHOUT a backup?", 60), "backup failure question shown")
check("tui-fail", "nvram" in s.text(), "says what failed (nvram)")
s.save("6-backup-failed")
s.key("y")
check("tui-fail", s.wait("DONE - Shomer OS has been written", 60), "flashed after yes")
check("tui-fail", "INCOMPLETE" in s.text(), "done screen points to the INCOMPLETE zip")
s.key("q")
st = s.finish()
check("tui-fail", s.code == 0, "exit code 0 (got %s)" % s.code)
fl = [c[2] if c[1] == "flash" else c[-2] for c in st["calls"] if c[0] == "fastboot" and "flash" in c]
check("tui-fail", fl[:3] == ["recovery", "recovery", "vbmeta"] and st["flashed"]["recovery"]["sha256"] == STOCK, "stock recovery back BEFORE vbmeta/super")
check("tui-fail", st.get("erased") == ["userdata"], "userdata erased (option default ON)")

# 4 no phone connected (Sam, Nov 4 2026): the version is typed by hand, in small letters with spaces
print("=== tui: no phone, version typed by hand")
s = Session("nophone", {"mode": "off", "product": "gflip6", "unlocked": True, "partdir": pd})
check("tui-typed", s.wait("Continue: find the phone"), "checks")
s.key(ENTER)
check("tui-typed", s.wait("Type the 4 letters"), "no phone: asks the software version")
s.key(ENTER)  # nothing typed
check("tui-typed", s.wait("Accepted answers: KEKA, UPCI"), "an empty answer is refused and the accepted list is shown")
check("tui-typed", s.wait("Type the 4 letters"), "asked again")
s.key(" upci ")
check("tui-typed", s.wait(" upci _"), "the typed text is shown in the box")
s.save("8-typed-upci")
s.key(ENTER)
check("tui-typed", s.wait("Done - press Enter"), "accepted: goes on to power off + unplug")
check("tui-typed", "is not a supported" not in s.text().split("Accepted answers")[-1], "not refused")
s.save("9-after-upci")
s.key("\x03")  # Ctrl+C -> quit?
s.key("y")
st = s.finish()
check("tui-typed", s.code == 3, "quit before anything was written: exit code 3 (got %s)" % s.code)
check("tui-typed", not [c for c in st["calls"] if c[0] == "fastboot" and ("flash" in c or "flashing" in c)], "nothing written")
log = ""
for f in glob.glob(os.path.join(os.path.dirname(BIN), "shomer-flasher-*.log")):
    log = open(f, encoding="utf-8", errors="replace").read() if "upci" in open(f, encoding="utf-8", errors="replace").read() else log
check("tui-typed", "OK - UPCI." in log, "log: 'OK - UPCI.'")

# 5 never block (Sam, Nov 4 2026): the phone reports another product -> warning + "Flash anyway as KEKA?" (default No)
print("=== tui: unknown product, warning, yes goes on")
s = Session("warn-yes", {"mode": "fastboot", "product": "gflip65", "unlocked": True, "partdir": pd})
check("tui-warn", s.wait("Continue: find the phone"), "checks")
s.key(ENTER)
check("tui-warn", s.wait("Type the 4 letters"), "asks the software version")
s.key("KEKA")
s.key(ENTER)
check("tui-warn", s.wait("Flash anyway as KEKA?"), "warning, then the question with the person's choice")
check("tui-warn", "gflip65" in s.text(), "the warning names what was detected (gflip65)")
s.save("10-variant-warning")
s.key("y")
check("tui-warn", s.wait("Phone found"), "yes: goes on to the phone screen")
s.key("\x03")
s.key("y")
st = s.finish()
check("tui-warn", not [c for c in st["calls"] if c[0] == "fastboot" and ("flash" in c or "flashing" in c)], "nothing written (quit before writing)")

print("=== tui: unknown product, warning, No stops")
s = Session("warn-no", {"mode": "fastboot", "product": "gflip65", "unlocked": True, "partdir": pd})
check("tui-warn-no", s.wait("Continue: find the phone"), "checks")
s.key(ENTER)
check("tui-warn-no", s.wait("Type the 4 letters"), "asks the software version")
s.key("KEKA")
s.key(ENTER)
check("tui-warn-no", s.wait("Flash anyway as KEKA?"), "warning + question")
s.key("n")
check("tui-warn-no", s.wait("stopped by you after the warning"), "No: stopped, says why")
s.key("q")
st = s.finish()
check("tui-warn-no", "Phone found" not in s.text() and not [c for c in st["calls"] if c[0] == "fastboot" and ("flash" in c or "flashing" in c)], "nothing written, no phone screen")

# 3 --cli and a non-terminal stdin still give the plain console
print("=== cli fallbacks")
p = subprocess.run([BIN, "--cli", "--dry-run"], capture_output=True, text=True, timeout=300, input="")
check("cli", p.returncode == 0 and "Dry run" in p.stdout and "\x1b[?1049h" not in p.stdout, "--dry-run: plain console")

fails = [r for r in results if not r[1]]
print("\n%d checks, %d failed" % (len(results), len(fails)))
sys.exit(1 if fails else 0)
