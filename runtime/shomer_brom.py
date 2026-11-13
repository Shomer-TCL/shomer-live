#!/usr/bin/python3
"""Strict helpers shared by the offline BROM enrollment and flash commands."""
import binascii
import hashlib
import json
import pathlib
import re
import struct

SECTOR = 512
HEAD_SIZE = 4096
HWCODE = 0x6739
PARTS = ("swversion", "super", "vbmeta", "vbmeta_system", "vbmeta_vendor")
WRITE_PARTS = ("vbmeta", "vbmeta_system", "vbmeta_vendor", "super")


def sha_bytes(data):
    return hashlib.sha256(data).hexdigest()


def sha_file(path, offset=0, length=None):
    h = hashlib.sha256()
    with pathlib.Path(path).open("rb") as f:
        f.seek(offset)
        left = length
        while left is None or left:
            block = f.read(1024 * 1024 if left is None else min(1024 * 1024, left))
            if not block:
                break
            h.update(block)
            if left is not None:
                left -= len(block)
    return h.hexdigest()


def gpt_partitions(user_image):
    """Parse and CRC-check the primary GPT from an MTKClient USER dump."""
    path = pathlib.Path(user_image)
    size = path.stat().st_size
    with path.open("rb") as f:
        f.seek(SECTOR)
        sector = f.read(SECTOR)
        if len(sector) != SECTOR or sector[:8] != b"EFI PART":
            raise ValueError("USER dump has no primary GPT header")
        header_size = struct.unpack_from("<I", sector, 12)[0]
        stored_crc = struct.unpack_from("<I", sector, 16)[0]
        if header_size < 92 or header_size > SECTOR:
            raise ValueError("invalid GPT header size")
        check = bytearray(sector[:header_size])
        check[16:20] = b"\0" * 4
        if (binascii.crc32(check) & 0xffffffff) != stored_crc:
            raise ValueError("primary GPT header CRC mismatch")
        entries_lba = struct.unpack_from("<Q", sector, 72)[0]
        count, entry_size = struct.unpack_from("<II", sector, 80)
        entries_crc = struct.unpack_from("<I", sector, 88)[0]
        if count <= 0 or count > 4096 or entry_size < 128 or entry_size > 4096:
            raise ValueError("invalid GPT partition-entry geometry")
        entries_len = count * entry_size
        if entries_lba * SECTOR + entries_len > size:
            raise ValueError("GPT entries extend past USER dump")
        f.seek(entries_lba * SECTOR)
        entries = f.read(entries_len)
        if len(entries) != entries_len or (binascii.crc32(entries) & 0xffffffff) != entries_crc:
            raise ValueError("primary GPT entries CRC mismatch")
    result = {}
    for i in range(count):
        e = entries[i * entry_size:(i + 1) * entry_size]
        if e[:16] == b"\0" * 16:
            continue
        start, end = struct.unpack_from("<QQ", e, 32)
        if end < start or (end + 1) * SECTOR > size:
            raise ValueError("GPT partition is outside USER dump")
        name = e[56:128].decode("utf-16le", errors="strict").split("\0", 1)[0].strip()
        key = name.casefold()
        if key in result:
            raise ValueError("duplicate GPT partition name: " + name)
        result[key] = {
            "name": name,
            "offset": start * SECTOR,
            "size": (end - start + 1) * SECTOR,
            # Keep both GUIDs in the reference: equal partition sizes alone
            # are not enough to treat two phones as the same storage layout.
            "type_guid": e[:16].hex(),
            "unique_guid": e[16:32].hex(),
        }
    return result


def _read_at(path, offset, length):
    with pathlib.Path(path).open("rb") as f:
        f.seek(offset)
        data = f.read(length)
    if len(data) != length:
        raise ValueError("short read while parsing backup: " + str(path))
    return data


def _validate_backup(directory):
    root = pathlib.Path(directory)
    info_path = root / "backup-info.json"
    if not info_path.is_file():
        raise ValueError("expected a completed Shomer BROM backup directory")
    info = json.loads(info_path.read_text())
    if info.get("complete") is not True:
        raise ValueError("backup-info.json does not mark this backup complete")
    entries = {e.get("file"): e for e in info.get("entries", [])}
    for name in ("user.bin", "boot1.bin", "boot2.bin"):
        p = root / name
        e = entries.get(name)
        if not p.is_file() or not e:
            raise ValueError("completed backup is missing " + name)
        if p.stat().st_size != e.get("size") or sha_file(p) != e.get("sha256"):
            raise ValueError("backup manifest verification failed for " + name)
    sums = root / "SHA256SUMS"
    if not sums.is_file():
        raise ValueError("backup SHA256SUMS is missing")
    return root, info


def fingerprint_from_backup(directory, require_model_marker=True):
    root, info = _validate_backup(directory)
    user = root / "user.bin"
    parts = gpt_partitions(user)
    missing = [name for name in PARTS if name.casefold() not in parts]
    if missing:
        raise ValueError("GPT is missing required identity partitions: " + ", ".join(missing))
    sw = parts["swversion"]
    sw_bytes = _read_at(user, sw["offset"], sw["size"])
    super_part = parts["super"]
    super_head = _read_at(user, super_part["offset"], min(HEAD_SIZE, super_part["size"]))
    if b"UPCI" not in sw_bytes.upper():
        raise ValueError("reference swversion partition does not identify UPCI")
    # The product fingerprint should be present in the ext4-backed super bytes.
    # Search in a bounded streaming pass so model ambiguity fails at enrollment.
    if require_model_marker and not scan_region(user, super_part["offset"], super_part["size"], b"TCL/4058G/"):
        raise ValueError("reference super does not contain the 4058G product fingerprint")
    heads = {}
    for name in ("super", "vbmeta", "vbmeta_system", "vbmeta_vendor"):
        p = parts[name]
        n = min(HEAD_SIZE, p["size"])
        heads[name] = {"size": n, "sha256": sha_bytes(_read_at(user, p["offset"], n))}
    return {
        "schema": 1,
        "model": "4058G",
        "build": "UPCI",
        "hwcode": f"0x{HWCODE:04x}",
        "boot1": {"size": (root / "boot1.bin").stat().st_size, "sha256": sha_file(root / "boot1.bin")},
        "swversion": {"size": sw["size"], "sha256": sha_bytes(sw_bytes), "contains": "UPCI"},
        "heads": heads,
        "gpt": {name: parts[name] for name in ("super", "vbmeta", "vbmeta_system", "vbmeta_vendor")},
        "backup_info_sha256": sha_file(root / "backup-info.json"),
    }


