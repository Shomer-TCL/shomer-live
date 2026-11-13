// Command shomer-flasher writes Shomer OS to a TCL Flip 2 (T408DL KEKA / 4058G UPCI).
// The ROM, fastboot and adb are inside this one file. See README.md.
package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/Shomer-TCL/shomer-flasher/internal/bundle"
	"github.com/Shomer-TCL/shomer-flasher/internal/flasher"
	"github.com/Shomer-TCL/shomer-flasher/internal/gui"
	workfolder "github.com/Shomer-TCL/shomer-flasher/internal/workdir"
)

var version = "dev" // set by -ldflags
var forceTerminalUI bool

//go:embed THIRD_PARTY_LICENSES.txt
var thirdPartyLicenses string

func main() {
	os.Exit(run())
}

func run() int {
	var (
		port     = flag.String("port", "", "serial port of the phone's preloader (normally found automatically), e.g. COM5 or /dev/ttyACM0")
		workdir  = flag.String("workdir", "", "folder to unpack the ROM into (needs ~2.3 GB free); default: a folder next to this program")
		keep     = flag.Bool("keep", false, "keep the unpacked ROM after finishing")
		dry      = flag.Bool("dry-run", false, "check and unpack the ROM and show what would run, without touching any phone")
		info     = flag.Bool("info", false, "print what is inside this flasher and exit")
		noPause  = flag.Bool("no-pause", false, "do not wait for Enter before closing")
		showVer  = flag.Bool("version", false, "print version and exit")
		bromMode = flag.Bool("brom", false, "backup-first flash through MTKClient BROM (Shomer Live Linux only; requires --brom-reference)")
		bromRef  = flag.String("brom-reference", "", "enrolled private 4058G UPCI BROM backup directory")
		backup   = flag.Bool("backup", true, "backup-first: copy every partition of the phone into a zip on this computer before flashing (default on)")
		noBak    = flag.Bool("no-backup", false, "do not make a backup (recovery is then never touched)")
		bakDir   = flag.String("backup-dir", "", "folder for the backup zip (needs ~3 GB free, ~8 GB with --backup-userdata); default: a folder 'shomer-backups' next to this program")
		bakUser  = flag.Bool("backup-userdata", false, "also copy userdata (phone data, ~4.6 GB, encrypted). Only possible if the bootloader is ALREADY unlocked: unlocking erases it")
		resRec   = flag.Bool("restore-recovery", false, "only write the stock TCL recovery back (use after an interrupted backup), then stop")
		variant  = flag.String("variant", "", "the phone's software version, KEKA or UPCI, if you already know it (answers the 'which version?' question; useful when adb cannot read the phone)")
		forceV   = flag.Bool("force-variant", false, "if the model/variant/version check warns, go on with your own KEKA/UPCI choice without asking (the warning is still shown). Checksums, backup, disk and connection checks still stop")
		contNB   = flag.Bool("continue-without-backup", false, "if the backup fails, flash anyway without asking (for unattended runs). The stock recovery is still put back first")
		cli      = flag.Bool("cli", false, "plain console mode: questions in this window, no full-screen TUI (Linux) or window (Windows)")
		forceUI  = flag.Bool("ui", false, "open the TUI (Linux) / window (Windows) even when not started from a terminal")
		guiUI    = flag.Bool("gui", false, "open the native desktop window (GTK build on Linux, Win32 on Windows)")
		tuiUI    = flag.Bool("tui", false, "use the Linux terminal interface even in a native GTK build")
		logDir   = flag.String("log-dir", "", "folder for persistent logs (recommended on a live USB)")
		lics     = flag.Bool("licenses", false, "print the licences of the open-source parts inside this program and exit")
		selfT    = flag.Bool("gui-selftest", false, "test only (Windows): open the window's question boxes and answer them through Windows messages; prints PASS/FAIL lines")
		smoke    = flag.String("gui-smoke", "", "test only: open the window, show every screen with example data, close again; PNG screenshots go into the folder given ('-' = none)")
	)
	flag.Parse()
	if *bromMode {
		if runtime.GOOS != "linux" {
			fmt.Fprintln(os.Stderr, "--brom is available only in the Shomer Live Linux environment")
			return 2
		}
		if *bromRef == "" || *bakDir == "" || *workdir == "" || *logDir == "" {
			fmt.Fprintln(os.Stderr, "--brom requires --brom-reference, --backup-dir, --workdir, and --log-dir")
			return 2
		}
		if *noBak || !*backup || *contNB || *bakUser || *resRec || *dry || *variant != "" || *forceV || *guiUI || *tuiUI || *cli {
			fmt.Fprintln(os.Stderr, "--brom cannot skip its BROM backup or be combined with recovery, variant, dry-run, or other interface options")
			return 2
		}
		helper, err := exec.LookPath("shomer-brom-flash")
		if err != nil {
			fmt.Fprintln(os.Stderr, "shomer-brom-flash is not installed; run this option from Shomer Live")
			return 2
		}
		cmd := exec.Command(helper, "--reference", *bromRef, "--backup-dir", *bakDir, "--workdir", *workdir, "--log-dir", *logDir, "--kit-dir", "/opt/shomer/TCL-Flip2")
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				return e.ExitCode()
			}
			fmt.Fprintln(os.Stderr, "could not start BROM flasher:", err)
			return 1
		}
		return 0
	}
	if *bromRef != "" {
		fmt.Fprintln(os.Stderr, "--brom-reference requires --brom")
		return 2
	}
	forceTerminalUI = *tuiUI
	if *guiUI && (*tuiUI || *cli) {
		fmt.Fprintln(os.Stderr, "--gui cannot be combined with --tui or --cli")
		return 2
	}
	if *guiUI && !gui.SupportsNative {
		fmt.Fprintln(os.Stderr, "this binary has no native GUI; use the GTK Linux build or --tui")
		return 2
	}
	pause := func() {
		if !*noPause {
			fmt.Print("\nPress Enter to close this window...")
			var b [1]byte
			os.Stdin.Read(b[:])
		}
	}
	if *showVer {
		fmt.Println("shomer-flasher", version)
		return 0
	}
	if *lics {
		fmt.Print(thirdPartyLicenses)
		return 0
	}
	if *smoke != "" {
		return guiSmoke(*smoke)
	}
	if *selfT {
		return guiSelfTest()
	}
	// TUI (Linux) / window (Windows) unless asked for the console, or not interactive
	useUI := !*cli && !*noPause && !*dry && !*info && !*resRec &&
		(*forceUI || *guiUI || *tuiUI || (gui.SupportsNative && os.Getenv("DISPLAY") != "" && !*tuiUI) || (term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))))
	if useUI && runtime.GOOS == "windows" {
		hideOwnConsole() // double-clicked: the window is the interface
	}

	// fail shows a problem that stops the program before (or instead of) the
	// interface. A double-clicked Windows exe has hidden its console, so there
	// the message is a message box: printing it and waiting for Enter on the
	// hidden console would leave an invisible program hanging (1.4.2 did).
	fail := func(msg string) int {
		if useUI && runtime.GOOS == "windows" {
			gui.ShowError("Shomer flasher", msg)
			return 2
		}
		fmt.Println("ERROR:", msg)
		pause()
		return 2
	}
	exe, err := os.Executable()
	if err != nil {
		return fail(err.Error())
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	exeDir := filepath.Dir(exe)

	if *info {
		pl, err := bundle.Open(exe)
		if err != nil {
			fmt.Println("ERROR:", err)
			return 2
		}
		defer pl.Close()
		m := pl.Manifest
		fmt.Printf("%s build %s (ROM commit %s, packed %s)\n", m.Name, m.Build, m.ROMCommit, m.BuiltAt)
		fmt.Printf("super: %d bytes, sha256 %s\n", m.SuperRaw.Size, m.SuperRaw.SHA256)
		for n, fi := range m.Files {
			fmt.Printf("  %-40s %10d  %s\n", n, fi.Size, fi.SHA256)
		}
		fmt.Printf("accepted products %v, models %v, builds %v\n", m.Products, m.Models, m.Builds)
		if m.PlatformTools != "" {
			fmt.Printf("fastboot/adb: Android platform-tools %s\n", m.PlatformTools)
		} else {
			fmt.Println("fastboot/adb: platform-tools version not recorded")
		}
		if m.Recovery != nil {
			fmt.Printf("backup recovery %s, stock recovery %s\n", m.Recovery.Dump, m.Recovery.Stock)
		} else {
			fmt.Println("no backup recovery inside (built before backup-first)")
		}
		return 0
	}

	// one copy at a time: two copies delete each other's unpacked ROM and tools
	// and stop each other's adb. The OS frees the lock if the program dies.
	glk, err := workfolder.Acquire(filepath.Join(os.TempDir(), "shomer-flasher.lock"))
	if err != nil {
		return fail(err.Error())
	}
	defer glk.Release()

	// log file next to the program (fallback: temp folder)
	stamp := time.Now().Format("20060102-150405")
	logBase := exeDir
	if *logDir != "" {
		if err := os.MkdirAll(*logDir, 0700); err != nil {
			return fail(fmt.Sprintf("cannot create log directory: %v", err))
		}
		logBase = *logDir
	}
	logPath := filepath.Join(logBase, "shomer-flasher-"+stamp+".log")
	logf, err := os.Create(logPath)
	if err != nil && *logDir == "" {
		logPath = filepath.Join(os.TempDir(), "shomer-flasher-"+stamp+".log")
		logf, err = os.Create(logPath)
	}
	if err != nil && *logDir != "" {
		return fail(fmt.Sprintf("cannot write persistent log: %v", err))
	}
	var logw io.Writer = io.Discard
	if err == nil {
		defer logf.Close()
		logw = logf
	}
	ui := flasher.NewUI(os.Stdin, os.Stdout, logw)
	ui.Log("shomer-flasher %s on %s/%s, exe %s, ui %v", version, runtime.GOOS, runtime.GOARCH, exe, useUI)
	if !useUI {
		ui.Say("Log file: %s", logPath)
	}

	pl, err := bundle.Open(exe)
	if err != nil {
		ui.Log("ERROR: %v", err)
		return fail(fmt.Sprintf("%v\n\nThe flasher file looks damaged or incomplete: download it again.", err))
	}
	defer pl.Close()

	// The bundled CI fixture contains a fake ROM. Never let a normal launch use real USB tools.
	if !fixtureIOAllowed(pl.Manifest.Build, *dry, os.Getenv("SHOMER_FASTBOOT"), os.Getenv("SHOMER_ADB")) {
		return fail("This is a test fixture with a fake ROM. Configure both mock tool overrides or use --dry-run. Use the Shomer-Flasher program to flash a real phone.")
	}

	// work folder: always the flasher's own marked subfolder of --workdir (or
	// of the program's folder); only the flasher's own files are ever removed
	base := *workdir
	if base == "" {
		base = exeDir
	}
	wdir, err := workfolder.Prepare(base)
	if err != nil && *workdir == "" {
		ui.Log("work folder next to the program: %v; using the temp folder", err)
		wdir, err = workfolder.Prepare(os.TempDir())
	}
	if err != nil {
		ui.Log("ERROR: %v", err)
		return fail(err.Error())
	}
	wlk, err := workfolder.Acquire(filepath.Join(wdir.Path, workfolder.LockName))
	if err != nil {
		ui.Log("ERROR: %v", err)
		return fail(err.Error())
	}
	defer func() { wlk.Release(); wdir.Cleanup(*keep) }()
	wd := wdir.Path
	ui.Log("work folder %s", wd)

	bd := *bakDir
	if bd == "" {
		bd = filepath.Join(exeDir, "shomer-backups")
	}
	if abs, err := filepath.Abs(bd); err == nil {
		bd = abs
	}

	// tools: fastboot/adb from the payload, unless overridden (tests)
	toolDir := wdir.ToolDir()
	paths, err := extractTools(pl, wdir)
	if err != nil {
		ui.Log("ERROR unpacking fastboot/adb: %v", err)
		return fail(fmt.Sprintf("unpacking fastboot/adb failed: %v\n\n(On Windows an antivirus sometimes blocks adb.exe: allow it, or download the flasher again.)", err))
	}
	if v := os.Getenv("SHOMER_FASTBOOT"); v != "" {
		paths["fastboot"] = v
	}
	if v := os.Getenv("SHOMER_ADB"); v != "" {
		paths["adb"] = v
	}
	runner := &flasher.ExecRunner{Paths: paths}
	fl := &flasher.Flow{
		UI:      ui,
		Run:     runner,
		Pre:     &flasher.SerialPreloader{Fixed: firstNonEmpty(*port, os.Getenv("SHOMER_PRELOADER_PORT"))},
		Payload: pl,
		Opts: flasher.Options{WorkDir: wd, KeepWorkDir: *keep, DryRun: *dry,
			Backup: *backup && !*noBak, BackupDir: bd, BackupUserdata: *bakUser, RestoreRecovery: *resRec,
			ContinueWithoutBackup: *contNB, Variant: *variant, ForceVariant: *forceV},
		ToolDir: toolDir,
	}
	if v := os.Getenv("SHOMER_TEST_FAST"); v == "1" {
		fl.PreloaderWait = 10 * time.Second
		fl.FastbootWait = 5 * time.Second
		fl.Sleep = func(d time.Duration) { time.Sleep(d / 20) }
	}
	var code int
	if useUI {
		code = runUI(fl, exe, logw, logPath, *bakUser)
		if code < 0 { // the TUI/window could not open: use the console
			fl.UI = ui
			ui.Say("Log file: %s", logPath)
			useUI = false
		}
	}
	if !useUI {
		err = fl.Execute()
	}
	// stop the adb server we may have started, so files can be deleted
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	runner.Run(ctx, "adb", []string{"kill-server"}, nil)
	cancel()
	fl.Cleanup()
	wlk.Release()
	wdir.Cleanup(*keep) // only the flasher's own files; folders only if empty
	if useUI {
		return code
	}
	if err != nil {
		var r flasher.ErrRefused
		if errors.As(err, &r) {
			ui.Say("")
			ui.Say("STOPPED: %s", r.Msg)
			pause()
			return 3
		}
		ui.Say("")
		ui.Say("ERROR: %v", err)
		ui.Say("The log file has the details: %s", logPath)
		pause()
		return 1
	}
	pause()
	return 0
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}

