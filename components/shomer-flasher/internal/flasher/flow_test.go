package flasher

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Shomer-TCL/shomer-flasher/internal/bundle"
)

// ---------- fake phone ----------

type fakePhone struct {
	mode                   string // off | adb | fastboot | fastbootd
	model, build           string
	product                string
	unlocked               bool
	unlockRefuse           bool
	rebootOnUnlock         bool
	superFailsInBootloader bool
	calls                  []string
	flashed                map[string]string // partition -> sha256 of file
	verityFlags            map[string]bool
	erased                 []string

	// backup-first
	parts                  map[string][]byte // partition contents, readable in the backup recovery
	devs                   map[string]string // partition -> /dev/block/...
	corruptReads           map[string]int    // partition -> how many reads come back damaged
	noRoot                 bool
	noFastbootFromRecovery bool
	failStockRecovery      bool // writing the stock recovery fails
	failDumpRecovery       bool // writing recovery2 fails

	// 1.4.3
	dropByName map[string]bool   // partitions missing from /dev/block/by-name (the phone's list)
	noBootArea bool              // no /dev/block/mmcblk0boot0 node and no preloader line
	getvarAll  string            // extra `(bootloader) key: value` lines for getvar all
	adbErr     map[string]string // getprop name -> adb error text (exit 1)
}

func (p *fakePhone) setupParts() {
	p.parts = map[string][]byte{}
	p.devs = map[string]string{}
	names := []string{"proinfo", "nvram", "nvdata", "nvcfg", "persist", "protect1", "protect2", "seccfg", "md1img", "md1dsp",
		"boot", "vendor_boot", "dtbo", "lk", "lk2", "logo", "vbmeta", "vbmeta_system", "vbmeta_vendor", "tee1", "tee2",
		"super", "cache", "metadata", "recovery", "frp", "otp", "userdata"}
	for i, n := range names {
		b := make([]byte, 4096*(1+i%3))
		rand.Read(b)
		if n == "super" {
			b = make([]byte, 256<<10) // mostly zeros, like the real one
			rand.Read(b[:8192])
		}
		p.parts[n] = b
		p.devs[n] = fmt.Sprintf("/dev/block/mmcblk0p%d", i+1)
	}
	pre := make([]byte, 4096)
	rand.Read(pre)
	p.parts["mmcblk0boot0"] = pre
	p.devs["mmcblk0boot0"] = "/dev/block/mmcblk0boot0"
	p.corruptReads = map[string]int{}
}

func (p *fakePhone) partByDev(dev string) string {
	for n, d := range p.devs {
		if d == dev {
			return n
		}
	}
	return ""
}

// RunStream: adb exec-out "su -c '... dd if=<dev> ...'" writes the partition bytes.
func (p *fakePhone) RunStream(ctx context.Context, tool string, args []string, w io.Writer) (string, error) {
	cmd := tool + " " + strings.Join(args, " ")
	p.calls = append(p.calls, cmd)
	if p.mode != "recovery" || tool != "adb" {
		return "error: no devices/emulators found\n", errors.New("exit status 1")
	}
	i := strings.Index(cmd, "if=")
	dev := strings.Fields(cmd[i+3:])[0]
	n := p.partByDev(dev)
	b := p.parts[n]
	if p.corruptReads[n] > 0 {
		p.corruptReads[n]--
		bad := append([]byte{}, b...)
		bad[len(bad)/2] ^= 0x55
		b = bad[:len(bad)-100] // short and damaged
	}
	w.Write(b)
	return "", nil
}

func (p *fakePhone) Run(ctx context.Context, tool string, args []string, stream io.Writer) (string, error) {
	cmd := tool + " " + strings.Join(args, " ")
	p.calls = append(p.calls, cmd)
	out := p.do(tool, args)
	if stream != nil {
		io.WriteString(stream, out)
	}
	if strings.Contains(out, "FAILED") {
		return out, errors.New("exit status 1")
	}
	return out, nil
}

