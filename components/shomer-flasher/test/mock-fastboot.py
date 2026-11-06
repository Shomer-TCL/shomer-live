#!/usr/bin/env python3
"""Mock fastboot for end-to-end tests of shomer-flasher (NOT a real fastboot).

State lives in the JSON file named by $MOCK_STATE:
  mode: off|fastboot|fastbootd|booting, product, unlocked (bool),
  unlock_refuse (bool), calls (list), flashed {part: {sha256, size, verity}}, erased []
Every invocation is appended to calls. `flash` hashes the file it is given.
"""
import hashlib, json, os, sys, fcntl

path = os.environ["MOCK_STATE"]
args = sys.argv[1:]
with open(path + ".lock", "w") as lk:
    fcntl.flock(lk, fcntl.LOCK_EX)
    st = json.load(open(path))
    st.setdefault("calls", []).append(["fastboot"] + args)
    out, err, code = "", "", 0
    infb = st["mode"] in ("fastboot", "fastbootd")
    verity = False
    if args[:2] == ["--disable-verity", "--disable-verification"]:
        verity = True
        args = args[2:]
    if args == ["devices"]:
        if infb:
            out = "MOCK0001\t%s\n" % st["mode"]
    elif not infb:
        err, code = "< waiting for any device >\n", 1
    elif args[:2] == ["getvar", "product"]:
        err = "product: %s\nFinished. Total time: 0.001s\n" % st["product"]
    elif args[:2] == ["getvar", "unlocked"]:
        err = "unlocked: %s\nFinished. Total time: 0.001s\n" % ("yes" if st["unlocked"] else "no")
    elif args[:2] == ["getvar", "all"]:
        err = "(bootloader) max-download-size: 0x8000000\n(bootloader) product: %s\nall:\nFinished.\n" % st["product"]
    elif args[:2] == ["getvar", "is-userspace"]:
        err = "is-userspace: %s\n" % ("yes" if st["mode"] == "fastbootd" else "no")
    elif args[:2] == ["getvar", "partition-size:recovery"]:
        err = "partition-size:recovery: 0x1800000\n"
    elif args == ["reboot", "bootloader"]:
        st["mode"] = "fastboot"
        err = "Rebooting into bootloader OKAY\n"
    elif args == ["flashing", "unlock"]:
        if st.get("unlock_refuse"):
            err, code = "FAILED (remote: 'Unlock operation is not allowed')\nfastboot: error: Command failed\n", 1
        else:
            st["unlocked"] = True
            err = "                                                   OKAY [  6.120s]\nFinished. Total time: 6.121s\n"
    elif args[:1] == ["flash"] and len(args) == 3:
        part, f = args[1], args[2]
        h = hashlib.sha256(); n = 0
        with open(f, "rb") as fh:
            for b in iter(lambda: fh.read(1 << 22), b""):
                h.update(b); n += len(b)
        st.setdefault("flashed", {})[part] = {"sha256": h.hexdigest(), "size": n, "verity_off": verity, "mode": st["mode"]}
        if n > 128 << 20:
            err = "Invalid sparse file format at header magic\nSending sparse '%s' 1/%d (131068 KB) OKAY\nWriting '%s' OKAY\nFinished.\n" % (part, n // (128 << 20) + 1, part)
        else:
            err = "Sending '%s' (%d KB) OKAY\nWriting '%s' OKAY\nFinished.\n" % (part, n // 1024, part)
    elif args[:1] == ["erase"]:
        st.setdefault("erased", []).append(args[1])
        err = "Erasing '%s' OKAY\n" % args[1]
    elif args == ["reboot", "recovery"]:
        # only the pinned backup recovery (recovery2.img) gives the root adb the flasher needs
        dump = "394ad6bb38321565b121dec8c5dff098cd238919b766f9cbc0c994f4f0a376ac"
        st["mode"] = "recovery" if st.get("flashed", {}).get("recovery", {}).get("sha256") == dump else "stockrecovery"
        err = "Rebooting into recovery OKAY\n"
    elif args == ["reboot", "fastboot"]:
        st["mode"] = "fastbootd"; err = "Rebooting into fastboot OKAY\n"
    elif args == ["reboot"]:
        st["mode"] = "booting"; err = "Rebooting OKAY\nFinished.\n"
    elif args[:2] == ["oem", "device-info"]:
        err = "(bootloader) Device unlocked: %s\nOKAY\n" % ("true" if st["unlocked"] else "false")
    else:
        err, code = "FAILED (remote: 'unknown command')\nfastboot: error: Command failed\n", 1
    json.dump(st, open(path + ".tmp", "w"), indent=1)
    os.replace(path + ".tmp", path)
sys.stdout.write(out); sys.stderr.write(err)
sys.exit(code)
