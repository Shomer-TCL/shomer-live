package flasher

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Shomer-TCL/shomer-flasher/internal/bundle"
)

// AllowedPartitions is the complete list of partitions this program will ever
// write. Anything else is refused in code (see flashPartition). modem, nvram,
// nvdata, proinfo (IMEI), boot, recovery, lk, preloader are NEVER written.
var AllowedPartitions = map[string]bool{
	"super":         true,
	"vbmeta":        true,
	"vbmeta_system": true,
	"vbmeta_vendor": true,
}

// Options are command-line choices.
type Options struct {
	WorkDir      string // where super.raw is unpacked; default: next to the program
	KeepWorkDir  bool
	DryRun       bool // verify + unpack + show plan, never touch the phone
	SkipBuildAsk bool // tests only: treat the build as confirmed

	Backup          bool   // backup-first: copy every partition to a zip on this computer before flashing
	BackupDir       string // where the backup zip goes
	BackupUserdata  bool   // also copy userdata (only possible if the bootloader was already unlocked)
	RestoreRecovery bool   // only write the stock recovery back (after an interrupted backup), then stop

	// ContinueWithoutBackup: if the backup fails, flash anyway without asking
	// (unattended runs). The stock recovery is still put back first.
	ContinueWithoutBackup bool
	// Preconfirmed: the TUI/GUI confirm screen already showed every write and
	// the person said yes, so the console's typed UNLOCK / FLASH / "start the
	// backup?" questions are skipped. EraseData is then the person's choice.
	Preconfirmed bool
	EraseData    bool

	// Variant: the software version given on the command line (--variant
	// KEKA or UPCI). It answers the "which version?" question in advance.
	Variant string
	// ForceVariant (--force-variant): when a model/variant/version check warns,
	// continue with the person's own KEKA/UPCI choice without asking. The
	// warning is still shown and logged. No other safety check is skipped.
	ForceVariant bool
}

// Flow is one run of the flasher.
type Flow struct {
	UI      Frontend
	Run     Runner
	Pre     Preloader
	Payload *bundle.Payload
	Opts    Options
	ToolDir string

	// timing knobs (tests shorten them)
	PreloaderWait time.Duration
	FastbootWait  time.Duration
	Sleep         func(time.Duration)

	superRaw        string
	wiped           bool
	inFastbootd     bool
	permissionHint  bool
	buildOK         string
	product         string
	recSerial       string // adb serial while the phone is in the backup recovery
	dumpRecoveryOn  bool   // recovery has been written (or a write was tried): stock must go back before anything else
	BackupPath      string // the finished backup zip
	IncompletePath  string // an INCOMPLETE backup zip that was kept after a failed backup
	recoveryWritten bool   // the backup step touched recovery (and put stock back)
	model           string // ro.product.model if adb could read it
	detectedBuild   string
	lastTyped       string   // the last version answer the person typed (shown in the warning)
	variantWarned   []string // model/variant/version warnings the person chose to go past (shown at the end, logged)
	buildSource     string   // where buildOK came from: "adb" (read from the phone), "typed" (the person, or --variant), "skipped" (tests)
	userdataSaved   bool     // userdata was copied and checked into the backup zip
	phoneProps      []string // build fingerprints/versions found in the phone's own super while it was backed up
	phone           PhoneInfo
}

func (f *Flow) sleep(d time.Duration) {
	if f.Sleep != nil {
		f.Sleep(d)
		return
	}
	time.Sleep(d)
}

// ErrRefused means the program stopped on purpose (wrong phone, bad checksum,
// person said no). Nothing was written to the phone unless the message says so.
type ErrRefused struct{ Msg string }

func (e ErrRefused) Error() string { return e.Msg }

func refuse(format string, a ...any) error { return ErrRefused{Msg: fmt.Sprintf(format, a...)} }

// ---------- payload ----------

func (f *Flow) verifyPayload() error {
	m := f.Payload.Manifest
	f.UI.Step("Checking the ROM inside this program (%s, build %s)", m.Name, m.Build)
	err := f.Payload.Verify(func(d, t int64) { f.UI.Progress("checking", d, t) })
	if err != nil {
		return refuse("%v", err)
	}
	f.UI.Say("    OK - every file matches its SHA-256 checksum.")
	return nil
}