def write_log(logdir, label, command, timeout=3600):
    """Run one MTKClient command, preserving stdout/stderr for diagnosis."""
    import subprocess
    logdir = pathlib.Path(logdir)
    logdir.mkdir(parents=True, exist_ok=True)
    path = logdir / (label + ".log")
    with path.open("wb") as out:
        try:
            proc = subprocess.run(command, stdout=out, stderr=subprocess.STDOUT, timeout=timeout)
        except Exception as exc:
            out.write(("\nERROR: " + str(exc) + "\n").encode())
            raise ValueError(label + " failed; see " + str(path)) from exc
    if proc.returncode:
        raise ValueError(label + " failed with exit " + str(proc.returncode) + "; see " + str(path))
    return path


def live_fingerprint(profile, logdir, mtk="mtk"):
    """Read BROM chip code, preloader area, swversion and partition heads."""
    import tempfile
    import shutil
    mtk_path = shutil.which(mtk) or mtk
    root = pathlib.Path(logdir) / "identity"
    root.mkdir(parents=True, exist_ok=True)
    chip_log = write_log(root, "gettargetconfig", [mtk_path, "gettargetconfig"])
    hwcode = parse_hwcode(chip_log.read_text(errors="replace"))
    if hwcode != HWCODE:
        raise ValueError(f"unexpected MTK hardware code 0x{hwcode:04x}; expected 0x{HWCODE:04x}")
    boot1 = root / "boot1.live.bin"
    write_log(root, "boot1", [mtk_path, "rf", str(boot1), "--parttype", "boot1"], timeout=3600)
    if not boot1.is_file() or boot1.stat().st_size != profile["boot1"]["size"]:
        raise ValueError("BROM boot1 read size differs from the trusted reference")
    sw = root / "swversion.live.bin"
    write_log(root, "swversion", [mtk_path, "r", "swversion", str(sw)], timeout=600)
    if not sw.is_file() or sw.stat().st_size != profile["swversion"]["size"]:
        raise ValueError("BROM swversion read size differs from the trusted reference")
    heads = {}
    for part, item in profile["heads"].items():
        target = root / (part + ".head.bin")
        write_log(root, part + "-head", [mtk_path, "r", part, str(target), "--offset", "0", "--length", str(item["size"])], timeout=600)
        if not target.is_file() or target.stat().st_size != item["size"]:
            raise ValueError("BROM " + part + " head read is short")
        heads[part] = {"size": item["size"], "sha256": sha_file(target)}
    swdata = pathlib.Path(sw).read_bytes()
    if b"UPCI" not in swdata.upper():
        raise ValueError("connected handset swversion does not identify UPCI")
    return {
        "schema": 1,
        "model": "4058G",
        "build": "UPCI",
        "hwcode": f"0x{hwcode:04x}",
        "boot1": {"size": boot1.stat().st_size, "sha256": sha_file(boot1)},
        "swversion": {"size": sw.stat().st_size, "sha256": sha_file(sw), "contains": "UPCI"},
        "heads": heads,
        "gpt": profile["gpt"],
        "backup_info_sha256": profile["backup_info_sha256"],
    }


def scan_region(path, offset, length, needle):
    overlap = max(0, len(needle) - 1)
    carry = b""
    with pathlib.Path(path).open("rb") as f:
        f.seek(offset)
        left = length
        while left:
            block = f.read(min(1024 * 1024, left))
            if not block:
                return False
            if needle in carry + block:
                return True
            carry = (carry + block)[-overlap:] if overlap else b""
            left -= len(block)
    return False


def compare_fingerprints(expected, actual):
    fields = ("model", "build", "hwcode", "boot1", "swversion", "heads", "gpt")
    return [field for field in fields if expected.get(field) != actual.get(field)]


def parse_hwcode(text):
    matches = re.findall(r"HW code\s*:\s*(0x[0-9a-fA-F]+)", text, re.I)
    if not matches:
        raise ValueError("MTKClient did not report a BROM hardware code")
    vals = {int(v, 16) for v in matches}
    if len(vals) != 1:
        raise ValueError("MTKClient reported conflicting hardware codes")
    return vals.pop()


def validate_identity_profile(profile):
    if profile.get("schema") != 1 or profile.get("model") != "4058G" or profile.get("build") != "UPCI":
        raise ValueError("BROM reference must identify exactly the supported 4058G UPCI build")
    if profile.get("hwcode") != f"0x{HWCODE:04x}":
        raise ValueError("BROM reference has an unsupported MTK hardware code")
    if set(profile.get("heads", {})) != {"super", "vbmeta", "vbmeta_system", "vbmeta_vendor"}:
        raise ValueError("BROM reference is missing partition-head fingerprints")

