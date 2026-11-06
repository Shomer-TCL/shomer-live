package flasher

// Tests for the 1.4.3 fixes (bug list of Nov 4 2026, numbers as in that list).

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func refusedWith(t *testing.T, err error, out interface{ String() string }, want ...string) ErrRefused {
	t.Helper()
	var r ErrRefused
	if !errors.As(err, &r) {
		t.Fatalf("want a refusal, got %v\n%s", err, out)
	}
	for _, w := range want {
		if !strings.Contains(r.Msg, w) {
			t.Fatalf("refusal %q does not contain %q\n%s", r.Msg, w, out)
		}
	}
	return r
}

// Bug 1: a failed phone-data copy makes the backup INCOMPLETE, and erasing the
// data then needs an explicit yes (default No stops before anything is written).
func TestFix1UserdataFailedIsIncompleteAndEraseNeedsYes(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	ph.corruptReads["userdata"] = 9
	ph.corruptReads["metadata"] = 9
	// build, start backup, continue without backup = y, erase anyway? Enter (= No)
	f, out := newFlow(t, tp, ph, "KEKA\n\ny\n\n")
	f.Opts.Backup, f.Opts.BackupUserdata = true, true
	err := f.Execute()
	refusedWith(t, err, out, "phone data was not saved")
	if !strings.Contains(out.String(), "PHONE DATA WAS NOT SAVED") || !strings.Contains(out.String(), "Erase the phone data anyway?") {
		t.Fatalf("no explicit erase question:\n%s", out)
	}
	assertNothingFlashed(t, ph)
	if len(ph.erased) != 0 {
		t.Fatalf("erased %v", ph.erased)
	}
	_, info, _ := readZip(t, f.IncompletePath)
	if info.Complete || !strings.Contains(strings.Join(info.Missing, ","), "userdata") || !strings.Contains(strings.Join(info.Missing, ","), "metadata") {
		t.Fatalf("info %+v", info)
	}
	if !strings.HasSuffix(f.IncompletePath, "-INCOMPLETE.zip") || f.BackupPath != "" {
		t.Fatalf("paths %q %q", f.BackupPath, f.IncompletePath)
	}
}