func (f *Flow) unpackSuper() error {
	m := f.Payload.Manifest
	dir := f.Opts.WorkDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create work folder %s: %v", dir, err)
	}
	need := m.SuperRaw.Size + 200<<20
	if free, err := freeBytes(dir); err == nil && free < need {
		return refuse("not enough free disk space in %s: need %d MB, have %d MB. Free some space or run with --workdir <folder on a bigger disk>", dir, need>>20, free>>20)
	}
	f.UI.Step("Unpacking the ROM to %s (about %d MB, takes 1-3 minutes)", dir, m.SuperRaw.Size>>20)
	r, err := f.Payload.Reader(m.SuperGz)
	if err != nil {
		return err
	}
	zr, err := gzip.NewReader(r)
	if err != nil {
		return refuse("ROM inside the program is damaged: %v", err)
	}
	out := filepath.Join(dir, "super.raw")
	w, err := os.Create(out)
	if err != nil {
		return err
	}
	h := sha256.New()
	buf := make([]byte, 1<<20)
	var done int64
	zero := make([]byte, len(buf))
	for {
		n, rerr := io.ReadFull(zr, buf)
		if n > 0 {
			h.Write(buf[:n])
			// leave all-zero blocks as holes where the filesystem supports it
			if string(buf[:n]) == string(zero[:n]) {
				if _, err := w.Seek(int64(n), io.SeekCurrent); err != nil {
					w.Close()
					return err
				}
			} else if _, err := w.Write(buf[:n]); err != nil {
				w.Close()
				return fmt.Errorf("writing %s failed: %v (disk full?)", out, err)
			}
			done += int64(n)
			f.UI.Progress("unpacking", done, m.SuperRaw.Size)
		}
		if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
			break
		}
		if rerr != nil {
			w.Close()
			return refuse("ROM inside the program is damaged: %v", rerr)
		}
	}
	if err := w.Truncate(done); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if done != m.SuperRaw.Size || got != m.SuperRaw.SHA256 {
		os.Remove(out)
		return refuse("CHECKSUM MISMATCH after unpacking: got %s (%d bytes), expected %s (%d bytes). Nothing was written to the phone. Download the flasher again", got, done, m.SuperRaw.SHA256, m.SuperRaw.Size)
	}
	f.UI.Say("    OK - unpacked ROM matches its SHA-256 (%s...).", got[:16])
	f.superRaw = out
	return nil
}

// ---------- tool helpers ----------

func (f *Flow) fastboot(timeout time.Duration, stream bool, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var sw io.Writer
	if stream {
		sw = &uiWriter{f.UI}
	}
	f.UI.Log("run: fastboot %s", strings.Join(args, " "))
	out, err := f.Run.Run(ctx, "fastboot", args, sw)
	f.UI.Log("fastboot output:\n%s", out)
	if err != nil {
		f.UI.Log("fastboot error: %v", err)
	}
	return out, err
}

func (f *Flow) adb(timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	f.UI.Log("run: adb %s", strings.Join(args, " "))
	out, err := f.Run.Run(ctx, "adb", args, nil)
	f.UI.Log("adb output:\n%s", out)
	return out, err
}

type uiWriter struct{ u Frontend }

func (w *uiWriter) Write(b []byte) (int, error) {
	w.u.Raw(string(b))
	return len(b), nil
}

func (f *Flow) event(kind EventKind, step, detail string) {
	f.UI.Event(Event{Kind: kind, Step: step, Detail: detail})
}

func (f *Flow) fastbootDevices() []Device {
	out, _ := f.fastboot(15*time.Second, false, "devices")
	return parseFastbootDevices(out)
}

func (f *Flow) adbDevices() []Device {
	out, _ := f.adb(20*time.Second, "devices")
	return parseAdbDevices(out)
}