func (p *fakePhone) do(tool string, a []string) string {
	if tool == "adb" {
		if len(a) >= 2 && a[0] == "-s" {
			a = a[2:]
		}
		switch {
		case len(a) == 1 && a[0] == "devices":
			if p.mode == "adb" {
				return "List of devices attached\nFAKE123\tdevice\n"
			}
			if p.mode == "recovery" {
				return "List of devices attached\nFAKE123\trecovery\n"
			}
			return "List of devices attached\n\n"
		case p.mode == "recovery" && len(a) == 2 && a[0] == "shell" && strings.HasPrefix(a[1], "su -c "):
			c := a[1]
			switch {
			case p.noRoot:
				return "su: not found\n"
			case strings.Contains(c, "'id'"):
				return "uid=0(root) gid=0(root) context=u:r:magisk:s0\n"
			case strings.Contains(c, "blockdev"):
				// like the real phone: by-name never says mmcblk0boot0, at most preloader_a;
				// the flasher's command adds the boot area itself
				var sb strings.Builder
				for n, d := range p.devs {
					if n == "mmcblk0boot0" || p.dropByName[n] {
						continue
					}
					fmt.Fprintf(&sb, "%s %s %d\n", n, d, len(p.parts[n]))
				}
				if !p.noBootArea {
					fmt.Fprintf(&sb, "preloader_a /dev/block/mmcblk0boot0 %d\n", len(p.parts["mmcblk0boot0"]))
					if strings.Contains(c, "[ -b /dev/block/mmcblk0boot0 ]") {
						fmt.Fprintf(&sb, "mmcblk0boot0 /dev/block/mmcblk0boot0 %d\n", len(p.parts["mmcblk0boot0"]))
					}
				}
				sb.WriteString("mmcblk0rpmb /dev/block/mmcblk0rpmb 4194304\nbroken /dev/block/xx notanumber\n")
				return sb.String()
			case strings.Contains(c, "sha256sum "):
				dev := strings.TrimSuffix(strings.Fields(c[strings.Index(c, "sha256sum ")+10:])[0], "'")
				s := sha256.Sum256(p.parts[p.partByDev(dev)])
				return hex.EncodeToString(s[:]) + "  " + dev + "\n"
			}
			return "sh: unknown\n"
		case p.mode == "recovery" && len(a) == 2 && a[0] == "reboot" && a[1] == "bootloader":
			if !p.noFastbootFromRecovery {
				p.mode = "fastboot"
			}
			return ""
		case len(a) >= 3 && a[0] == "shell" && a[1] == "getprop":
			if e, ok := p.adbErr[a[2]]; ok {
				return e + "\nFAILED\n" // Run turns FAILED into exit status 1
			}
			switch a[2] {
			case "ro.bootmode":
				if p.mode == "recovery" {
					return "recovery\n"
				}
				return "normal\n"
			case "ro.product.model":
				return p.model + "\n"
			case "ro.build.version.incremental":
				return p.build + "\n"
			case "ro.build.fingerprint":
				return "TCL/" + p.model + "/Gflip6:11/RP1A.200720.011/" + p.build + ":user/release-keys\n"
			}
		case len(a) >= 3 && a[0] == "shell" && a[1] == "reboot" && a[2] == "-p":
			p.mode = "off"
			return ""
		case len(a) == 1 && a[0] == "kill-server":
			return ""
		}
		return "adb: unknown\n"
	}
	// fastboot
	inFB := p.mode == "fastboot" || p.mode == "fastbootd"
	if len(a) == 1 && a[0] == "devices" {
		if inFB {
			return "FAKE123\t" + map[bool]string{true: "fastbootd", false: "fastboot"}[p.mode == "fastbootd"] + "\n"
		}
		return ""
	}
	if !inFB {
		return "< waiting for any device >\n"
	}
	verity := false
	if len(a) >= 2 && a[0] == "--disable-verity" && a[1] == "--disable-verification" {
		verity = true
		a = a[2:]
	}
	switch {
	case a[0] == "getvar" && a[1] == "product":
		return "product: " + p.product + "\nFinished. Total time: 0.002s\n"
	case a[0] == "getvar" && a[1] == "unlocked":
		return "unlocked: " + map[bool]string{true: "yes", false: "no"}[p.unlocked] + "\nFinished. Total time: 0.002s\n"
	case a[0] == "getvar" && a[1] == "all":
		return "(bootloader) max-download-size: 0x8000000\n" + p.getvarAll + "all: \nFinished.\n"
	case a[0] == "getvar" && a[1] == "is-userspace":
		return "is-userspace: " + map[bool]string{true: "yes", false: "no"}[p.mode == "fastbootd"] + "\n"
	case a[0] == "getvar" && a[1] == "partition-size:recovery":
		return fmt.Sprintf("partition-size:recovery: 0x%x\n", RecoverySize)
	case a[0] == "flashing" && a[1] == "unlock":
		if p.unlockRefuse {
			return "FAILED (remote: 'Unlock operation is not allowed')\nfastboot: error: Command failed\n"
		}
		p.unlocked = true
		if p.rebootOnUnlock {
			p.mode = "off"
		}
		return "OKAY [ 5.000s]\nFinished.\n"
	case a[0] == "flash":
		part, file := a[1], a[2]
		if part == "super" && p.mode == "fastboot" && p.superFailsInBootloader {
			return "Sending sparse 'super' 1/16 (131068 KB)\nFAILED (remote: 'partition not found')\n"
		}
		b, err := os.ReadFile(file)
		if err != nil {
			return "FAILED (cannot read file)\n"
		}
		s := sha256.Sum256(b)
		if part == "recovery" && ((p.failStockRecovery && hex.EncodeToString(s[:]) == StockRecoverySHA256) || (p.failDumpRecovery && hex.EncodeToString(s[:]) == DumpRecoverySHA256)) {
			return "Sending 'recovery' (24576 KB)\nFAILED (remote: 'write error')\n"
		}
		p.flashed[part] = hex.EncodeToString(s[:])
		p.verityFlags[part] = verity
		return "Sending '" + part + "' OKAY\nWriting '" + part + "' OKAY\nFinished.\n"
	case a[0] == "erase":
		p.erased = append(p.erased, a[1])
		return "Erasing '" + a[1] + "' OKAY\n"
	case a[0] == "reboot" && len(a) == 2 && a[1] == "recovery":
		if p.flashed["recovery"] == DumpRecoverySHA256 {
			p.mode = "recovery"
		} else {
			p.mode = "stockrecovery"
		}
		return "Rebooting into recovery OKAY\n"
	case a[0] == "reboot" && len(a) == 2 && a[1] == "fastboot":
		p.mode = "fastbootd"
		return "Rebooting into fastboot OKAY\n"
	case a[0] == "reboot" && len(a) == 2 && a[1] == "bootloader":
		p.mode = "fastboot"
		return "Rebooting into bootloader OKAY\n"
	case a[0] == "reboot":
		p.mode = "booting"
		return "Rebooting OKAY\n"
	}
	return "FAILED (unknown command)\n"
}

