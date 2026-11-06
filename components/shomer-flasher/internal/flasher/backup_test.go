package flasher

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func init() {
	// tests must not depend on the free space of the machine running them
	freeBytesFn = func(string) (int64, error) { return 1 << 40, nil }
}

func backupPhone(mode string, unlocked bool) *fakePhone {
	p := &fakePhone{mode: mode, model: "T408DL", build: "KEKA", product: "gflip6", unlocked: unlocked}
	p.setupParts()
	return p
}

func zipsIn(t *testing.T, dir string) []string {
	t.Helper()
	var z []string
	es, _ := os.ReadDir(dir)
	for _, e := range es {
		z = append(z, e.Name())
	}
	return z
}

// readZip returns entry name -> sha256 and the parsed backup-info.json.
func readZip(t *testing.T, path string) (map[string]string, BackupInfo, string) {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	got := map[string]string{}
	var info BackupInfo
	var sums string
	for _, f := range zr.File {
		r, _ := f.Open()
		b, _ := io.ReadAll(r)
		r.Close()
		s := sha256.Sum256(b)
		got[f.Name] = hex.EncodeToString(s[:])
		if f.Name == "backup-info.json" {
			json.Unmarshal(b, &info)
		}
		if f.Name == "SHA256SUMS" {
			sums = string(b)
		}
	}
	return got, info, sums
}

