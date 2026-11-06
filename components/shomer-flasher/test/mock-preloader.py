#!/usr/bin/env python3
"""Mock MediaTek preloader on a pseudo-terminal (NOT a phone).
Prints the pty path, then answers the TCL boot sequence like the real preloader:
receives 8 bytes "FASTBOOT", replies "READYTOO" and switches $MOCK_STATE mode to fastboot
(only while mode is "off", like a real phone that is powering up)."""
import json, os, pty, sys, time, fcntl, tty
path = os.environ["MOCK_STATE"]
m, s = pty.openpty()
tty.setraw(s)
name = os.ttyname(s)
open(sys.argv[1], "w").write(name)
buf = b""
while True:
    try:
        b = os.read(m, 64)
    except OSError:
        time.sleep(0.05); continue
    buf += b
    if len(buf) >= 8:
        seq, buf = buf[:8], buf[8:]
        with open(path + ".lock", "w") as lk:
            fcntl.flock(lk, fcntl.LOCK_EX)
            st = json.load(open(path))
            st.setdefault("calls", []).append(["preloader", seq.decode(errors="replace")])
            if seq == b"FASTBOOT" and st["mode"] == "off":
                st["mode"] = "fastboot"
                os.write(m, b"READYTOO")
            json.dump(st, open(path + ".tmp", "w"), indent=1)
            os.replace(path + ".tmp", path)