type fakePre struct {
	phone *fakePhone
	n     int
}

func (f *fakePre) Ports() []string {
	if f.phone.mode == "off" {
		return []string{"FAKECOM"}
	}
	return nil
}
func (f *fakePre) Handshake(port, seq string, t time.Duration) (string, error) {
	f.n++
	if seq != "FASTBOOT" {
		return "", errors.New("bad seq")
	}
	f.phone.mode = "fastboot"
	return "READYTOO", nil
}

// ---------- fake payload ----------

type testPayload struct {
	path   string
	rawSHA string
	vbSHA  map[string]string
}

// small fake recovery images; their sha256s replace the real pinned ones in tests
var (
	fakeDumpRec  = bytes.Repeat([]byte("DUMPREC2"), 512)
	fakeStockRec = bytes.Repeat([]byte("STOCKREC"), 512)
)

func init() {
	s := sha256.Sum256(fakeDumpRec)
	DumpRecoverySHA256 = hex.EncodeToString(s[:])
	s = sha256.Sum256(fakeStockRec)
	StockRecoverySHA256 = hex.EncodeToString(s[:])
	RecoverySize = int64(len(fakeDumpRec))
}

func makePayload(t *testing.T, corrupt bool) testPayload {
	return makePayloadX(t, corrupt, true)
}