func TestFix1UserdataFailedEraseExplicitYes(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	ph.corruptReads["userdata"] = 9
	f, out := newFlow(t, tp, ph, "KEKA\n\ny\ny\nFLASH\n")
	f.Opts.Backup, f.Opts.BackupUserdata = true, true
	if err := f.Execute(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(ph.erased) != 1 || ph.flashed["super"] != tp.rawSHA {
		t.Fatalf("erased %v super %q", ph.erased, ph.flashed["super"])
	}
}

// The confirm screen (Preconfirmed, erase ON) and --continue-without-backup do
// not answer the erase question: it is still asked.
func TestFix1PreconfirmedAndContinueFlagStillAskErase(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	ph.corruptReads["userdata"] = 9
	f, out := newFlow(t, tp, ph, "KEKA\nn\n")
	f.Opts.Backup, f.Opts.BackupUserdata, f.Opts.ContinueWithoutBackup = true, true, true
	f.Opts.Preconfirmed, f.Opts.EraseData = true, true
	err := f.Execute()
	refusedWith(t, err, out, "phone data was not saved")
	assertNothingFlashed(t, ph)
	if len(ph.erased) != 0 {
		t.Fatal("erased without a yes")
	}
	// input closed at that question: stops too (unattended runs never erase unsaved data)
	ph2 := backupPhone("fastboot", true)
	ph2.corruptReads["userdata"] = 9
	f2, out2 := newFlow(t, tp, ph2, "KEKA\n")
	f2.Opts.Backup, f2.Opts.BackupUserdata, f2.Opts.ContinueWithoutBackup = true, true, true
	f2.Opts.Preconfirmed, f2.Opts.EraseData = true, true
	if err := f2.Execute(); err == nil || len(ph2.erased) != 0 {
		t.Fatalf("closed input: err %v erased %v\n%s", err, ph2.erased, out2)
	}
	assertNothingFlashed(t, ph2)
}

// A good userdata copy: no extra question.
func TestFix1UserdataSavedNoExtraQuestion(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	f, out := newFlow(t, tp, ph, "KEKA\n\ny\nFLASH\n")
	f.Opts.Backup, f.Opts.BackupUserdata = true, true
	if err := f.Execute(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(out.String(), "NOT SAVED") || len(ph.erased) != 1 {
		t.Fatalf("erased %v\n%s", ph.erased, out)
	}
}

// Bug 2: the IMEI partitions (and the other must-haves) are required by name.
func TestFix2RequiredPartitionsMissingFromListIncomplete(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	ph.dropByName = map[string]bool{"nvram": true, "proinfo": true, "nvdata": true}
	f, out := newFlow(t, tp, ph, "KEKA\n\n\n") // continue without backup? default No
	f.Opts.Backup = true
	err := f.Execute()
	refusedWith(t, err, out, "nvram", "proinfo", "nvdata", "stopped by you")
	assertNothingFlashed(t, ph)
	_, info, _ := readZip(t, f.IncompletePath)
	if info.Complete || strings.Join(info.Missing, ",") != "nvram,nvdata,proinfo" && !(len(info.Missing) == 3) {
		t.Fatalf("info %+v", info)
	}
	for _, e := range info.Partitions {
		if e.Name == "nvram" && e.Status != "missing" {
			t.Fatalf("nvram entry %+v", e)
		}
	}
}

func TestFix2MissingRequiredHelper(t *testing.T) {
	var all []PhonePartition
	for _, n := range backupRequired {
		all = append(all, PhonePartition{Name: n, Dev: "/dev/block/x" + n})
	}
	all = append(all, PhonePartition{Name: "preloader_a", Dev: bootAreaDev})
	if m := missingRequired(all, false); len(m) != 0 {
		t.Fatalf("missing %v", m)
	}
	if m := missingRequired(all, true); strings.Join(m, ",") != "userdata,metadata" {
		t.Fatalf("with userdata: %v", m)
	}
	if m := missingRequired(all[1:len(all)-1], false); len(m) != 2 || m[0] != "proinfo" || !strings.HasPrefix(m[1], BootAreaName) {
		t.Fatalf("missing %v", m)
	}
}

// Bug 3: the boot area is backed up under the name the check looks for, and a
// failed or absent boot area makes the backup INCOMPLETE.
func TestFix3BootAreaChecked(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	ph.corruptReads["mmcblk0boot0"] = 9
	f, out := newFlow(t, tp, ph, "KEKA\n\n\n")
	f.Opts.Backup = true
	refusedWith(t, f.Execute(), out, BootAreaName)
	assertNothingFlashed(t, ph)

	ph2 := backupPhone("fastboot", true)
	ph2.noBootArea = true
	f2, out2 := newFlow(t, tp, ph2, "KEKA\n\n\n")
	f2.Opts.Backup = true
	refusedWith(t, f2.Execute(), out2, BootAreaName+" (preloader)")
	assertNothingFlashed(t, ph2)
}

func TestFix3ListCommandAddsBootArea(t *testing.T) {
	if !strings.Contains(listPartitionsCmd, "[ -b /dev/block/mmcblk0boot0 ]") || strings.Contains(listPartitionsCmd, "'") {
		t.Fatalf("list command %q (must add the boot area and contain no single quote: it runs inside su -c '...')", listPartitionsCmd)
	}
	ps := parsePartitionList("preloader_a /dev/block/mmcblk0boot0 4194304\npreloader_b /dev/block/mmcblk0boot1 4194304\n" +
		"mmcblk0boot0 /dev/block/mmcblk0boot0 4194304\nboot /dev/block/mmcblk0p20 100\n")
	names := []string{}
	for _, p := range ps {
		names = append(names, p.Name)
	}
	if strings.Join(names, ",") != "boot,mmcblk0boot0,preloader_b" {
		t.Fatalf("names %v", names)
	}
	if !isCritical(PhonePartition{Name: "preloader_b", Dev: "/dev/block/mmcblk0boot1"}, false) || !isCritical(PhonePartition{Name: "x", Dev: bootAreaDev}, false) {
		t.Fatal("boot area not critical")
	}
}

// Bug 10: an adb error is never shown as the phone's model.
func TestFix10AdbErrorIsNotTheModel(t *testing.T) {
	tp := makePayload(t, false)
	for _, e := range []string{"error: closed", "adb: device 'FAKE123' not found"} {
		ph := backupPhone("adb", false)
		ph.adbErr = map[string]string{"ro.product.model": e}
		f, out := newFlow(t, tp, ph, "\n")
		r := refusedWith(t, f.Execute(), out, "lost the connection to the phone", "cable")
		if strings.Contains(r.Msg, "this phone is a") || strings.Contains(out.String(), "model \"error") {
			t.Fatalf("error shown as model: %s\n%s", r.Msg, out)
		}
		if ph.mode != "adb" || len(writeCalls(ph)) != 0 {
			t.Fatal("phone touched")
		}
	}
	if !adbErrorText("error: device offline") || adbErrorText("T408DL") || adbErrorText("4058G") {
		t.Fatal("adbErrorText")
	}
}

// Stronger variant check: exact product, getvar facts, the phone's own files.
func TestVariantProductMustBeExact(t *testing.T) {
	tp := makePayload(t, false)
	for _, prod := range []string{"gflip65", "Gflip6_TMO", "gflip6x"} {
		ph := backupPhone("fastboot", true)
		ph.product = prod
		f, out := newFlow(t, tp, ph, "KEKA\n\n") // 1.4.3: warning, Enter = No
		refusedWith(t, f.Execute(), out, "stopped by you after the warning", "made for product gflip6")
		assertNothingFlashed(t, ph)
	}
	if !productAccepted("GFLIP6", []string{"gflip6"}) || !productAccepted(" gflip6 ", []string{"gflip6"}) {
		t.Fatal("gflip6 refused")
	}
}

func TestVariantGetvarFactsNamingOtherVariantRefused(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", false)
	ph.getvarAll = "(bootloader) variant: Gflip6_TMO\n"
	f, out := newFlow(t, tp, ph, "KEKA\n\n") // 1.4.3: warning, Enter = No
	refusedWith(t, f.Execute(), out, "variant", "Gflip6_TMO", "stopped by you after the warning")
	if len(writeCalls(ph)) != 0 {
		t.Fatalf("writes %v", writeCalls(ph))
	}
	// harmless facts pass
	ph2 := backupPhone("fastboot", true)
	ph2.getvarAll = "(bootloader) version-bootloader: KEKA-0001\n(bootloader) partition-size:nvram: 0x500000\n"
	f2, out2 := newFlow(t, tp, ph2, "KEKA\nn\nFLASH\n")
	if err := f2.Execute(); err != nil {
		t.Fatalf("%v\n%s", err, out2)
	}
}

func superWith(ph *fakePhone, props string) {
	b := ph.parts["super"]
	copy(b[100000:], []byte("\x00\x00"+props+"\x00"))
}

func TestVariantFromBackupForeignRefused(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	superWith(ph, "# begin build properties\nro.build.version.incremental=LF91\nro.build.fingerprint=TCL/4058W/Gflip6_TMO:11/RP1A.200720.011/LF91:user/release-keys\n")
	f, out := newFlow(t, tp, ph, "KEKA\n\n\n") // 1.4.3: warning, Enter = No
	f.Opts.Backup = true
	refusedWith(t, f.Execute(), out, "not T408DL KEKA", "Shomer was not written", "stopped by you after the warning")
	assertNothingFlashed(t, ph)
	if ph.flashed["recovery"] != StockRecoverySHA256 || f.BackupPath == "" {
		t.Fatal("backup/stock recovery")
	}
}

func TestVariantFromBackupConfirmed(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	superWith(ph, "ro.vendor.build.fingerprint=TCL/4058G/Gflip6_NA_OM:11/RP1A.200720.011/UPCI:user/release-keys\n")
	f, out := newFlow(t, tp, ph, "KEKA\n\nn\nFLASH\n")
	f.Opts.Backup = true
	if err := f.Execute(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out.String(), "the phone's own software files say UPCI") || ph.flashed["super"] != tp.rawSHA {
		t.Fatalf("not confirmed\n%s", out)
	}
}

func TestVariantFromBackupUnknownAsksDefaultNo(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	superWith(ph, "ro.build.version.incremental=QK50\n")
	f, out := newFlow(t, tp, ph, "KEKA\n\n\n")
	f.Opts.Backup = true
	refusedWith(t, f.Execute(), out, "could not confirm", "stopped by you after the warning")
	assertNothingFlashed(t, ph)
	if !strings.Contains(out.String(), "ro.build.version.incremental=QK50") {
		t.Fatalf("props not shown\n%s", out)
	}
	// yes goes on
	ph2 := backupPhone("fastboot", true)
	superWith(ph2, "ro.build.version.incremental=QK50\n")
	f2, out2 := newFlow(t, tp, ph2, "KEKA\n\ny\nn\nFLASH\n")
	f2.Opts.Backup = true
	if err := f2.Execute(); err != nil || ph2.flashed["super"] != tp.rawSHA {
		t.Fatalf("%v\n%s", err, out2)
	}
}

func TestPropScannerAcrossWrites(t *testing.T) {
	data := []byte(strings.Repeat("\x00", 5000) + "ro.build.fingerprint=TCL/T408DL/Gflip6_TF:11/RP1A.200720.011/KEKA:user/release-keys\n" +
		strings.Repeat("x", 3000) + "ro.system.build.version.incremental=KEKA\x00")
	for _, chunk := range []int{1, 7, 64, 5030, 1 << 20} {
		s := &propScanner{}
		for i := 0; i < len(data); i += chunk {
			s.Write(data[i:min(len(data), i+chunk)])
		}
		got := strings.Join(s.Found(), "|")
		if got != "ro.build.fingerprint=TCL/T408DL/Gflip6_TF:11/RP1A.200720.011/KEKA:user/release-keys|ro.system.build.version.incremental=KEKA" {
			t.Fatalf("chunk %d: %q", chunk, got)
		}
	}
	if c, fo := variantVerdict([]string{"ro.build.version.incremental=KEFS"}, []string{"KEKA", "UPCI"}); c != "" || fo == "" {
		t.Fatal("old T408DL build not foreign")
	}
}

// Suspected: FAT32 with userdata, and the "file too large" explanation.
func TestFAT32WithUserdataRefusedUpFront(t *testing.T) {
	old := fsIsFATFn
	fsIsFATFn = func(string) bool { return true }
	defer func() { fsIsFATFn = old }()
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	f, out := newFlow(t, tp, ph, "KEKA\n\n")
	f.Opts.Backup, f.Opts.BackupUserdata = true, true
	f.Execute()
	if !strings.Contains(out.String(), "FAT32") {
		t.Fatalf("no FAT32 message\n%s", out)
	}
	if _, ok := ph.flashed["recovery"]; ok {
		t.Fatal("recovery written before the FAT32 refusal")
	}
	if e := zipWriteErr(&os.PathError{Op: "write", Path: "x", Err: syscall.EFBIG}); !strings.Contains(e.Error(), "FAT32") {
		t.Fatal(e)
	}
}

// Suspected: a child process that keeps the output pipe open no longer hangs Run.
func TestRunDoesNotHangOnInheritedPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script")
	}
	d := t.TempDir()
	sc := filepath.Join(d, "adb")
	os.WriteFile(sc, []byte("#!/bin/sh\necho started\nsleep 30 &\nexit 0\n"), 0o755)
	old := toolWaitDelay
	toolWaitDelay = 500 * time.Millisecond
	defer func() { toolWaitDelay = old }()
	r := &ExecRunner{Paths: map[string]string{"adb": sc}}
	start := time.Now()
	out, _ := r.Run(context.Background(), "adb", nil, nil)
	if el := time.Since(start); el > 10*time.Second || !strings.Contains(out, "started") {
		t.Fatalf("took %v, out %q", el, out)
	}
}

// vbmeta is checked again right before it is written.
func TestVBMetaCheckedAfterUnpack(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	f, _ := newFlow(t, tp, ph, "")
	if err := f.Prepare(); err != nil {
		t.Fatal(err)
	}
	fi := f.Payload.Manifest.Files["avb/vbmeta"]
	fi.SHA256 = strings.Repeat("0", 64)
	f.Payload.Manifest.Files["avb/vbmeta"] = fi
	if _, err := f.extractVBMeta("vbmeta"); err == nil || !strings.Contains(err.Error(), "CHECKSUM MISMATCH") {
		t.Fatalf("got %v", err)
	}
}