// waitFastboot waits until exactly one fastboot device is present.
func (f *Flow) waitFastboot(d time.Duration) (bool, error) {
	deadline := time.Now().Add(d)
	for {
		ds := f.fastbootDevices()
		if len(ds) > 1 {
			return false, refuse("more than one phone is in fastboot mode. Unplug every phone except the one to flash, then run again")
		}
		if len(ds) == 1 {
			if ds[0].State == "no-permissions" {
				f.permissionHint = true
				f.printPermissionHelp()
				return false, refuse("this computer user is not allowed to talk to the phone over USB (see the help above)")
			}
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		f.sleep(time.Second)
	}
}

func (f *Flow) printPermissionHelp() {
	if runtime.GOOS == "windows" {
		f.UI.Say("    Windows could not open the phone. Install the MediaTek USB drivers (see README, step 1) and try again.")
		return
	}
	f.UI.Say("    Linux: run this program with sudo, or install the udev rule from the README:")
	f.UI.Say(`      echo 'SUBSYSTEM=="usb", ATTR{idVendor}=="0e8d", MODE="0666"' | sudo tee /etc/udev/rules.d/51-shomer-flip.rules`)
	f.UI.Say(`      echo 'SUBSYSTEM=="usb", ATTR{idVendor}=="18d1", MODE="0666"' | sudo tee -a /etc/udev/rules.d/51-shomer-flip.rules`)
	f.UI.Say("      sudo udevadm control --reload-rules && sudo udevadm trigger")
	f.UI.Say("    and add yourself to the serial group:  sudo usermod -aG dialout $USER  (log out and in again).")
}

// ---------- build check ----------

func containsAnyFold(s string, list []string) bool {
	for _, x := range list {
		if x != "" && strings.Contains(strings.ToLower(s), strings.ToLower(x)) {
			return true
		}
	}
	return false
}

// checkBuildViaAdb reads the phone's model and build over adb.
// Returns (decided, error). decided=false means adb could not tell.
func (f *Flow) checkBuildViaAdb(serial string) (bool, error) {
	m := f.Payload.Manifest
	var lost string
	get := func(prop string) string {
		out, err := f.adb(20*time.Second, "-s", serial, "shell", "getprop", prop)
		out = strings.TrimSpace(out)
		if err != nil || adbErrorText(out) {
			// an adb error is never a phone answer: never show it as the model
			if lost == "" {
				lost = firstLine(out)
				if lost == "" && err != nil {
					lost = err.Error()
				}
			}
			return ""
		}
		return out
	}
	model := get("ro.product.model")
	inc := get("ro.build.version.incremental")
	fp := get("ro.build.fingerprint")
	if lost != "" {
		return true, refuse("lost the connection to the phone over USB while reading its model (adb said: %s). Check the cable (a data cable, straight into the computer), then run this program again. Nothing was changed on the phone", lost)
	}
	if model == "" && inc == "" {
		return false, nil
	}
	f.model = model
	f.detectedBuild = inc
	f.UI.Say("    Phone says: model %q, build %q", model, inc)
	f.UI.Log("fingerprint %s", fp)
	if !containsAnyFold(model, m.Models) {
		// Sam (Nov 4): a model/version check never blocks; it warns and asks.
		if err := f.overrideVariant(fmt.Sprintf("this phone says its model is %q. This ROM is made for: %s.", model, strings.Join(m.Models, ", ")),
			nil, "Nothing was changed on the phone"); err != nil {
			return true, err
		}
		return true, nil
	}
	if !containsAnyFold(inc, m.Builds) && !containsAnyFold(fp, m.Builds) {
		if err := f.overrideVariant(fmt.Sprintf("this phone says its software is %q. This ROM is made for software %s.", inc, strings.Join(m.Builds, " or ")),
			[]string{"On an older T408DL, the safe way is to first install TCL's own software update",
				"(Settings > System update) until it shows KEKA, then run this again."},
			"Nothing was changed on the phone"); err != nil {
			return true, err
		}
		return true, nil
	}
	f.buildOK = inc
	f.buildSource = "adb"
	f.UI.Say("    OK - supported phone and software.")
	return true, nil
}

// adbErrorText: adb printed one of its own errors instead of a phone's answer.
func adbErrorText(s string) bool {
	lo := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(lo, "error:") || strings.HasPrefix(lo, "adb:") || strings.HasPrefix(lo, "* daemon") ||
		strings.Contains(lo, "device offline") || strings.Contains(lo, "no devices/emulators found") ||
		(strings.Contains(lo, "device '") && strings.Contains(lo, "not found"))
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if len(s) > 120 {
		s = s[:120] + "..."
	}
	return s
}

// hebrewLayout maps the letters of a standard Israeli (SI-1452) keyboard to
// the Latin key they sit on, so "UPCI" typed with the Hebrew layout still on
// ("ופבן") is understood.
var hebrewLayout = map[rune]rune{
	'ש': 'A', 'נ': 'B', 'ב': 'C', 'ג': 'D', 'ק': 'E', 'כ': 'F', 'ע': 'G', 'י': 'H', 'ן': 'I', 'ח': 'J',
	'ל': 'K', 'ך': 'L', 'צ': 'M', 'מ': 'N', 'ם': 'O', 'פ': 'P', '/': 'Q', 'ר': 'R', 'ד': 'S', 'א': 'T',
	'ו': 'U', 'ה': 'V', '\'': 'W', 'ס': 'X', 'ט': 'Y', 'ז': 'Z',
}

// ParseBuild reads the software version the person typed. It ignores capital
// or small letters, spaces and quotes around it, accepts the code inside a
// longer build number (e.g. "4058G_UPCI_05"), and letters typed with the
// Hebrew keyboard layout. It returns the matching entry of builds, or false.
func ParseBuild(typed string, builds []string) (string, bool) {
	s := strings.TrimSpace(typed)
	s = strings.Trim(s, "\"'`“”‘’.,;: \t")
	if s == "" {
		return "", false
	}
	try := func(s string) (string, bool) {
		u := strings.ToUpper(s)
		for _, b := range builds {
			if b != "" && u == strings.ToUpper(b) {
				return b, true
			}
		}
		// a longer build number: exactly one supported code as a whole word in it
		words := strings.FieldsFunc(u, func(r rune) bool { return !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') })
		found := ""
		for _, w := range words {
			for _, b := range builds {
				if b != "" && w == strings.ToUpper(b) {
					if found != "" && found != b {
						return "", false
					}
					found = b
				}
			}
		}
		return found, found != ""
	}
	if b, ok := try(s); ok {
		return b, true
	}
	var lat []rune
	changed := false
	for _, r := range s {
		if l, ok := hebrewLayout[r]; ok {
			lat = append(lat, l)
			changed = true
		} else {
			lat = append(lat, r)
		}
	}
	if changed {
		return try(string(lat))
	}
	return "", false
}

// isQuit: the person typed Q (or quit) to stop.
func isQuit(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "q" || s == "quit" || s == "/" // "/" is Q on the Hebrew layout
}

func (f *Flow) askBuild() error {
	m := f.Payload.Manifest
	if f.Opts.SkipBuildAsk {
		f.buildOK = "skipped"
		f.buildSource = "skipped"
		return nil
	}
	if ok, err := f.variantFromFlag(); ok || err != nil {
		return err
	}
	list := strings.Join(m.Builds, " or ")
	f.UI.Say("")
	f.UI.Say("    I could not read the phone's software version myself, so please check it on the phone:")
	f.UI.Say("      Settings > About phone > Build number (or Software version).")
	f.UI.Say("      On a supported phone it contains %s.", list)
	for i := 0; i < 3; i++ {
		ans, err := f.UI.Ask("    Type the 4 letters you see (" + strings.Join(m.Builds, "/") + "), or Q to quit:")
		if err != nil {
			return err
		}
		if isQuit(ans) {
			return refuse("stopped by you. Nothing was changed on the phone")
		}
		f.lastTyped = strings.TrimSpace(ans)
		if b, ok := ParseBuild(ans, m.Builds); ok {
			f.buildOK = b
			f.buildSource = "typed"
			f.UI.Say("    OK - %s.", b)
			return nil
		}
		if strings.TrimSpace(ans) == "" {
			f.UI.Say("    Nothing was typed. Accepted answers: %s (capital or small letters).", strings.Join(m.Builds, ", "))
		} else {
			f.UI.Say("    %q is not a supported software version. Accepted answers: %s (capital or small letters). This ROM is only for %s.", strings.TrimSpace(ans), strings.Join(m.Builds, ", "), list)
		}
		if i < 2 {
			f.UI.Say("    Please try again (%d of 3 tries left).", 2-i)
		}
	}
	// three answers that are not a supported version: warn and let the person choose (never block)
	return f.overrideVariant(fmt.Sprintf("the software version you typed (%q) is not one this ROM knows. This ROM is made for %s.", f.lastTyped, list),
		nil, "Nothing was changed on the phone")
}

// ---------- the run ----------

// Execute runs the whole flow on the console: Prepare, Detect, WritePhone.
func (f *Flow) Execute() error {
	if err := f.Prepare(); err != nil {
		return err
	}
	if f.Opts.DryRun {
		return nil
	}
	if _, err := f.Detect(); err != nil {
		return err
	}
	return f.WritePhone()
}

// Prepare checks the ROM inside the program and unpacks it. With DryRun it
// also prints the plan. Nothing touches a phone.
func (f *Flow) Prepare() error {
	if f.PreloaderWait == 0 {
		f.PreloaderWait = 3 * time.Minute
	}
	if f.FastbootWait == 0 {
		f.FastbootWait = 60 * time.Second
	}
	m := f.Payload.Manifest

	f.UI.Say("Shomer OS flasher - %s build %s (ROM commit %s)", m.Name, m.Build, m.ROMCommit)
	f.UI.Say("For: TCL Flip 2 T408DL with software KEKA, and TCL 4058G with software UPCI (same phone).")
	f.UI.Say("This program writes ONLY these partitions: super (the ROM) and vbmeta, vbmeta_system, vbmeta_vendor")
	f.UI.Say("(stock TCL KEKA files with verification switched off). It never writes IMEI, modem, nvram or boot.")
	if f.Opts.Backup {
		f.UI.Say("Backup is ON: before flashing, every partition is READ into a zip on this computer. For that the")
		f.UI.Say("recovery partition is written twice: a reading recovery first, then the stock TCL recovery back.")
		f.UI.Say("If the backup fails you are asked whether to flash without it (--no-backup skips it).")
	} else {
		f.UI.Say("Backup is OFF (--no-backup): recovery is not touched.")
	}

	if err := f.verifyPayload(); err != nil {
		return err
	}
	if err := f.unpackSuper(); err != nil {
		return err
	}
	if f.Opts.DryRun {
		f.UI.Step("Dry run: the ROM is fine. These commands WOULD run (nothing is sent to a phone):")
		for _, l := range f.plan(true) {
			f.UI.Say("    %s", l)
		}
	}
	return nil
}

// Detect finds the phone, gets it into fastboot mode and identifies it.
// Nothing is written.
func (f *Flow) Detect() (PhoneInfo, error) {
	if f.PreloaderWait == 0 {
		f.PreloaderWait = 3 * time.Minute
	}
	if f.FastbootWait == 0 {
		f.FastbootWait = 60 * time.Second
	}
	f.UI.Step("Looking for the phone")
	inFastboot := false
	if ds := f.fastbootDevices(); len(ds) > 0 {
		if _, err := f.waitFastboot(time.Second); err != nil {
			return PhoneInfo{}, err
		}
		f.UI.Say("    The phone is already in fastboot mode.")
		inFastboot = true
		if err := f.askBuild(); err != nil {
			return PhoneInfo{}, err
		}
	} else {
		ads := f.adbDevices()
		var ready []Device
		for _, d := range ads {
			if d.State == "device" {
				ready = append(ready, d)
			} else if d.State == "unauthorized" {
				f.UI.Say("    A phone is connected but did not allow USB debugging yet. Open the phone, choose ALLOW on the 'Allow USB debugging?' box, then run this program again.")
				return PhoneInfo{}, refuse("USB debugging not allowed on the phone yet")
			}
		}
		if len(ready) > 1 {
			return PhoneInfo{}, refuse("more than one phone is connected. Unplug every phone except the one to flash")
		}
		if len(ready) == 1 {
			decided, err := f.checkBuildViaAdb(ready[0].Serial)
			if err != nil {
				return PhoneInfo{}, err
			}
			if !decided {
				if err := f.askBuild(); err != nil {
					return PhoneInfo{}, err
				}
			}
			f.UI.Say("")
			f.UI.Say("    This phone cannot be sent to fastboot mode with a command; it has to be caught while it powers up.")
			f.UI.Say("    Turning the phone OFF now...")
			f.adb(20*time.Second, "-s", ready[0].Serial, "shell", "reboot", "-p")
		} else {
			if err := f.askBuild(); err != nil {
				return PhoneInfo{}, err
			}
		}
	}

	if !inFastboot {
		if err := f.catchFastboot(); err != nil {
			return PhoneInfo{}, err
		}
	}

	if err := f.checkProduct(); err != nil {
		return PhoneInfo{}, err
	}
	state := f.readUnlock()
	f.UI.Say("    Bootloader is %s.", state)
	serial := ""
	if ds := f.fastbootDevices(); len(ds) == 1 {
		serial = ds[0].Serial
	}
	f.phone = PhoneInfo{Serial: serial, Product: f.product, Model: f.model, Build: f.buildOK, Unlock: state, Mode: f.fastbootMode(), BuildSource: f.buildSource, DetectedBuild: f.detectedBuild}
	ph := f.phone
	f.UI.Event(Event{Kind: EvPhone, Phone: &ph})
	return f.phone, nil
}

// WritePhone writes the phone: unlock if needed, backup (if on), then the ROM.
// Detect must have run first.
func (f *Flow) WritePhone() error {
	if err := f.validateBackupDestination(); err != nil {
		return err
	}
	// 4. unlock if needed
	if f.phone.Unlock != Unlocked {
		f.event(EvStepStart, StepUnlock, "")
		if err := f.unlock(); err != nil {
			f.event(EvStepFailed, StepUnlock, err.Error())
			return err
		}
		f.event(EvStepDone, StepUnlock, "")
	}

	// 4b. only put the stock recovery back (after an interrupted backup)
	if f.Opts.RestoreRecovery {
		if !f.backupAvailable() {
			return refuse("this flasher has no stock recovery inside. Nothing was written")
		}
		f.UI.Step("Writing the stock TCL recovery back (nothing else)")
		if err := f.flashRecovery("stock"); err != nil {
			return f.writeFailed(err, false)
		}
		if out, rerr := f.fastboot(60*time.Second, true, "reboot"); rerr != nil || fastbootFailed(out) {
			return fmt.Errorf("stock recovery restored, but reboot request failed: %s", lastLines(out, 4))
		}
		f.UI.Say("    OK - stock recovery written. The phone is restarting. Nothing else was changed.")
		return nil
	}

	// 4c. backup-first: copy the whole phone to this computer before writing anything else.
	// Default on, but not blocking: if it fails the person decides (or --continue-without-backup).
	if f.Opts.Backup {
		if err := f.backupStep(); err != nil {
			return err
		}
	}
	if f.dumpRecoveryOn {
		// never reached: backupStep only returns nil once stock recovery is back
		return fmt.Errorf("internal safety stop: the stock recovery is not back on the phone, so nothing else is written")
	}

	// 4d. the phone's own software, read out of super during the backup
	if err := f.verifyVariant(); err != nil {
		return err
	}

	// 5. data wipe choice (unlock already wiped)
	eraseData := false
	userdataLost := f.Opts.Backup && f.Opts.BackupUserdata && !f.wiped &&
		!(f.userdataSaved && (f.BackupPath != "" || f.IncompletePath != ""))
	if !f.wiped && userdataLost {
		// asked to save the phone data, and it is not saved: erasing it needs an explicit yes,
		// whatever was chosen before (also with --continue-without-backup and on the confirm screen)
		f.UI.Warn("PHONE DATA WAS NOT SAVED.",
			"You asked to include the phone data (userdata) in the backup, but it is not in a backup file.",
			"Erasing it now would lose it for good.")
		yes, err := f.UI.YesNo("    Erase the phone data anyway? (n = stop here: the data stays on the phone, Shomer is not written)", false)
		if err != nil {
			return err
		}
		if !yes {
			return refuse("stopped by you: the phone data was not saved, so nothing was erased and Shomer was not written")
		}
		eraseData = true
	} else if !f.wiped {
		if f.Opts.Preconfirmed {
			eraseData = f.Opts.EraseData
		} else {
			f.UI.Say("")
			f.UI.Say("    The bootloader was already unlocked, so the phone still has its old data.")
			f.UI.Say("    A clean start is strongly recommended (old apps' data can stop Shomer from starting).")
			yes, err := f.UI.YesNo("    Erase all data on the phone (contacts, messages, photos)?", true)
			if err != nil {
				return err
			}
			eraseData = yes
		}
	}

	// 6. last confirmation
	f.UI.Step("Ready to write the ROM")
	for _, l := range f.plan(eraseData) {
		f.UI.Say("    %s", l)
	}
	if !f.Opts.Preconfirmed {
		f.UI.Say("    Takes about 5-15 minutes. Do NOT unplug the cable or close this window while it runs.")
		ok, err := f.UI.Confirm("    Type FLASH and press Enter to start (anything else stops):", "FLASH")
		if err != nil {
			return err
		}
		if !ok {
			return refuse("stopped by you before writing. The phone was not flashed")
		}
	}

	// 7. write
	if err := f.flashAll(eraseData); err != nil {
		return err
	}

	// 8. reboot
	f.event(EvStepStart, StepReboot, "")
	f.UI.Step("Restarting the phone")
	if out, rerr := f.fastboot(60*time.Second, true, "reboot"); rerr != nil || fastbootFailed(out) {
		f.event(EvStepFailed, StepReboot, lastLines(out, 4))
		return fmt.Errorf("partitions written, but reboot request failed: %s", lastLines(out, 4))
	}
	f.event(EvStepDone, StepReboot, "")
	f.printAfter()
	f.event(EvFinished, "", "")
	return nil
}

func (f *Flow) plan(eraseData bool) []string {
	var l []string
	steps := PlanSteps(&f.Payload.Manifest, true, Plan{Backup: f.Opts.DryRun && f.Opts.Backup, BackupDir: f.Opts.BackupDir, EraseData: eraseData})
	for _, st := range steps {
		l = append(l, st.Commands...)
	}
	return l
}

// catchFastboot guides the person through power-off + plug-in and catches the preloader.
func (f *Flow) catchFastboot() error {
	for attempt := 1; ; attempt++ {
		f.UI.Step("Getting the phone into FASTBOOT mode (try %d)", attempt)
		f.UI.Say("    1. UNPLUG the USB cable from the phone.")
		f.UI.Say("    2. Make sure the phone is completely OFF: close the flip, hold the red End key until it")
		f.UI.Say("       turns off - or simply take the battery out and put it back in. Wait 5 seconds.")
		if err := f.UI.Enter("    Press Enter when the phone is OFF and UNPLUGGED..."); err != nil {
			return err
		}
		f.UI.Say("    3. Now PLUG the USB cable into the phone (keep it off). I am watching for it...")
		err := f.enterFastbootViaPreloader(f.PreloaderWait)
		if err == nil {
			f.UI.Say("    Waiting for fastboot (the phone screen should show '=> FASTBOOT mode...')")
			ok, err := f.waitFastboot(f.FastbootWait)
			if err != nil {
				return err
			}
			if ok {
				f.UI.Say("    OK - phone is in fastboot mode.")
				return nil
			}
			f.UI.Say("    The phone answered but no fastboot device showed up.")
			if runtime.GOOS == "windows" {
				f.UI.Say("    On Windows this is almost always the driver: install the MediaTek USB drivers (README step 1).")
			} else {
				f.printPermissionHelp()
			}
		} else {
			f.UI.Say("")
			f.UI.Say("    I did not catch the phone. Things to check:")
			if runtime.GOOS == "windows" {
				f.UI.Say("    - MediaTek USB drivers installed (README step 1). Device Manager should flash a 'MediaTek PreLoader USB VCOM' or 'USB Serial Device' for 2-3 s when you plug in.")
			} else {
				f.UI.Say("    - Run with sudo, or add yourself to the dialout group (README).")
				f.UI.Say("    - ModemManager can grab the port: sudo systemctl stop ModemManager   (start it again after).")
			}
			f.UI.Say("    - Use a USB data cable (not charge-only), straight into the computer (no hub).")
			f.UI.Say("    - The phone must be fully OFF before you plug in. Taking the battery out is the surest way.")
			if f.permissionHint {
				f.printPermissionHelp()
			}
		}
		again, err := f.UI.YesNo("    Try again?", true)
		if err != nil {
			return err
		}
		if !again {
			return refuse("could not get the phone into fastboot mode. Nothing was written to the phone")
		}
	}
}

func (f *Flow) checkProduct() error {
	out, _ := f.fastboot(20*time.Second, false, "getvar", "product")
	prod, ok := parseGetvar(out, "product")
	all, _ := f.fastboot(60*time.Second, false, "getvar", "all")
	products := strings.Join(f.Payload.Manifest.Products, ", ")
	if !ok || prod == "" {
		// Sam (Nov 4): never block on what the phone says it is; warn and ask
		return f.overrideVariant("could not detect what this phone is: fastboot getvar product gave no answer. This ROM is made for product "+products+" (TCL Flip 2 T408DL KEKA / 4058G UPCI).",
			nil, "Nothing was written to the phone")
	}
	f.UI.Say("    Phone reports product %q.", prod)
	f.product = prod
	if !productAccepted(prod, f.Payload.Manifest.Products) {
		return f.overrideVariant(fmt.Sprintf("this phone reports product %q. This ROM is made for product %s (TCL Flip 2 T408DL KEKA / 4058G UPCI).", prod, products),
			nil, "Nothing was written to the phone")
	}
	// the bootloader's other facts must not name a variant this ROM is not for
	facts := getvarFacts(all)
	keys := make([]string, 0, len(facts))
	for k := range facts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.HasPrefix(k, "partition-") {
			continue
		}
		if fv := findForeign(facts[k]); fv != "" {
			return f.overrideVariant(fmt.Sprintf("this phone's bootloader says %s: %s, which is %s, not a T408DL KEKA / 4058G UPCI.", k, facts[k], fv),
				nil, "Nothing was written to the phone")
		}
	}
	return nil
}

func (f *Flow) readUnlock() UnlockState {
	g, _ := f.fastboot(20*time.Second, false, "getvar", "unlocked")
	s := parseUnlocked(g, "")
	if s == UnlockUnknown {
		d, _ := f.fastboot(20*time.Second, false, "oem", "device-info")
		s = parseUnlocked(g, d)
	}
	return s
}

func (f *Flow) unlock() error {
	if err := f.requireMode(ModeBootloader); err != nil {
		return err
	}
	f.Warn()
	if !f.Opts.Preconfirmed {
		ok, err := f.UI.Confirm("    Type UNLOCK and press Enter to unlock (anything else stops):", "UNLOCK")
		if err != nil {
			return err
		}
		if !ok {
			return refuse("stopped by you. The phone is still locked and was not changed")
		}
	}
	f.UI.Step("Unlocking the bootloader")
	f.UI.Say("    LOOK AT THE PHONE NOW: it asks whether to unlock.")
	f.UI.Say("    Press VOLUME UP to say YES.  (Volume Down = No, cancel.)")
	out, err := f.fastboot(5*time.Minute, true, "flashing", "unlock")
	lo := strings.ToLower(out)
	if strings.Contains(lo, "unknown command") || strings.Contains(lo, "not supported") || strings.Contains(lo, "unrecognized") {
		f.UI.Say("    'flashing unlock' is not known to this bootloader, trying 'oem unlock'...")
		out, err = f.fastboot(5*time.Minute, true, "oem", "unlock")
		lo = strings.ToLower(out)
	}
	if strings.Contains(lo, "not allowed") || (strings.Contains(lo, "oem unlock") && strings.Contains(lo, "disabled")) || strings.Contains(lo, "unlock_ability") {
		f.printOEMUnlockHelp()
		return refuse("the bootloader refused to unlock (OEM unlocking is switched off). Nothing was written")
	}
	_ = err
	// The phone may stay in fastboot or restart. Re-detect.
	ok2, err := f.waitFastboot(f.FastbootWait)
	if err != nil {
		return err
	}
	if !ok2 {
		f.UI.Say("    The phone left fastboot mode (that is normal for some units after unlocking).")
		f.UI.Say("    We need to catch it again.")
		if err := f.catchFastboot(); err != nil {
			return err
		}
	}
	st := f.readUnlock()
	// Some bootloaders answer the command first and wait for the key press after.
	for i := 0; st != Unlocked && i < 30; i++ {
		if i == 0 {
			f.UI.Say("    Waiting for you to press VOLUME UP on the phone...")
		}
		f.sleep(3 * time.Second)
		present, err := f.waitFastboot(2 * time.Second)
		if err != nil {
			return err
		}
		if !present {
			f.UI.Say("    The phone left fastboot mode. We need to catch it again.")
			if err := f.catchFastboot(); err != nil {
				return err
			}
		}
		st = f.readUnlock()
	}
	if st != Unlocked {
		f.UI.Say("    The bootloader still says %s.", st)
		f.UI.Say("    If you pressed Volume Down, or did not press anything, run the program again and press VOLUME UP.")
		f.printOEMUnlockHelp()
		return refuse("bootloader is not unlocked, so the ROM cannot be written. Nothing was written")
	}
	f.UI.Say("    OK - bootloader UNLOCKED (the phone's data has been wiped by the unlock).")
	f.wiped = true
	return nil
}

func (f *Flow) Warn() {
	f.UI.Warn(
		"UNLOCKING ERASES EVERYTHING ON THE PHONE.",
		"Contacts, messages, photos, recordings and settings will be DELETED.",
		"This cannot be undone. Copy anything you need off the phone FIRST.",
		"",
		"After unlocking, the phone shows a warning screen at every start",
		"('orange state' / device unlocked). That is normal and cannot be removed.",
	)
}

func (f *Flow) printOEMUnlockHelp() {
	f.UI.Say("")
	f.UI.Say("    If the phone refuses to unlock, turn on 'OEM unlocking' first:")
	f.UI.Say("      1. Turn the phone on normally.")
	f.UI.Say("      2. Settings > About phone > press 'Build number' 7 times ('You are now a developer').")
	f.UI.Say("      3. Settings > System > Developer options > OEM unlocking = ON (confirm).")
	f.UI.Say("      4. Run this program again.")
}

// flashPartition is the ONLY place that writes to the phone.
func (f *Flow) flashPartition(part, file string, disableVerity bool) error {
	if !AllowedPartitions[part] {
		return fmt.Errorf("internal safety stop: refusing to write partition %q (only %v are allowed)", part, keys(AllowedPartitions))
	}
	if strings.HasPrefix(part, "vbmeta") != disableVerity {
		return fmt.Errorf("internal safety stop: verity flags only belong on vbmeta partitions")
	}
	want := ModeBootloader
	if part == "super" {
		want = ModeFastbootd
	}
	if err := f.requireMode(want); err != nil {
		return err
	}
	args := []string{}
	if disableVerity {
		args = append(args, "--disable-verity", "--disable-verification")
	}
	args = append(args, "flash", part, file)
	out, err := f.fastboot(60*time.Minute, true, args...)
	if err != nil || fastbootFailed(out) {
		return fmt.Errorf("writing %s failed: %s", part, lastLines(out, 3))
	}
	return nil
}

func keys(m map[string]bool) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	return k
}

func fastbootFailed(out string) bool {
	lo := strings.ToLower(out)
	return strings.Contains(lo, "failed (") || strings.Contains(lo, "failed:") || strings.Contains(lo, "error:")
}

func lastLines(s string, n int) string {
	ls := strings.Split(strings.TrimSpace(s), "\n")
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	return strings.Join(ls, " | ")
}

func (f *Flow) extractVBMeta(name string) (string, error) {
	r, err := f.Payload.Reader("avb/" + name)
	if err != nil {
		return "", err
	}
	p := filepath.Join(f.Opts.WorkDir, name+".img")
	w, err := os.Create(p)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(w, h), r); err != nil {
		w.Close()
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	// the unpacked file must still be the checked one right before it is written
	if fi, ok := f.Payload.Manifest.Files["avb/"+name]; ok && fi.SHA256 != hex.EncodeToString(h.Sum(nil)) {
		os.Remove(p)
		return "", refuse("CHECKSUM MISMATCH: %s changed after it was unpacked. Nothing more was written. Download the flasher again", name)
	}
	return p, nil
}

func (f *Flow) flashAll(eraseData bool) error {
	m := f.Payload.Manifest
	for i, v := range m.VBMeta {
		f.event(EvStepStart, StepVBMeta(v), "")
		f.UI.Step("Writing %s (verification off) [%d/%d]", v, i+1, len(m.VBMeta)+1)
		p, err := f.extractVBMeta(v)
		if err != nil {
			f.event(EvStepFailed, StepVBMeta(v), err.Error())
			return err
		}
		if err := f.flashPartition(v, p, true); err != nil {
			f.event(EvStepFailed, StepVBMeta(v), err.Error())
			return f.writeFailed(err, false)
		}
		f.event(EvStepDone, StepVBMeta(v), "")
	}
	f.event(EvStepStart, StepSuper, "")
	f.UI.Step("Writing super - the Shomer OS ROM [%d/%d] (several minutes; fastboot sends it in pieces)", len(m.VBMeta)+1, len(m.VBMeta)+1)
	if err := f.flashPartition("super", f.superRaw, false); err != nil {
		f.event(EvStepFailed, StepSuper, err.Error())
		return f.writeFailed(err, f.inFastbootd)
	}
	f.UI.Say("    OK - super written.")
	f.event(EvStepDone, StepSuper, "")
	if eraseData {
		if err := f.requireMode(ModeFastbootd); err != nil {
			return err
		}
		f.event(EvStepStart, StepErase, "")
		f.UI.Step("Erasing user data")
		out, err := f.fastboot(10*time.Minute, true, "erase", "userdata")
		if err != nil || fastbootFailed(out) {
			f.UI.Say("    Erasing data did not work from here (%s).", lastLines(out, 2))
			f.UI.Say("    If the phone does not finish starting, do a factory reset from recovery (README 'Stuck').")
			f.event(EvStepFailed, StepErase, "requested data erase did not complete")
			return fmt.Errorf("ROM written but requested userdata erase failed: %s", lastLines(out, 4))
		} else {
			f.event(EvStepDone, StepErase, "")
		}
	}
	return nil
}

func (f *Flow) writeFailed(err error, superStarted bool) error {
	f.UI.Warn(
		"WRITING FAILED: "+err.Error(),
		"Do not panic. The phone can still be flashed again: run this program again,",
		"it will catch the phone the same way. The bootloader was not touched.",
		"Log file: see the path printed at the top.",
	)
	if superStarted {
		f.UI.Say("    super may be half written - the phone will not start until it is flashed again. That is fixable.")
	}
	return err
}

func (f *Flow) printAfter() {
	f.UI.Step("DONE - Shomer OS has been written")
	if f.BackupPath != "" {
		f.UI.Say("    The backup of the phone as it was is here (keep it private, it holds the IMEI):")
		f.UI.Say("      %s", f.BackupPath)
	}
	if len(f.variantWarned) > 0 {
		f.UI.Say("    Note: you chose to go on as %s after %d warning(s) about the phone's model/version (in the log).", f.buildOK, len(f.variantWarned))
	}
	f.UI.Say("    What happens now:")
	f.UI.Say("    - The phone shows an 'orange state' / unlocked warning for a few seconds at every start. Normal.")
	f.UI.Say("    - The FIRST start takes longer than usual (up to 5-10 minutes on the TCL logo / Shomer screen).")
	f.UI.Say("      Leave it alone and plugged in or charged. Do not pull the battery during the first start.")
	f.UI.Say("    - BEFORE you use it: add this phone's IMEI in the Shomer admin portal and give it a level.")
	f.UI.Say("      The IMEI is on the sticker under the battery, on the box, or dial *#06# on the phone.")
	f.UI.Say("      Until the server knows the IMEI the phone runs in its safe default level (emergency calls always work).")
	f.UI.Say("    - Connect the phone to the internet (SIM data or Wi-Fi) so it can fetch its level.")
	f.UI.Say("    If it is stuck for more than 15 minutes, see 'If something goes wrong' in the README.")
}

// Cleanup removes the unpacked files.
func (f *Flow) Cleanup() {
	if f.Opts.KeepWorkDir || f.Opts.WorkDir == "" {
		return
	}
	for _, n := range []string{"super.raw", "vbmeta.img", "vbmeta_system.img", "vbmeta_vendor.img", "recovery-dump.img", "recovery-stock.img"} {
		os.Remove(filepath.Join(f.Opts.WorkDir, n))
	}
	os.Remove(f.Opts.WorkDir) // only if empty
}

var _ = errors.New