func makePayloadX(t *testing.T, corrupt, withRecovery bool) testPayload {
	t.Helper()
	dir := t.TempDir()
	raw := make([]byte, 6<<20) // 6 MB "super": zeros + random middle
	rand.Read(raw[1<<20 : 3<<20])
	rs := sha256.Sum256(raw)
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write(raw)
	zw.Close()
	write := func(n string, b []byte) string {
		p := filepath.Join(dir, n)
		os.WriteFile(p, b, 0o644)
		return p
	}
	files := map[string]string{
		"tools/linux/fastboot": write("fastboot", []byte("#!/bin/sh\n")),
		"tools/linux/adb":      write("adb", []byte("#!/bin/sh\n")),
		"rom/super.raw.gz":     write("super.raw.gz", gz.Bytes()),
	}
	vb := map[string]string{}
	for _, v := range []string{"vbmeta", "vbmeta_system", "vbmeta_vendor"} {
		b := append([]byte("AVB0"), bytes.Repeat([]byte{byte(len(v))}, 4092)...)
		files["avb/"+v] = write(v, b)
		s := sha256.Sum256(b)
		vb[v] = hex.EncodeToString(s[:])
	}
	order := []string{"tools/linux/fastboot", "tools/linux/adb", "avb/vbmeta", "avb/vbmeta_system", "avb/vbmeta_vendor"}
	if withRecovery {
		files["recovery/dump-recovery2.img"] = write("dump.img", fakeDumpRec)
		files["recovery/stock-recovery-KEKA.img"] = write("stock.img", fakeStockRec)
		order = append(order, "recovery/dump-recovery2.img", "recovery/stock-recovery-KEKA.img")
	}
	order = append(order, "rom/super.raw.gz")
	m := bundle.Manifest{
		Name: "Shomer OS", Build: "test", ROMCommit: "abc", SuperGz: "rom/super.raw.gz",
		SuperRaw: bundle.FileInfo{SHA256: hex.EncodeToString(rs[:]), Size: int64(len(raw))},
		Files:    map[string]bundle.FileInfo{},
		VBMeta:   []string{"vbmeta", "vbmeta_system", "vbmeta_vendor"},
		Products: []string{"gflip6"}, Models: []string{"T408DL", "4058G"}, Builds: []string{"KEKA", "UPCI"},
		Tools: []string{"tools/linux/fastboot", "tools/linux/adb"},
	}
	if withRecovery {
		m.Recovery = &bundle.RecoveryFiles{Dump: "recovery/dump-recovery2.img", Stock: "recovery/stock-recovery-KEKA.img"}
	}
	for n, p := range files {
		b, _ := os.ReadFile(p)
		s := sha256.Sum256(b)
		m.Files[n] = bundle.FileInfo{SHA256: hex.EncodeToString(s[:]), Size: int64(len(b))}
	}
	out := filepath.Join(dir, "flasher")
	o, _ := os.Create(out)
	o.Write([]byte("STUB PROGRAM BYTES"))
	if err := bundle.Write(o, m, files, order); err != nil {
		t.Fatal(err)
	}
	o.Close()
	if corrupt {
		// flip one byte inside super.raw.gz (near the end of the file, before the trailer)
		b, _ := os.ReadFile(out)
		b[len(b)-bundle.TrailerSize-5000] ^= 0xFF
		os.WriteFile(out, b, 0o755)
	}
	return testPayload{path: out, rawSHA: m.SuperRaw.SHA256, vbSHA: vb}
}

