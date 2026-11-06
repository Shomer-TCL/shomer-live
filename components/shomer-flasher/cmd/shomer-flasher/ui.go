package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/Shomer-TCL/shomer-flasher/internal/app"
	"github.com/Shomer-TCL/shomer-flasher/internal/flasher"
	"github.com/Shomer-TCL/shomer-flasher/internal/gui"
	"github.com/Shomer-TCL/shomer-flasher/internal/tui"
)

// runUI runs the TUI (Linux) or the window (Windows). It returns the exit
// code, or -1 if the interface could not open (then the console is used).
func runUI(fl *flasher.Flow, exe string, logw io.Writer, logPath string, userdata bool) int {
	plan := flasher.Plan{Backup: fl.Opts.Backup, BackupDir: fl.Opts.BackupDir, Userdata: userdata, EraseData: true}
	m := app.NewModel(&fl.Payload.Manifest, version, plan, logPath)
	console := fl.UI
	c := app.NewController(fl, app.RealEnv(runtime.GOOS, exe, flasher.FreeBytes), logw)
	var err error
	if gui.SupportsNative && !forceTerminalUI {
		err = gui.Run(m, c)
	} else {
		err = tui.Run(m, c)
	}
	if err != nil && m.Screen == app.ScreenChecks && !m.Busy() && !m.Prepared {
		showOwnConsole()
		fmt.Fprintf(os.Stderr, "Could not open the full-screen interface (%v). Using the plain console instead.\n", err)
		fl.UI = console
		return -1
	}
	// after the TUI closes, leave the result in the terminal
	if runtime.GOOS != "windows" {
		for _, l := range m.DoneLines() {
			fmt.Println(l)
		}
	}
	switch {
	case m.Result.OK:
		return 0
	case m.Result.Refused, m.Result.Err == nil:
		return 3 // stopped on purpose, or closed before the end
	case errors.Is(m.Result.Err, flasher.ErrInputClosed):
		return 3
	}
	return 1
}

// guiSmoke opens the window with example data (CI). dir "-" = no screenshots.
func guiSmoke(dir string) int {
	if dir == "-" {
		dir = ""
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Println("ERROR:", err)
		return 2
	}
	m := app.NewModel(app.DemoManifest(), version, flasher.Plan{Backup: true, BackupDir: `C:\Users\Sam\Downloads\shomer-backups`, EraseData: true},
		`C:\Users\Sam\Downloads\shomer-flasher-20261103-101500.log`)
	if runtime.GOOS == "linux" {
		m.Plan.BackupDir = "/media/backup/Shomer-backups"
		m.Result.LogPath = "/media/backup/Shomer-logs/demo.log"
	}
	if err := gui.Smoke(m, dir); err != nil {
		fmt.Println("ERROR:", err)
		return 2
	}
	fmt.Println("gui smoke OK", version)
	return 0
}

// guiSelfTest drives the window's question boxes through Windows messages (CI).
func guiSelfTest() int {
	m := app.NewModel(app.DemoManifest(), version, flasher.Plan{Backup: true, BackupDir: `C:\Users\Sam\Downloads\shomer-backups`, EraseData: true}, "")
	if err := gui.SelfTest(m, os.Stdout); err != nil {
		fmt.Println("ERROR:", err)
		return 2
	}
	fmt.Println("gui selftest OK", version)
	return 0
}
