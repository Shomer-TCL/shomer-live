#!/usr/bin/env python3
"""Mock adb for end-to-end tests (NOT a real adb). Uses the same $MOCK_STATE file.
mode "adb" = phone on with USB debugging; model/build from state.
mode "recovery" = the backup recovery (recovery2): su works, partitions are the files in
state["partdir"] (name -> /dev/block/mmcblk0pN), exec-out streams them raw."""
import hashlib, json, os, sys, fcntl
path = os.environ["MOCK_STATE"]
a = sys.argv[1:]
with open(path + ".lock", "w") as lk:
    fcntl.flock(lk, fcntl.LOCK_EX)
    st = json.load(open(path))
    st.setdefault("calls", []).append(["adb"] + a)
    if a[:1] == ["-s"]:
        a = a[2:]
    out = ""
    bout = None
    parts = {}
    if st.get("partdir"):
        for i, n in enumerate(sorted(os.listdir(st["partdir"]))):
            # the eMMC boot area: by-name calls it preloader_a at most, never mmcblk0boot0
            parts["/dev/block/mmcblk0boot0" if n == "mmcblk0boot0" else "/dev/block/mmcblk0p%d" % (i + 1)] = n
    code = 0
    if a == ["devices"]:
        line = {"adb": "MOCK0001\tdevice\n", "recovery": "MOCK0001\trecovery\n"}.get(st["mode"], "")
        out = "List of devices attached\n" + line + "\n"
    elif st["mode"] == "recovery" and len(a) == 2 and a[0] == "shell" and a[1].startswith("su -c "):
        c = a[1]
        if "'id'" in c:
            out = "uid=0(root) gid=0(root)\n"
        elif "blockdev" in c:
            drop = set(st.get("drop", []))
            for d, n in parts.items():
                size = os.path.getsize(os.path.join(st["partdir"], n))
                if n == "mmcblk0boot0":
                    out += "preloader_a %s %d\n" % (d, size)
                    if "[ -b /dev/block/mmcblk0boot0 ]" in c:  # the flasher asks for the boot area itself
                        out += "mmcblk0boot0 %s %d\n" % (d, size)
                elif n not in drop:
                    out += "%s %s %d\n" % (n, d, size)
            out += "mmcblk0rpmb /dev/block/mmcblk0rpmb 4194304\n"
        elif "sha256sum " in c:
            dev = c.split("sha256sum ")[1].strip("'").split()[0]
            out = hashlib.sha256(open(os.path.join(st["partdir"], parts[dev]), "rb").read()).hexdigest() + "  " + dev + "\n"
    elif st["mode"] == "recovery" and len(a) == 2 and a[0] == "exec-out" and "if=" in a[1]:
        dev = a[1].split("if=")[1].split()[0]
        bout = open(os.path.join(st["partdir"], parts[dev]), "rb").read()
        if parts[dev] in st.get("corrupt", []):
            bout = bout[:len(bout) // 2]  # short read: the copy is damaged every time
        st.setdefault("reads", []).append(dev)
    elif st["mode"] == "recovery" and a == ["reboot", "bootloader"]:
        st["mode"] = "fastboot"
    elif a[:2] == ["shell", "getprop"] and st.get("adb_error") and a[2] == "ro.product.model":
        out, code = st["adb_error"] + "\n", 1  # the phone dropped off USB between 'devices' and 'getprop'
    elif a[:2] == ["shell", "getprop"]:
        out = {"ro.product.model": st.get("model", ""), "ro.build.version.incremental": st.get("build", ""),
               "ro.bootmode": "recovery" if st["mode"] == "recovery" else "normal",
               "ro.build.fingerprint": "TCL/%s/Gflip6_TF:11/RP1A.200720.011/%s:user/release-keys" % (st.get("model"), st.get("build"))}.get(a[2], "") + "\n"
    elif a == ["shell", "reboot", "-p"]:
        st["mode"] = "off"
    json.dump(st, open(path + ".tmp", "w"), indent=1)
    os.replace(path + ".tmp", path)
if bout is not None:
    sys.stdout.buffer.write(bout)
else:
    sys.stdout.write(out)
sys.exit(code)