func newFlow(t *testing.T, tp testPayload, phone *fakePhone, input string) (*Flow, *bytes.Buffer) {
	t.Helper()
	pl, err := bundle.Open(tp.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pl.Close() })
	var out bytes.Buffer
	if phone.flashed == nil {
		phone.flashed = map[string]string{}
		phone.verityFlags = map[string]bool{}
	}
	f := &Flow{
		UI:            NewUI(strings.NewReader(input), &out, io.Discard),
		Run:           phone,
		Pre:           &fakePre{phone: phone},
		Payload:       pl,
		Opts:          Options{WorkDir: t.TempDir(), BackupDir: t.TempDir()},
		PreloaderWait: 2 * time.Second,
		FastbootWait:  2 * time.Second,
		Sleep:         func(time.Duration) {},
	}
	return f, &out
}

func flashCalls(p *fakePhone) []string {
	var c []string
	for _, x := range p.calls {
		if strings.Contains(x, " flash ") || strings.Contains(x, " erase ") || strings.Contains(x, "flashing unlock") {
			c = append(c, x)
		}
	}
	return c
}

func assertOnlyAllowedWrites(t *testing.T, p *fakePhone) {
	t.Helper()
	for part := range p.flashed {
		if !AllowedPartitions[part] {
			t.Fatalf("wrote forbidden partition %s", part)
		}
	}
	for _, e := range p.erased {
		if e != "userdata" {
			t.Fatalf("erased forbidden partition %s", e)
		}
	}
}

// ---------- tests ----------

func TestExpectedReply(t *testing.T) {
	if got := ExpectedReply("FASTBOOT"); got != "READYTOO" {
		t.Fatalf("got %q", got)
	}
}

func TestLockedPhoneUnlockThenFlash(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "adb", model: "T408DL", build: "KEKA", product: "gflip6"}
	// Enter (phone off), UNLOCK, FLASH
	f, out := newFlow(t, tp, ph, "\nUNLOCK\nFLASH\n")
	if err := f.Execute(); err != nil {
		t.Fatalf("Execute: %v\n%s", err, out)
	}
	if !ph.unlocked {
		t.Fatal("not unlocked")
	}
	if ph.flashed["super"] != tp.rawSHA {
		t.Fatalf("super sha %s != %s", ph.flashed["super"], tp.rawSHA)
	}
	for v, s := range tp.vbSHA {
		if ph.flashed[v] != s || !ph.verityFlags[v] {
			t.Fatalf("%s not flashed with verity off (sha %s flags %v)", v, ph.flashed[v], ph.verityFlags[v])
		}
	}
	if ph.verityFlags["super"] {
		t.Fatal("verity flags on super")
	}
	if len(ph.erased) != 0 {
		t.Fatal("erase after unlock should not be needed")
	}
	want := []string{
		"fastboot flashing unlock",
		"fastboot --disable-verity --disable-verification flash vbmeta",
		"fastboot --disable-verity --disable-verification flash vbmeta_system",
		"fastboot --disable-verity --disable-verification flash vbmeta_vendor",
		"fastboot flash super",
	}
	got := flashCalls(ph)
	if len(got) != len(want) {
		t.Fatalf("write calls %v", got)
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) {
			t.Fatalf("call %d = %q, want prefix %q", i, got[i], want[i])
		}
	}
	if ph.mode != "booting" {
		t.Fatal("no reboot at the end")
	}
	assertOnlyAllowedWrites(t, ph)
	if !strings.Contains(out.String(), "IMEI") {
		t.Fatal("after-flash IMEI instructions missing")
	}
}

func TestUnlockRebootsPhoneCatchAgain(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "off", product: "gflip6", rebootOnUnlock: true}
	// build typed, Enter, UNLOCK, (phone reboots) Enter, FLASH
	f, out := newFlow(t, tp, ph, "UPCI\n\nUNLOCK\n\nFLASH\n")
	if err := f.Execute(); err != nil {
		t.Fatalf("Execute: %v\n%s", err, out)
	}
	if ph.flashed["super"] != tp.rawSHA {
		t.Fatal("super not flashed")
	}
	assertOnlyAllowedWrites(t, ph)
}