// extractTools writes the payload's tools/<goos>/* files into dir and returns tool paths.
func extractTools(pl *bundle.Payload, wdir *workfolder.Dir) (map[string]string, error) {
	dir := wdir.ToolDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	paths := map[string]string{}
	for _, name := range pl.Manifest.Tools {
		r, err := pl.Reader(name)
		if err != nil {
			return nil, err
		}
		dst := filepath.Join(dir, filepath.Base(name))
		wdir.AddTool(name)
		w, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return nil, err
		}
		h := sha256.New()
		if _, err := io.Copy(io.MultiWriter(w, h), r); err != nil {
			w.Close()
			return nil, err
		}
		w.Close()
		if fi, ok := pl.Manifest.Files[name]; ok && fi.SHA256 != hex.EncodeToString(h.Sum(nil)) {
			return nil, fmt.Errorf("%s checksum mismatch", name)
		}
		base := strings.TrimSuffix(filepath.Base(name), ".exe")
		if base == "fastboot" || base == "adb" {
			paths[base] = dst
		}
	}
	if paths["fastboot"] == "" || paths["adb"] == "" {
		return nil, fmt.Errorf("fastboot/adb missing from payload")
	}
	return paths, nil
}

func fixtureIOAllowed(build string, dry bool, fastboot, adb string) bool {
	return build != "ci-test" || dry || (fastboot != "" && adb != "")
}
