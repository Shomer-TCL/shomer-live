package flasher

// Backup-first mode: before the ROM is written, the flasher copies every
// partition of the phone into one zip file on the computer.
//
// How (TCL Flip 2 / MT6739, see README "Backup"):
//  1. `fastboot boot` does not work on this phone's bootloader (refused when
//     locked, fails and boots Android when unlocked), so a reading recovery
//     has to be WRITTEN to the recovery partition: neutronscott's
//     recovery2.img, which has su and busybox.
//  2. `fastboot reboot recovery`, then over adb every partition is READ with
//     dd and streamed into backup-<serial>-<date>.zip on the computer, each
//     one checked against a SHA-256 computed on the phone.
//  3. The zip is re-read and checked against its SHA256SUMS.
//  4. Back to fastboot, and the genuine stock TCL KEKA recovery is written
//     back (the dump recovery must never stay on a Shomer phone: it is a
//     root shell for anyone with a cable).
//
// The only partition this file ever writes is "recovery", and only with one
// of the two images whose SHA-256 is pinned below. Everything else is read.

import (
	"archive/zip"
	"bufio"
	"compress/flate"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Pinned recovery images. mkbundle refuses any other bytes, and
// flashRecovery checks the file again right before writing.
// (vars only so the unit tests can pin their small fake images.)
var (
	// neutronscott's recovery2.img (http://scottn.us/downloads/recovery2.img),
	// built from TCL KEE7 recovery + Magisk su + busybox. 25,165,824 bytes.
	DumpRecoverySHA256 = "394ad6bb38321565b121dec8c5dff098cd238919b766f9cbc0c994f4f0a376ac"
	// Genuine stock TCL T408DL KEKA recovery, rebuilt exactly the way the phone
	// rebuilds it itself (stock KEKA boot + /vendor/recovery-from-boot.p,
	// result SHA-1 40e9d0d4bc1e10efa308e9a279f70b2300ff9200 as in
	// /vendor/bin/install-recovery.sh). 25,165,824 bytes.
	StockRecoverySHA256       = "4fa8dacf08c9fe2131616d377b6c4da5b44bb504df1c67274c37aa7c38b551d3"
	RecoverySize        int64 = 25165824
)

// freeBytesFn is freeBytes; tests replace it.
var freeBytesFn = freeBytes

// RecoveryPartition is the one partition outside AllowedPartitions that the
// backup step may write, and only with the two pinned images above.
const RecoveryPartition = "recovery"

// Partitions never read: RPMB cannot be read with dd (authenticated access only).
var backupSkip = map[string]bool{"mmcblk0rpmb": true, "rpmb": true}

// If any of these is listed by the phone but cannot be read and verified,
// the backup has failed (it is kept, marked INCOMPLETE).
var backupCritical = map[string]bool{
	"proinfo": true, "nvram": true, "nvdata": true, "nvcfg": true, "persist": true,
	"protect1": true, "protect2": true, "seccfg": true, "md1img": true, "md1dsp": true,
	"boot": true, "vendor_boot": true, "dtbo": true, "lk": true, "lk2": true, "logo": true,
	"vbmeta": true, "vbmeta_system": true, "vbmeta_vendor": true, "tee1": true, "tee2": true,
	"super": true, "mmcblk0boot0": true,
}

// backupRequired must be in the phone's partition list BY NAME: if one is
// absent (a blockdev call printed nothing, a missing link, a recovery that
// lists things differently), the backup is INCOMPLETE exactly as if it had
// failed to read. These are on every MT6739 TCL Flip 2 (and the flasher
// itself writes vbmeta, vbmeta_system and vbmeta_vendor, so they exist).
// proinfo, nvram and nvdata hold the IMEI: they are why the backup exists.
var backupRequired = []string{
	"proinfo", "nvram", "nvdata", "nvcfg", "persist", "protect1", "protect2", "seccfg",
	"md1img", "lk", "boot", "logo", "vbmeta", "vbmeta_system", "vbmeta_vendor", "super",
}

// BootAreaName is how the zip names the eMMC boot area (the preloader). The
// phone's /dev/block/by-name never calls it that (at most preloader,
// preloader_a/_b), so the listing command adds it explicitly.
const BootAreaName = "mmcblk0boot0"

// bootAreaDev is the eMMC boot area device.
const bootAreaDev = "/dev/block/mmcblk0boot0"

// isBootArea: the partition is the boot area (preloader), under any name.
func isBootArea(p PhonePartition) bool {
	return p.Dev == bootAreaDev || p.Name == BootAreaName || strings.HasPrefix(p.Name, "preloader")
}

// isCritical: a read failure of this partition makes the backup INCOMPLETE.
// With phone data requested, userdata and metadata (which a userdata restore
// needs) are critical too.
func isCritical(p PhonePartition, userdata bool) bool {
	if backupCritical[p.Name] || isBootArea(p) {
		return true
	}
	return userdata && (p.Name == "userdata" || p.Name == "metadata")
}

// missingRequired lists what must be backed up but is not in the phone's list.
func missingRequired(parts []PhonePartition, userdata bool) []string {
	have := map[string]bool{}
	boot := false
	for _, p := range parts {
		have[p.Name] = true
		if isBootArea(p) {
			boot = true
		}
	}
	var miss []string
	req := append([]string{}, backupRequired...)
	if userdata {
		req = append(req, "userdata", "metadata")
	}
	for _, n := range req {
		if !have[n] {
			miss = append(miss, n)
		}
	}
	if !boot {
		miss = append(miss, BootAreaName+" (preloader)")
	}
	return miss
}

// listPartitionsCmd lists /dev/block/by-name and, explicitly, the eMMC boot area.
const listPartitionsCmd = `for p in /dev/block/by-name/*; do t=$(readlink -f $p); echo "${p##*/} $t $(blockdev --getsize64 $t)"; done; ` +
	`[ -b ` + bootAreaDev + ` ] && echo "` + BootAreaName + ` ` + bootAreaDev + ` $(blockdev --getsize64 ` + bootAreaDev + `)"`

// StreamRunner runs a tool and sends its standard output to w untouched
// (binary safe). Standard error is returned as text.
type StreamRunner interface {
	RunStream(ctx context.Context, tool string, args []string, w io.Writer) (string, error)
}

// PhonePartition is one block device found in recovery.
type PhonePartition struct {
	Name string
	Dev  string // resolved block device, e.g. /dev/block/mmcblk0p32
	Size int64
}

// BackupEntry is one line of the backup manifest.
type BackupEntry struct {
	Name   string `json:"name"`
	File   string `json:"file"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Status string `json:"status"` // ok | failed
	Note   string `json:"note,omitempty"`
}

// BackupInfo is backup-info.json inside the zip.
type BackupInfo struct {
	Tool         string        `json:"tool"`
	Created      string        `json:"created"`
	Serial       string        `json:"serial"`
	Product      string        `json:"product"`
	Build        string        `json:"build"`
	WasUnlocked  bool          `json:"bootloader_was_unlocked_before_this_run"`
	Userdata     bool          `json:"userdata_included"`
	Complete     bool          `json:"complete"`
	Missing      []string      `json:"missing_critical,omitempty"`
	RecoveryNote string        `json:"recovery_note"`
	Partitions   []BackupEntry `json:"partitions"`
}

func (f *Flow) backupAvailable() bool {
	r := f.Payload.Manifest.Recovery
	return r != nil && r.Dump != "" && r.Stock != ""
}

// backupNeed is a first, cautious estimate of the space the zip needs
// (raw sizes, as if nothing compressed). Real sizes are checked again in recovery.
func backupNeed(userdata bool) int64 {
	n := int64(2900) << 20 // every partition except userdata is ~2.75 GB on this phone
	if userdata {
		n += int64(4700) << 20
	}
	return n
}

var byNameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// parsePartitionList reads lines "name /dev/block/xxx size" and drops
// duplicates (two names for one device), RPMB and anything unreadable.
func parsePartitionList(out string) []PhonePartition {
	byDev := map[string]PhonePartition{}
	for _, l := range strings.Split(out, "\n") {
		fs := strings.Fields(strings.TrimSpace(l))
		if len(fs) != 3 || !byNameRe.MatchString(fs[0]) || !strings.HasPrefix(fs[1], "/dev/block/") {
			continue
		}
		size, err := strconv.ParseInt(fs[2], 10, 64)
		if err != nil || size <= 0 || backupSkip[fs[0]] || strings.Contains(fs[1], "rpmb") {
			continue
		}
		p := PhonePartition{Name: fs[0], Dev: fs[1], Size: size}
		if old, ok := byDev[p.Dev]; ok {
			// keep the real name, not an alias like preloader_a
			if strings.HasPrefix(p.Name, "preloader") || (!strings.HasPrefix(old.Name, "preloader") && old.Name < p.Name) {
				continue
			}
		}
		byDev[p.Dev] = p
	}
	var ps []PhonePartition
	for _, p := range byDev {
		ps = append(ps, p)
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].Name < ps[j].Name })
	return ps
}

var shaRe = regexp.MustCompile(`(?m)^([0-9a-f]{64})\b`)

func parseSHA(out string) string {
	m := shaRe.FindStringSubmatch(strings.TrimSpace(out))
	if m == nil {
		return ""
	}
	return m[1]
}

func (f *Flow) su(timeout time.Duration, cmd string) (string, error) {
	return f.adb(timeout, "-s", f.recSerial, "shell", "su -c '"+cmd+"'")
}

// flashRecovery writes one of the two pinned images to the recovery partition.
func (f *Flow) flashRecovery(kind string) error {
	var name, want string
	switch kind {
	case "dump":
		name, want = f.Payload.Manifest.Recovery.Dump, DumpRecoverySHA256
	case "stock":
		name, want = f.Payload.Manifest.Recovery.Stock, StockRecoverySHA256
	default:
		return fmt.Errorf("internal safety stop: unknown recovery image %q", kind)
	}
	p, err := f.extractPayloadFile(name, "recovery-"+kind+".img")
	if err != nil {
		return err
	}
	got, size, err := fileSHA(p)
	if err != nil {
		return err
	}
	if got != want || size != RecoverySize {
		return fmt.Errorf("internal safety stop: %s recovery image is not the pinned one (sha256 %s, %d bytes)", kind, got, size)
	}
	if err := f.requireMode(ModeBootloader); err != nil {
		return err
	}
	if err := f.checkRecoveryPartition(); err != nil {
		return err
	}
	// From this point a write is attempted; failure leaves the actual contents
	// uncertain, not proof that the rooted recovery was installed successfully.
	f.dumpRecoveryOn = true
	f.recoveryWritten = true
	out, err := f.fastboot(5*time.Minute, true, "flash", RecoveryPartition, p)
	if err != nil || fastbootFailed(out) {
		return fmt.Errorf("writing recovery failed: %s", lastLines(out, 3))
	}
	if kind == "dump" {
		f.dumpRecoveryOn = true
	} else {
		f.dumpRecoveryOn = false // stock is back
	}
	return nil
}

func fileSHA(p string) (string, int64, error) {
	fh, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer fh.Close()
	h := sha256.New()
	n, err := io.Copy(h, fh)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

func (f *Flow) extractPayloadFile(name, out string) (string, error) {
	r, err := f.Payload.Reader(name)
	if err != nil {
		return "", err
	}
	p := filepath.Join(f.Opts.WorkDir, out)
	w, err := os.Create(p)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(w, r); err != nil {
		w.Close()
		return "", err
	}
	return p, w.Close()
}

// waitRecoveryAdb waits for the phone to show up in adb in recovery mode.
func (f *Flow) waitRecoveryAdb(d time.Duration) (string, error) {
	deadline := time.Now().Add(d)
	for {
		ds := f.adbDevices()
		var rec []Device
		for _, x := range ds {
			if x.State == "recovery" || x.State == "device" || x.State == "unauthorized" {
				rec = append(rec, x)
			}
		}
		if len(rec) > 1 {
			return "", refuse("more than one phone is connected over adb. Unplug every other phone")
		}
		if len(rec) == 1 && rec[0].State != "unauthorized" {
			mode, modeErr := f.adb(20*time.Second, "-s", rec[0].Serial, "shell", "getprop", "ro.bootmode")
			if modeErr == nil && strings.TrimSpace(mode) == "recovery" {
				return rec[0].Serial, nil
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("the phone did not show up in recovery mode over USB")
		}
		f.sleep(2 * time.Second)
	}
}

// backToFastboot leaves recovery and catches the bootloader again.
func (f *Flow) backToFastboot() error {
	if ok, err := f.waitFastboot(time.Second); err != nil {
		return err
	} else if ok {
		return nil // still (or already) in fastboot
	}
	if f.recSerial != "" {
		f.adb(30*time.Second, "-s", f.recSerial, "reboot", "bootloader")
		ok, err := f.waitFastboot(f.FastbootWait)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		f.UI.Say("    The phone did not go to fastboot by itself. Turning it off so we can catch it.")
		f.adb(20*time.Second, "-s", f.recSerial, "shell", "reboot", "-p")
	}
	return f.catchFastboot()
}

// restoreStockRecovery puts the genuine stock recovery back. It is called
// after a good backup AND after a failed one.
func (f *Flow) restoreStockRecovery() error {
	f.event(EvStepStart, StepBackupRestore, "")
	f.UI.Step("Putting the stock TCL recovery back")
	if err := f.backToFastboot(); err != nil {
		return err
	}
	if err := f.flashRecovery("stock"); err != nil {
		return err
	}
	f.UI.Say("    OK - the stock TCL KEKA recovery is back (the backup recovery is gone from the phone).")
	if strings.EqualFold(f.buildOK, "UPCI") {
		f.UI.Say("    (A 4058G with UPCI is KEKA hardware: it gets TCL's KEKA recovery, the same one Shomer's KEKA base expects.)")
	}
	f.event(EvStepDone, StepBackupRestore, "")
	return nil
}

// errBackupSkipped: the person said no to "Start the backup?" (console only).
var errBackupSkipped = fmt.Errorf("backup skipped by you")

// ErrRecoveryNotRestored means the stock recovery could not be put back after
// the backup step. Nothing else is ever written to the phone in that state.
type ErrRecoveryNotRestored struct{ Err error }

func (e ErrRecoveryNotRestored) Error() string {
	return "the stock TCL recovery could not be put back (" + e.Err.Error() + "). Nothing else was written. Run this program again with --restore-recovery (or again normally): it puts it back first"
}

// backupStep runs the backup and makes it NOT blocking: if it fails, the
// person sees what failed and decides whether to flash without a backup
// (or --continue-without-backup decides). The stock recovery is always back
// on the phone before this returns nil.
func (f *Flow) backupStep() error {
	err := f.Backup()
	if err == nil {
		return nil
	}
	if err == errBackupSkipped {
		f.UI.Say("    Backup skipped by you. Flashing without a backup.")
		for _, s := range []string{StepBackupRecovery, StepBackupRead, StepBackupRestore} {
			f.event(EvStepSkipped, s, "skipped by you")
		}
		return nil
	}
	var nr ErrRecoveryNotRestored
	if errors.As(err, &nr) || f.dumpRecoveryOn {
		// fail closed: never flash while the reading recovery may be on the phone
		f.event(EvStepFailed, StepBackupRestore, err.Error())
		return err
	}
	if errors.Is(err, ErrInputClosed) {
		return err
	}
	msg := err.Error()
	f.UI.Event(Event{Kind: EvBackupFailed, Detail: msg, Path: f.IncompletePath})
	lines := []string{"BACKUP FAILED: " + msg}
	if f.recoveryWritten {
		lines = append(lines, "The stock TCL recovery is back on the phone. Nothing else was written yet.")
	} else {
		lines = append(lines, "Nothing was written to the phone by the backup step.")
	}
	if f.IncompletePath != "" {
		lines = append(lines, "The part that was copied is kept, clearly marked INCOMPLETE:", "  "+f.IncompletePath)
	} else {
		lines = append(lines, "No backup file was kept.")
	}
	f.UI.Warn(lines...)
	if f.Opts.ContinueWithoutBackup {
		f.UI.Say("    --continue-without-backup: flashing WITHOUT a backup.")
		return nil
	}
	yes, aerr := f.UI.YesNo("    Backup failed: continue flashing WITHOUT a backup?", false)
	if aerr != nil {
		return aerr
	}
	if !yes {
		return refuse("stopped by you after the backup failed (%s). The phone was not flashed", msg)
	}
	f.UI.Say("    OK - flashing WITHOUT a backup.")
	return nil
}

// Backup runs the whole backup step. The phone must be in fastboot and unlocked.
// On every path that wrote recovery, the stock recovery is written back before
// it returns; if that fails the error is ErrRecoveryNotRestored.
func (f *Flow) Backup() (err error) {
	if err := f.validateBackupDestination(); err != nil {
		return err
	}
	if !f.backupAvailable() {
		f.event(EvStepFailed, StepBackupRecovery, "not in this flasher")
		return refuse("this flasher was built without the backup recovery, so it cannot make a backup (download a newer flasher, or use --no-backup)")
	}
	userdata := f.Opts.BackupUserdata && !f.wiped
	if f.Opts.BackupUserdata && f.wiped {
		f.UI.Say("    (Phone data is NOT in the backup: unlocking just erased it, so there is nothing left to save.)")
	}
	dir := f.Opts.BackupDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.event(EvStepFailed, StepBackupRecovery, "backup folder")
		return refuse("cannot create the backup folder %s: %v (use --backup-dir <folder>)", dir, err)
	}
	if userdata && fsIsFATFn(dir) {
		f.event(EvStepFailed, StepBackupRecovery, "FAT32 drive")
		return refuse("the backup folder %s is on a FAT32 drive, which cannot hold a file of 4 GB or more; with phone data the backup is about 7 GB. Choose a folder on an NTFS or exFAT drive (or switch phone data off)", dir)
	}
	if free, ferr := freeBytesFn(dir); ferr == nil && free < backupNeed(userdata) {
		f.event(EvStepFailed, StepBackupRecovery, "not enough free space")
		return refuse("not enough free space for the backup in %s: need about %d MB, have %d MB (free some space, or use --backup-dir <folder on a bigger disk>)", dir, backupNeed(userdata)>>20, free>>20)
	}
	f.UI.Step("Backup: copying the whole phone to this computer first")
	f.UI.Say("    1. A special recovery (neutronscott's recovery2) is written to the phone's recovery partition.")
	f.UI.Say("    2. The phone restarts into it, and every partition is READ and saved into one zip file here:")
	f.UI.Say("         %s", dir)
	f.UI.Say("    3. The stock TCL recovery is written back. Nothing else on the phone is changed by the backup.")
	if userdata {
		f.UI.Say("    Phone data (userdata, ~4.6 GB, encrypted) IS included. It can only be put back on this same phone.")
	} else if !f.wiped {
		f.UI.Say("    Phone data (userdata) is NOT included (use --backup-userdata for that).")
	}
	f.UI.Say("    Takes about 5-15 minutes. The zip holds this phone's IMEI and identity: keep it private.")
	if !f.Opts.Preconfirmed {
		ok, err := f.UI.YesNo("    Start the backup? (n = flash without a backup)", true)
		if err != nil {
			return err
		}
		if !ok {
			return errBackupSkipped
		}
	}

	// From the moment recovery is touched, the stock recovery must go back,
	// whatever happens (even if writing recovery2 itself failed half way).
	defer func() {
		if f.dumpRecoveryOn {
			if rerr := f.restoreStockRecovery(); rerr != nil {
				f.event(EvStepFailed, StepBackupRestore, rerr.Error())
				f.UI.Warn("COULD NOT PUT THE STOCK RECOVERY BACK: "+rerr.Error(),
					"NOTHING ELSE WILL BE WRITTEN. Run this program again (or with --restore-recovery):",
					"it puts the stock recovery back before anything else.",
					"Recovery contents are uncertain; it may contain the rooted recovery. Keep the phone private until verified.")
				err = ErrRecoveryNotRestored{Err: rerr}
			}
		}
	}()

	f.event(EvStepStart, StepBackupRecovery, "")
	if err := f.flashRecovery("dump"); err != nil {
		f.event(EvStepFailed, StepBackupRecovery, err.Error())
		return f.writeFailed(err, false)
	}
	f.UI.Say("    Restarting the phone into the backup recovery (the screen may stay on the TCL logo / warning, that is fine)...")
	if out, rerr := f.fastboot(60*time.Second, true, "reboot", "recovery"); rerr != nil || fastbootFailed(out) {
		return fmt.Errorf("could not reboot into backup recovery: %s", lastLines(out, 4))
	}
	serial, err := f.waitRecoveryAdb(3 * time.Minute)
	if err != nil {
		f.event(EvStepFailed, StepBackupRecovery, err.Error())
		return refuse("%v. The backup was not made", err)
	}
	f.recSerial = serial
	if out, _ := f.su(30*time.Second, "id"); !strings.Contains(out, "uid=0") {
		f.event(EvStepFailed, StepBackupRecovery, "no root")
		return refuse("the backup recovery did not give root access (%s). The backup was not made", strings.TrimSpace(out))
	}
	f.event(EvStepDone, StepBackupRecovery, "")
	f.event(EvStepStart, StepBackupRead, "")
	list, _ := f.su(60*time.Second, listPartitionsCmd)
	parts := parsePartitionList(list)
	if len(parts) < 10 {
		f.event(EvStepFailed, StepBackupRead, "partition list")
		return refuse("the phone listed only %d partitions; something is wrong. The backup was not made", len(parts))
	}
	var sel []PhonePartition
	var total int64
	for _, p := range parts {
		if p.Name == "userdata" && !userdata {
			continue
		}
		sel = append(sel, p)
		total += p.Size
	}
	if free, ferr := freeBytesFn(dir); ferr == nil && free < total+(64<<20) {
		f.event(EvStepFailed, StepBackupRead, "not enough free space")
		return refuse("not enough free space for the backup in %s: the phone has %d MB to copy, the disk has %d MB free", dir, total>>20, free>>20)
	}
	serialSafe := regexp.MustCompile(`[^A-Za-z0-9]`).ReplaceAllString(serial, "")
	if serialSafe == "" {
		serialSafe = "phone"
	}
	base := fmt.Sprintf("backup-%s-%s", serialSafe, time.Now().Format("20060102-150405"))
	final := filepath.Join(dir, base+".zip")
	tmp := final + ".partial"
	info := BackupInfo{Tool: "shomer-flasher", Created: time.Now().UTC().Format(time.RFC3339), Serial: serial,
		Product: f.product, Build: f.buildOK, WasUnlocked: !f.wiped, Userdata: userdata,
		RecoveryNote: "recovery.img in this zip is the backup recovery (recovery2) that was on the phone while it was read; the phone's previous recovery was overwritten first, because the phone cannot be read without it. The genuine stock TCL KEKA recovery was written back afterwards."}
	f.UI.Step("Copying %d partitions (%d MB) into %s", len(sel), total>>20, final)
	notListed := missingRequired(sel, userdata)
	if len(notListed) > 0 {
		f.UI.Say("    NOT in the phone's partition list (cannot be copied): %s", strings.Join(notListed, ", "))
	}
	missing, err := f.writeBackupZip(tmp, sel, &info, userdata, notListed)
	if err != nil {
		// the zip itself could not be written (disk full, I/O): it is not a usable file
		os.Remove(tmp)
		f.event(EvStepFailed, StepBackupRead, err.Error())
		return err
	}
	f.UI.Say("    Checking the zip file again...")
	if verr := verifyBackupZip(tmp); verr != nil {
		os.Remove(tmp)
		f.event(EvStepFailed, StepBackupRead, "zip check")
		return refuse("the backup zip did not pass its check: %v. The backup was not kept", verr)
	}
	if len(missing) > 0 {
		// keep what was copied, but never under a name that looks complete
		inc := filepath.Join(dir, base+"-INCOMPLETE.zip")
		if rerr := os.Rename(tmp, inc); rerr != nil {
			os.Remove(tmp)
		} else {
			f.IncompletePath = inc
		}
		f.event(EvStepFailed, StepBackupRead, "missing: "+strings.Join(missing, ", "))
		return refuse("these partitions could not be copied and checked: %s. The backup is INCOMPLETE", strings.Join(missing, ", "))
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		f.event(EvStepFailed, StepBackupRead, err.Error())
		return err
	}
	f.BackupPath = final
	f.UI.Say("    OK - backup saved and checked: %s", final)
	f.event(EvStepDone, StepBackupRead, "")
	f.UI.Event(Event{Kind: EvBackupSaved, Path: final})
	return nil // the deferred restore puts the stock recovery back
}

type countWriter struct {
	w     io.Writer
	n     int64
	f     *Flow
	t     int64
	label string
}

func (c *countWriter) Write(b []byte) (int, error) {
	n, err := c.w.Write(b)
	c.n += int64(n)
	c.f.UI.Progress("reading "+c.label, c.n, c.t)
	return n, err
}

// writeBackupZip reads every partition into the zip. It returns the critical
// partitions that could not be copied (the zip is still a valid file), or an
// error if the zip itself could not be written.
func (f *Flow) writeBackupZip(path string, parts []PhonePartition, info *BackupInfo, userdata bool, notListed []string) (missing []string, err error) {
	sr, ok := f.Run.(StreamRunner)
	if !ok {
		return nil, fmt.Errorf("internal: runner cannot stream")
	}
	fh, err := os.Create(path)
	if err != nil {
		return nil, refuse("cannot create %s: %v", path, err)
	}
	defer fh.Close()
	bw := bufio.NewWriterSize(fh, 1<<20)
	zw := zip.NewWriter(bw)
	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) { return flate.NewWriter(w, flate.BestSpeed) })
	var failedCritical []string
	for i, p := range parts {
		e := BackupEntry{Name: p.Name, File: p.Name + ".img"}
		var lastErr string
		for attempt := 1; attempt <= 2; attempt++ {
			f.UI.Say("    [%d/%d] %s (%d MB)%s", i+1, len(parts), p.Name, p.Size>>20, map[bool]string{true: " - trying again", false: ""}[attempt > 1])
			phoneOut, _ := f.su(30*time.Minute, "sha256sum "+p.Dev)
			want := parseSHA(phoneOut)
			if want == "" {
				lastErr = "phone could not checksum it"
				continue
			}
			hdr := &zip.FileHeader{Name: e.File, Method: zip.Deflate, Modified: time.Now()}
			if attempt > 1 {
				hdr.Name = fmt.Sprintf("%s.retry%d.img", p.Name, attempt) // zip entries cannot be removed; mark it
			}
			zf, err := zw.CreateHeader(hdr)
			if err != nil {
				return nil, zipWriteErr(err)
			}
			h := sha256.New()
			var sink io.Writer = io.MultiWriter(zf, h)
			var ps *propScanner
			if p.Name == "super" {
				// the phone's own build fingerprints, for the variant check before flashing
				ps = &propScanner{}
				sink = io.MultiWriter(zf, h, ps)
			}
			cw := &countWriter{w: sink, f: f, t: p.Size, label: fmt.Sprintf("[%d/%d] %s", i+1, len(parts), p.Name)}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
			cmd := fmt.Sprintf("su -c 'busybox stty raw 2>/dev/null; busybox dd if=%s bs=1048576 2>/dev/null'", p.Dev)
			f.UI.Log("run: adb exec-out %s", cmd)
			stderr, rerr := sr.RunStream(ctx, "adb", []string{"-s", f.recSerial, "exec-out", cmd}, cw)
			cancel()
			got := hex.EncodeToString(h.Sum(nil))
			f.UI.Log("%s: read %d bytes sha %s, phone sha %s, err %v %s", p.Name, cw.n, got, want, rerr, strings.TrimSpace(stderr))
			if rerr == nil && cw.n == p.Size && got == want {
				e.File, e.Size, e.SHA256, e.Status = hdr.Name, cw.n, got, "ok"
				lastErr = ""
				if ps != nil {
					f.phoneProps = ps.Found()
					f.UI.Log("build props found in the phone's super: %q", f.phoneProps)
				}
				break
			}
			lastErr = fmt.Sprintf("read %d of %d bytes, checksum %s, phone says %s", cw.n, p.Size, short(got), short(want))
		}
		if lastErr != "" {
			e.Status, e.Note, e.Size = "failed", lastErr, 0
			f.UI.Say("    %s could NOT be copied: %s", p.Name, lastErr)
			if isCritical(p, userdata) {
				failedCritical = append(failedCritical, p.Name)
			}
		}
		if p.Name == "userdata" && e.Status == "ok" {
			f.userdataSaved = true
		}
		info.Partitions = append(info.Partitions, e)
	}
	for _, n := range notListed {
		info.Partitions = append(info.Partitions, BackupEntry{Name: n, Status: "missing", Note: "not in the phone's partition list"})
		failedCritical = append(failedCritical, n)
	}
	info.Complete = len(failedCritical) == 0
	info.Missing = failedCritical
	// SHA256SUMS (sha256sum -c format) and backup-info.json
	var sums strings.Builder
	for _, e := range info.Partitions {
		if e.Status == "ok" {
			fmt.Fprintf(&sums, "%s  %s\n", e.SHA256, e.File)
		}
	}
	w, err := zw.Create("SHA256SUMS")
	if err != nil {
		return nil, err
	}
	io.WriteString(w, sums.String())
	j, _ := json.MarshalIndent(info, "", "  ")
	w, err = zw.Create("backup-info.json")
	if err != nil {
		return nil, err
	}
	w.Write(j)
	w, _ = zw.Create("README.txt")
	if len(failedCritical) > 0 {
		fmt.Fprintf(w, "INCOMPLETE BACKUP: these partitions could NOT be copied (or were not in the phone's list): %s.\nThe other files listed in SHA256SUMS are good copies.\n\n", strings.Join(failedCritical, ", "))
	}
	io.WriteString(w, backupReadme)
	if err := zw.Close(); err != nil {
		return nil, zipWriteErr(err)
	}
	if err := bw.Flush(); err != nil {
		return nil, zipWriteErr(err)
	}
	if err := fh.Sync(); err != nil {
		return nil, err
	}
	return failedCritical, nil
}

// zipWriteErr explains a failed zip write: a full disk, or a drive that
// cannot hold files of 4 GB or more (FAT32).
func zipWriteErr(err error) error {
	if errors.Is(err, syscall.EFBIG) || strings.Contains(strings.ToLower(err.Error()), "file too large") {
		return refuse("writing the zip failed: %v (the drive cannot hold a file this big: FAT32 stops at 4 GB; use an NTFS or exFAT drive)", err)
	}
	return refuse("writing the zip failed: %v (disk full?)", err)
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12] + "..."
	}
	if s == "" {
		return "(none)"
	}
	return s
}

// verifyBackupZip re-reads every entry and checks it against SHA256SUMS.
func verifyBackupZip(path string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	sf, ok := files["SHA256SUMS"]
	if !ok {
		return fmt.Errorf("no SHA256SUMS in the zip")
	}
	rc, err := sf.Open()
	if err != nil {
		return err
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	n := 0
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		fs := strings.Fields(l)
		if len(fs) != 2 {
			continue
		}
		zf, ok := files[fs[1]]
		if !ok {
			return fmt.Errorf("%s is missing", fs[1])
		}
		r, err := zf.Open()
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, r)
		r.Close()
		if err != nil {
			return fmt.Errorf("%s: %v", fs[1], err)
		}
		if hex.EncodeToString(h.Sum(nil)) != fs[0] {
			return fmt.Errorf("%s does not match its checksum", fs[1])
		}
		n++
	}
	if n == 0 {
		return fmt.Errorf("the zip has no partitions")
	}
	return nil
}

const backupReadme = `Shomer flasher - full backup of one TCL Flip 2, made BEFORE Shomer OS was written.

Each <partition>.img is a raw copy of that partition, read with dd in recovery.
SHA256SUMS lists every good copy (check with: sha256sum -c SHA256SUMS).
backup-info.json says which phone, when, and any partition that could not be read.

KEEP THIS FILE PRIVATE: proinfo, nvram, nvdata and nvcfg hold this phone's IMEI and
radio calibration. They belong to THIS phone only. Never write them to another phone.

Only files listed in SHA256SUMS are good copies (a file named *.retry2.img is a second
try; a file not in the list, e.g. a failed first try, must not be used).

recovery.img in this zip is the backup recovery that was on the phone while it was being
read (the phone's earlier recovery had to be replaced first). The stock TCL recovery was
written back afterwards.

userdata (if present) is encrypted by the phone; it can only ever be restored to this
same phone together with its metadata partition, and even then it is not guaranteed.

To put the phone back to how it was (needs an unlocked bootloader, fastboot):
  fastboot flash super super.img      (in fastbootd: fastboot reboot fastboot first, if the bootloader refuses)
  fastboot flash vbmeta vbmeta.img
  fastboot flash vbmeta_system vbmeta_system.img
  fastboot flash vbmeta_vendor vbmeta_vendor.img
  fastboot flash boot boot.img
Do NOT flash preloader (mmcblk0boot*), lk, seccfg, efuse, otp, proinfo, nvram, nvdata
unless an expert tells you to: a wrong write there can brick the phone or lose its IMEI.
`