func TestAlreadyUnlockedErasesWhenAsked(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "fastboot", product: "gflip6", unlocked: true}
	f, out := newFlow(t, tp, ph, "keka\ny\nFLASH\n")
	if err := f.Execute(); err != nil {
		t.Fatalf("Execute: %v\n%s", err, out)
	}
	for _, c := range ph.calls {
		if strings.Contains(c, "flashing unlock") {
			t.Fatal("unlock run on an unlocked phone")
		}
	}
	if len(ph.erased) != 1 || ph.erased[0] != "userdata" {
		t.Fatalf("erased %v", ph.erased)
	}
	if ph.flashed["super"] != tp.rawSHA {
		t.Fatal("super not flashed")
	}
	assertOnlyAllowedWrites(t, ph)
}

func TestWrongProductRefused(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "fastboot", product: "Gflip3_TMO", unlocked: true}
	f, out := newFlow(t, tp, ph, "KEKA\nFLASH\n")
	err := f.Execute()
	var r ErrRefused
	if !errors.As(err, &r) || !strings.Contains(r.Msg, "Gflip3_TMO") {
		t.Fatalf("want refusal, got %v\n%s", err, out)
	}
	if len(flashCalls(ph)) != 0 {
		t.Fatalf("wrote to phone: %v", flashCalls(ph))
	}
}

func TestWrongModelOverAdbRefused(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "adb", model: "4058W", build: "LF91", product: "gflip6"}
	f, _ := newFlow(t, tp, ph, "UPCI\n\n") // 1.4.3: warning, choice UPCI, Enter = No
	err := f.Execute()
	var r ErrRefused
	if !errors.As(err, &r) || !strings.Contains(r.Msg, "4058W") || !strings.Contains(r.Msg, "stopped by you after the warning") {
		t.Fatalf("want refusal, got %v", err)
	}
	if len(flashCalls(ph)) != 0 || ph.mode != "adb" {
		t.Fatal("phone touched")
	}
}

func TestOldBuildOverAdbRefused(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "adb", model: "T408DL", build: "KEE7", product: "gflip6"}
	f, _ := newFlow(t, tp, ph, "KEKA\nn\n") // 1.4.3: warning, choice KEKA, No
	err := f.Execute()
	var r ErrRefused
	if !errors.As(err, &r) || !strings.Contains(r.Msg, "KEE7") {
		t.Fatalf("want refusal, got %v", err)
	}
}

func TestTypedBuildWrongRefused(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "off", product: "gflip6"}
	f, _ := newFlow(t, tp, ph, "UPBO\nKEE7\nxx\nq\n") // 1.4.3: after the warning, Q stops
	err := f.Execute()
	var r ErrRefused
	if !errors.As(err, &r) {
		t.Fatalf("want refusal, got %v", err)
	}
	if len(ph.calls) > 0 && len(flashCalls(ph)) != 0 {
		t.Fatal("wrote to phone")
	}
}

func TestBadChecksumRefusedBeforePhone(t *testing.T) {
	tp := makePayload(t, true)
	ph := &fakePhone{mode: "fastboot", product: "gflip6", unlocked: true}
	f, out := newFlow(t, tp, ph, "KEKA\ny\nFLASH\n")
	err := f.Execute()
	var r ErrRefused
	if !errors.As(err, &r) || !strings.Contains(r.Msg, "CHECKSUM MISMATCH") {
		t.Fatalf("want checksum refusal, got %v\n%s", err, out)
	}
	if len(ph.calls) != 0 {
		t.Fatalf("phone was contacted: %v", ph.calls)
	}
}

func TestUnlockNotAllowed(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "fastboot", product: "gflip6", unlockRefuse: true}
	f, out := newFlow(t, tp, ph, "KEKA\nUNLOCK\n")
	err := f.Execute()
	var r ErrRefused
	if !errors.As(err, &r) || !strings.Contains(out.String(), "OEM unlocking") {
		t.Fatalf("want refusal with OEM help, got %v\n%s", err, out)
	}
	if len(ph.flashed) != 0 {
		t.Fatal("flashed")
	}
}