func shaOf(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// writes in order: everything that changes the phone
func writeCalls(p *fakePhone) []string {
	var c []string
	for _, x := range p.calls {
		if strings.HasPrefix(x, "fastboot") && (strings.Contains(x, " flash ") || strings.Contains(x, " erase ") || strings.Contains(x, "flashing unlock")) {
			c = append(c, x)
		}
	}
	return c
}

func assertWritesWithBackup(t *testing.T, p *fakePhone) {
	t.Helper()
	for part := range p.flashed {
		if !AllowedPartitions[part] && part != RecoveryPartition {
			t.Fatalf("wrote forbidden partition %s", part)
		}
	}
	if s, ok := p.flashed[RecoveryPartition]; ok && s != StockRecoverySHA256 {
		t.Fatalf("recovery left as %s, not the stock recovery", s)
	}
}

func TestBackupLockedPhoneThenFlash(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("adb", false)
	// Enter (phone off), UNLOCK, start backup (Enter = yes), FLASH
	f, out := newFlow(t, tp, ph, "\nUNLOCK\n\nFLASH\n")
	f.Opts.Backup = true
	if err := f.Execute(); err != nil {
		t.Fatalf("Execute: %v\n%s", err, out)
	}
	// write order: unlock, dump recovery, stock recovery, vbmeta x3, super
	w := writeCalls(ph)
	want := []string{"fastboot flashing unlock", "fastboot flash recovery", "fastboot flash recovery",
		"fastboot --disable-verity --disable-verification flash vbmeta", "fastboot --disable-verity --disable-verification flash vbmeta_system",
		"fastboot --disable-verity --disable-verification flash vbmeta_vendor", "fastboot flash super"}
	if len(w) != len(want) {
		t.Fatalf("writes %v", w)
	}
	for i := range want {
		if !strings.HasPrefix(w[i], want[i]) {
			t.Fatalf("write %d = %q want %q", i, w[i], want[i])
		}
	}
	if ph.flashed["recovery"] != StockRecoverySHA256 {
		t.Fatal("stock recovery not restored")
	}
	if ph.flashed["super"] != tp.rawSHA {
		t.Fatal("super not flashed")
	}
	assertWritesWithBackup(t, ph)
	// the zip
	if f.BackupPath == "" || !strings.HasPrefix(filepath.Base(f.BackupPath), "backup-FAKE123-") || !strings.HasSuffix(f.BackupPath, ".zip") {
		t.Fatalf("backup path %q", f.BackupPath)
	}
	got, info, sums := readZip(t, f.BackupPath)
	for n, b := range ph.parts {
		if n == "userdata" {
			if _, ok := got["userdata.img"]; ok {
				t.Fatal("userdata in backup of a freshly unlocked phone")
			}
			continue
		}
		if got[n+".img"] != shaOf(b) {
			t.Fatalf("%s.img wrong or missing in zip", n)
		}
		if !strings.Contains(sums, shaOf(b)+"  "+n+".img") {
			t.Fatalf("%s missing from SHA256SUMS", n)
		}
	}
	if _, ok := got["mmcblk0rpmb.img"]; ok {
		t.Fatal("rpmb should be skipped")
	}
	if _, ok := got["preloader_a.img"]; ok {
		t.Fatal("alias preloader_a copied twice")
	}
	if info.Serial != "FAKE123" || info.Userdata || info.WasUnlocked || len(info.Partitions) != len(ph.parts)-1 {
		t.Fatalf("info %+v", info)
	}
	if err := verifyBackupZip(f.BackupPath); err != nil {
		t.Fatal(err)
	}
	if names := zipsIn(t, f.Opts.BackupDir); len(names) != 1 {
		t.Fatalf("backup dir holds %v", names)
	}
	if !strings.Contains(out.String(), f.BackupPath) {
		t.Fatal("backup path not shown at the end")
	}
	// every read happened in recovery, never as a write
	for _, c := range ph.calls {
		if strings.Contains(c, "exec-out") && (strings.Contains(c, " of=") || !strings.Contains(c, "if=/dev/block/")) {
			t.Fatalf("odd read command %q", c)
		}
	}
}

func TestBackupAlreadyUnlockedWithUserdata(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	// build, start backup, erase data? no, FLASH
	f, out := newFlow(t, tp, ph, "KEKA\n\nn\nFLASH\n")
	f.Opts.Backup, f.Opts.BackupUserdata = true, true
	if err := f.Execute(); err != nil {
		t.Fatalf("Execute: %v\n%s", err, out)
	}
	got, info, _ := readZip(t, f.BackupPath)
	if got["userdata.img"] != shaOf(ph.parts["userdata"]) || !info.Userdata || !info.WasUnlocked {
		t.Fatalf("userdata not in backup (info %+v)", info)
	}
	assertWritesWithBackup(t, ph)
}

func TestBackupUserdataAfterUnlockIsSkipped(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", false)
	f, out := newFlow(t, tp, ph, "KEKA\nUNLOCK\n\nFLASH\n")
	f.Opts.Backup, f.Opts.BackupUserdata = true, true
	if err := f.Execute(); err != nil {
		t.Fatalf("Execute: %v\n%s", err, out)
	}
	got, _, _ := readZip(t, f.BackupPath)
	if _, ok := got["userdata.img"]; ok {
		t.Fatal("userdata copied right after the unlock wiped it")
	}
	if !strings.Contains(out.String(), "unlocking just erased it") {
		t.Fatal("no explanation why userdata is not included")
	}
}

func TestBackupBadReadIsRetried(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	ph.corruptReads["boot"] = 1
	f, out := newFlow(t, tp, ph, "KEKA\n\nn\nFLASH\n")
	f.Opts.Backup = true
	if err := f.Execute(); err != nil {
		t.Fatalf("Execute: %v\n%s", err, out)
	}
	got, _, sums := readZip(t, f.BackupPath)
	if got["boot.retry2.img"] != shaOf(ph.parts["boot"]) || !strings.Contains(sums, "boot.retry2.img") || strings.Contains(sums, "  boot.img") {
		t.Fatalf("retry not recorded right: %s", sums)
	}
}

// writeIndex is the position of the first write call containing s, or -1.
func writeIndex(p *fakePhone, s string) int {
	for i, c := range writeCalls(p) {
		if strings.Contains(c, s) {
			return i
		}
	}
	return -1
}

func assertNothingFlashed(t *testing.T, ph *fakePhone) {
	t.Helper()
	for _, p := range []string{"super", "vbmeta", "vbmeta_system", "vbmeta_vendor"} {
		if _, ok := ph.flashed[p]; ok {
			t.Fatalf("%s written: %v", p, writeCalls(ph))
		}
	}
}

// stock recovery back BEFORE the first vbmeta/super write
func assertStockBeforeFlash(t *testing.T, ph *fakePhone) {
	t.Helper()
	ws := writeCalls(ph)
	last := -1
	for i, c := range ws {
		if strings.Contains(c, "recovery-stock.img") {
			last = i
		}
	}
	first := writeIndex(ph, "flash vbmeta")
	if last < 0 || first < 0 || last > first {
		t.Fatalf("stock recovery (%d) not written before vbmeta (%d): %v", last, first, ws)
	}
	for i, c := range ws {
		if strings.Contains(c, "recovery-dump.img") && i > last {
			t.Fatalf("recovery2 written after the stock restore: %v", ws)
		}
	}
	if ph.flashed["recovery"] != StockRecoverySHA256 {
		t.Fatal("stock recovery not what stays on the phone")
	}
}

func TestBackupCriticalFailureAskedNoStopsKeepsIncompleteZip(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	ph.corruptReads["nvram"] = 9
	// build, start backup (Enter = yes), "continue without a backup?" (Enter = default No)
	f, out := newFlow(t, tp, ph, "KEKA\n\n\n")
	f.Opts.Backup = true
	err := f.Execute()
	var r ErrRefused
	if !errors.As(err, &r) || !strings.Contains(r.Msg, "nvram") || !strings.Contains(r.Msg, "stopped by you") {
		t.Fatalf("want refusal naming nvram, got %v\n%s", err, out)
	}
	if !strings.Contains(out.String(), "Backup failed: continue flashing WITHOUT a backup? [y/N]") {
		t.Fatalf("prompt not shown:\n%s", out)
	}
	if ph.flashed["recovery"] != StockRecoverySHA256 {
		t.Fatal("stock recovery not put back after a failed backup")
	}
	assertNothingFlashed(t, ph)
	n := zipsIn(t, f.Opts.BackupDir)
	if len(n) != 1 || !strings.HasSuffix(n[0], "-INCOMPLETE.zip") || f.BackupPath != "" || f.IncompletePath == "" {
		t.Fatalf("want exactly one -INCOMPLETE.zip, got %v (backup %q, incomplete %q)", n, f.BackupPath, f.IncompletePath)
	}
	_, info, sums := readZip(t, f.IncompletePath)
	if info.Complete || len(info.Missing) != 1 || info.Missing[0] != "nvram" || strings.Contains(sums, "nvram") {
		t.Fatalf("info %+v sums %s", info, sums)
	}
}

func TestBackupCriticalFailureContinueYesFlashes(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	ph.corruptReads["nvram"] = 9
	// build, start backup, continue without backup = y, erase = n, FLASH
	f, out := newFlow(t, tp, ph, "KEKA\n\ny\nn\nFLASH\n")
	f.Opts.Backup = true
	if err := f.Execute(); err != nil {
		t.Fatalf("Execute: %v\n%s", err, out)
	}
	assertStockBeforeFlash(t, ph)
	if ph.flashed["super"] != tp.rawSHA {
		t.Fatal("super not flashed")
	}
	assertWritesWithBackup(t, ph)
	if !strings.Contains(out.String(), "INCOMPLETE") {
		t.Fatalf("INCOMPLETE not shown:\n%s", out)
	}
}

func TestBackupFailureContinueWithoutBackupFlagNoPrompt(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	ph.noRoot = true
	// build, start backup, erase = n, FLASH  (no "continue?" question)
	f, out := newFlow(t, tp, ph, "KEKA\n\nn\nFLASH\n")
	f.Opts.Backup = true
	f.Opts.ContinueWithoutBackup = true
	if err := f.Execute(); err != nil {
		t.Fatalf("Execute: %v\n%s", err, out)
	}
	if strings.Contains(out.String(), "continue flashing WITHOUT a backup?") {
		t.Fatal("asked although --continue-without-backup")
	}
	assertStockBeforeFlash(t, ph)
	if ph.flashed["super"] != tp.rawSHA {
		t.Fatal("super not flashed")
	}
	if n := zipsIn(t, f.Opts.BackupDir); len(n) != 0 {
		t.Fatalf("zip kept although nothing was read: %v", n)
	}
}

func TestBackupNoRootAskedNoStopsAndRestores(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	ph.noRoot = true
	f, _ := newFlow(t, tp, ph, "KEKA\n\nn\n")
	f.Opts.Backup = true
	err := f.Execute()
	var r ErrRefused
	if !errors.As(err, &r) || !strings.Contains(r.Msg, "root") {
		t.Fatalf("got %v", err)
	}
	if ph.flashed["recovery"] != StockRecoverySHA256 || ph.flashed["super"] != "" {
		t.Fatalf("flashed %v", ph.flashed)
	}
}

func TestBackupRecovery2WriteFailsStockRestoredThenAsks(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	ph.failDumpRecovery = true
	f, out := newFlow(t, tp, ph, "KEKA\n\ny\nn\nFLASH\n")
	f.Opts.Backup = true
	if err := f.Execute(); err != nil {
		t.Fatalf("Execute: %v\n%s", err, out)
	}
	assertStockBeforeFlash(t, ph)
	if strings.Contains(out.String(), "Press Enter when the phone is OFF") {
		t.Fatal("asked to catch the phone although it never left fastboot")
	}
}

func TestBackupStockRestoreFailsNeverFlashesEvenWithContinueFlag(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	ph.failStockRecovery = true
	f, out := newFlow(t, tp, ph, "KEKA\n\nn\nFLASH\n")
	f.Opts.Backup = true
	f.Opts.ContinueWithoutBackup = true
	err := f.Execute()
	var nr ErrRecoveryNotRestored
	if !errors.As(err, &nr) {
		t.Fatalf("want ErrRecoveryNotRestored, got %v\n%s", err, out)
	}
	assertNothingFlashed(t, ph)
	if !strings.Contains(out.String(), "COULD NOT PUT THE STOCK RECOVERY BACK") {
		t.Fatalf("no warning:\n%s", out)
	}
	// the backup itself was fine and is kept
	if f.BackupPath == "" {
		t.Fatal("good backup not kept")
	}
}

func TestBackupUserSaysNoFlashesWithoutBackup(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	// build, start backup = n, erase = n, FLASH
	f, out := newFlow(t, tp, ph, "KEKA\nn\nn\nFLASH\n")
	f.Opts.Backup = true
	if err := f.Execute(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, ok := ph.flashed["recovery"]; ok {
		t.Fatal("recovery touched although the backup was skipped")
	}
	if ph.flashed["super"] != tp.rawSHA {
		t.Fatal("not flashed")
	}
}

func TestBackupInputClosedAtContinueStops(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	ph.noRoot = true
	f, _ := newFlow(t, tp, ph, "KEKA\n\n")
	f.Opts.Backup = true
	err := f.Execute()
	if !errors.Is(err, ErrInputClosed) {
		t.Fatalf("got %v", err)
	}
	assertNothingFlashed(t, ph)
	if ph.flashed["recovery"] != StockRecoverySHA256 {
		t.Fatal("stock recovery not back")
	}
}

func TestBackupNotEnoughDisk(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	old := freeBytesFn
	freeBytesFn = func(string) (int64, error) { return 100 << 20, nil }
	defer func() { freeBytesFn = old }()
	f, _ := newFlow(t, tp, ph, "KEKA\n\n")
	f.Opts.Backup = true
	err := f.Execute()
	var r ErrRefused
	if !errors.As(err, &r) || !strings.Contains(r.Msg, "free space") || len(writeCalls(ph)) != 0 {
		t.Fatalf("got %v writes %v", err, writeCalls(ph))
	}
}

func TestBackupNeedsRecoveryInPayload(t *testing.T) {
	tp := makePayloadX(t, false, false)
	ph := backupPhone("fastboot", true)
	f, _ := newFlow(t, tp, ph, "KEKA\n\n")
	f.Opts.Backup = true
	err := f.Execute()
	var r ErrRefused
	if !errors.As(err, &r) || !strings.Contains(r.Msg, "--no-backup") || len(writeCalls(ph)) != 0 {
		t.Fatalf("got %v writes %v", err, writeCalls(ph))
	}
}

func TestNoBackupNeverTouchesRecovery(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	f, out := newFlow(t, tp, ph, "KEKA\nn\nFLASH\n")
	f.Opts.Backup = false
	if err := f.Execute(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, ok := ph.flashed["recovery"]; ok {
		t.Fatal("recovery written with --no-backup")
	}
	for _, c := range ph.calls {
		if strings.Contains(c, "reboot recovery") || strings.Contains(c, "exec-out") {
			t.Fatalf("backup step ran: %s", c)
		}
	}
}

func TestFlashRecoveryRefusesUnpinnedImage(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	f, _ := newFlow(t, tp, ph, "")
	old := DumpRecoverySHA256
	DumpRecoverySHA256 = strings.Repeat("0", 64)
	defer func() { DumpRecoverySHA256 = old }()
	if err := f.flashRecovery("dump"); err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("got %v", err)
	}
	if err := f.flashRecovery("boot"); err == nil {
		t.Fatal("unknown kind allowed")
	}
	if len(ph.calls) != 0 {
		t.Fatalf("fastboot called: %v", ph.calls)
	}
	// and the general writer still refuses recovery
	if err := f.flashPartition("recovery", "/dev/null", false); err == nil {
		t.Fatal("flashPartition allowed recovery")
	}
}

func TestRestoreRecoveryOnly(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	f, out := newFlow(t, tp, ph, "KEKA\n")
	f.Opts.RestoreRecovery = true
	if err := f.Execute(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(ph.flashed) != 1 || ph.flashed["recovery"] != StockRecoverySHA256 {
		t.Fatalf("flashed %v", ph.flashed)
	}
}

func TestDryRunShowsBackupPlan(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	f, out := newFlow(t, tp, ph, "")
	f.Opts.DryRun, f.Opts.Backup = true, true
	if err := f.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(ph.calls) != 0 || !strings.Contains(out.String(), "fastboot flash recovery <backup recovery") || !strings.Contains(out.String(), "stock TCL KEKA recovery") {
		t.Fatalf("calls %v\n%s", ph.calls, out)
	}
}

func TestParsePartitionList(t *testing.T) {
	in := "boot /dev/block/mmcblk0p32 25165824\n" +
		"preloader_a /dev/block/mmcblk0boot0 4194304\n" +
		"mmcblk0boot0 /dev/block/mmcblk0boot0 4194304\n" +
		"mmcblk0rpmb /dev/block/mmcblk0rpmb 4194304\n" +
		"bad name /dev/block/x 1\n" +
		"zero /dev/block/mmcblk0p99 0\n" +
		"evil;rm /dev/block/mmcblk0p98 10\n" +
		"super /dev/block/mmcblk0p41 2147483648\n"
	ps := parsePartitionList(in)
	var names []string
	for _, p := range ps {
		names = append(names, p.Name)
	}
	if strings.Join(names, ",") != "boot,mmcblk0boot0,super" {
		t.Fatalf("got %v", names)
	}
	if parseSHA("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  /dev/block/x\n") != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" || parseSHA("sha256sum: no such file") != "" {
		t.Fatal("parseSHA")
	}
}