func TestUserDeclinesUnlock(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "fastboot", product: "gflip6"}
	f, _ := newFlow(t, tp, ph, "KEKA\nno\n")
	err := f.Execute()
	var r ErrRefused
	if !errors.As(err, &r) || ph.unlocked || len(flashCalls(ph)) != 0 {
		t.Fatalf("got %v unlocked=%v", err, ph.unlocked)
	}
}

func TestUserDeclinesFlash(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "fastboot", product: "gflip6", unlocked: true}
	f, _ := newFlow(t, tp, ph, "KEKA\ny\nnope\n")
	err := f.Execute()
	var r ErrRefused
	if !errors.As(err, &r) || len(ph.flashed) != 0 {
		t.Fatalf("got %v flashed=%v", err, ph.flashed)
	}
}

func TestSuperFallsBackToFastbootd(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "fastboot", product: "gflip6", unlocked: true, superFailsInBootloader: true}
	f, out := newFlow(t, tp, ph, "KEKA\nn\nFLASH\n")
	if err := f.Execute(); err != nil {
		t.Fatalf("Execute: %v\n%s", err, out)
	}
	if ph.flashed["super"] != tp.rawSHA || !f.inFastbootd {
		t.Fatal("fastbootd fallback did not flash super")
	}
	assertOnlyAllowedWrites(t, ph)
}

func TestFlashPartitionAllowlist(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "fastboot"}
	f, _ := newFlow(t, tp, ph, "")
	for _, p := range []string{"boot", "recovery", "nvram", "nvdata", "proinfo", "md1img", "md1dsp", "lk", "preloader", "userdata", "vendor", "system"} {
		if err := f.flashPartition(p, "/dev/null", false); err == nil {
			t.Fatalf("%s was allowed", p)
		}
	}
	if err := f.flashPartition("super", "/dev/null", true); err == nil {
		t.Fatal("verity flags allowed on super")
	}
	if len(ph.calls) != 0 {
		t.Fatalf("fastboot called: %v", ph.calls)
	}
}

func TestDryRunTouchesNothing(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "fastboot", product: "gflip6"}
	f, out := newFlow(t, tp, ph, "")
	f.Opts.DryRun = true
	if err := f.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(ph.calls) != 0 {
		t.Fatalf("calls %v", ph.calls)
	}
	if !strings.Contains(out.String(), "flash super") {
		t.Fatal("plan not printed")
	}
}

func TestParsers(t *testing.T) {
	if s := parseUnlocked("unlocked: yes\nFinished.", ""); s != Unlocked {
		t.Fatal(s)
	}
	if s := parseUnlocked("unlocked: no\n", ""); s != Locked {
		t.Fatal(s)
	}
	if s := parseUnlocked("", "(bootloader) Device unlocked: false\n"); s != Locked {
		t.Fatal(s)
	}
	if s := parseUnlocked("(bootloader) unlocked: yes", ""); s != Unlocked {
		t.Fatal(s)
	}
	if v, _ := parseGetvar("product: gflip6\nFinished. Total time: 0.001s", "product"); v != "gflip6" {
		t.Fatal(v)
	}
	ds := parseFastbootDevices("0123456789ABCDEF\tfastboot\n")
	if len(ds) != 1 || ds[0].State != "fastboot" {
		t.Fatal(ds)
	}
	ds = parseFastbootDevices("0123\tno permissions; see [http://developer.android.com/tools/device.html]\n")
	if len(ds) != 1 || ds[0].State != "no-permissions" {
		t.Fatal(ds)
	}
	ds = parseAdbDevices("* daemon not running; starting now at tcp:5037\n* daemon started successfully\nList of devices attached\nABC\tunauthorized\n\n")
	if len(ds) != 1 || ds[0].State != "unauthorized" {
		t.Fatal(ds)
	}
}

var _ = fmt.Sprint
