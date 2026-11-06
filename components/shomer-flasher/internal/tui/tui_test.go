//go:build !windows

package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/Shomer-TCL/shomer-flasher/internal/app"
	"github.com/Shomer-TCL/shomer-flasher/internal/bundle"
	"github.com/Shomer-TCL/shomer-flasher/internal/flasher"
)

func newT() *Model {
	man := &bundle.Manifest{Name: "Shomer OS", Build: "2026-11-02_8a49ed1", VBMeta: []string{"vbmeta", "vbmeta_system", "vbmeta_vendor"},
		SuperRaw: bundle.FileInfo{Size: 2 << 30}}
	m := app.NewModel(man, "1.4.1", flasher.Plan{Backup: true, BackupDir: "/home/sam/shomer-backups", EraseData: true}, "/tmp/f.log")
	t := New(m, nil)
	t.w, t.h = 100, 40
	return t
}

func view(t *Model) string { return ansi.Strip(t.View()) }

func feed(t *Model, m app.Msg) { t.Update(appMsg(m)) }

func TestTUIWholeRun(t *testing.T) {
	tt := newT()
	feed(tt, app.Msg{Kind: app.MsgChecks, Checks: []app.Check{{ID: "sudo", Name: "Permission to use USB", Status: app.CheckWarn, Detail: "not running as root", Fix: "Quit (q) and start again with:  sudo ./x"},
		{ID: "rom", Name: "ROM inside this program", Status: app.CheckRunning}}})
	v := view(tt)
	if !strings.Contains(v, "SHOMER") || !strings.Contains(v, "[WARN]") || !strings.Contains(v, "sudo ./x") {
		t.Fatal(v)
	}
	tt.Key("enter")
	if len(tt.Actions) != 0 {
		t.Fatal("continued before the ROM check finished")
	}
	feed(tt, app.Msg{Kind: app.MsgCheck, Check: app.Check{ID: "rom", Name: "ROM inside this program", Status: app.CheckOK}})
	feed(tt, app.Msg{Kind: app.MsgPrepared})
	tt.Key("enter")
	if len(tt.Actions) != 1 || tt.Actions[0] != app.ActDetect || tt.M.Screen != app.ScreenPhone {
		t.Fatalf("%v %v", tt.Actions, tt.M.Screen)
	}
	// a flasher question shows its instructions, Enter answers it
	feed(tt, app.Msg{Kind: app.MsgLine, Line: app.Line{Kind: app.LineStep, Text: "Getting the phone into FASTBOOT mode (try 1)"}})
	feed(tt, app.Msg{Kind: app.MsgLine, Line: app.Line{Text: "    1. UNPLUG the USB cable from the phone."}})
	q := app.NewTestQuestion(app.QEnter, "    Press Enter when the phone is OFF and UNPLUGGED...", false)
	feed(tt, app.Msg{Kind: app.MsgQuestion, Question: q})
	if v := view(tt); !strings.Contains(v, "UNPLUG the USB cable") || !strings.Contains(v, "Done - press Enter") {
		t.Fatal(v)
	}
	tt.Key("enter")
	if a := app.TestAnswer(q); a.Cancel {
		t.Fatal("enter should answer")
	}
	feed(tt, app.Msg{Kind: app.MsgDetected, Phone: flasher.PhoneInfo{Serial: "MOCK0001", Product: "gflip6", Model: "T408DL", Build: "KEKA", Unlock: flasher.Locked}})
	if v := view(tt); !strings.Contains(v, "Phone found") || !strings.Contains(v, "MOCK0001") || !strings.Contains(v, "LOCKED") {
		t.Fatal(v)
	}
	tt.Key("enter")
	if tt.M.Screen != app.ScreenOptions {
		t.Fatal("options")
	}
	v = view(tt)
	if !strings.Contains(v, "[x] ON") || !strings.Contains(v, "Backup the phone first") {
		t.Fatal(v)
	}
	// backup off and on again with Enter on the first row
	tt.Key("enter")
	if tt.M.Plan.Backup {
		t.Fatal("toggle off")
	}
	tt.Key("enter")
	// edit the folder
	tt.Key("down")
	tt.Key("enter")
	for i := 0; i < len("/home/sam/shomer-backups"); i++ {
		tt.Key("backspace")
	}
	for _, r := range "/media/usb/backups" {
		tt.Key(string(r))
	}
	tt.Key("enter")
	if tt.M.Plan.BackupDir != "/media/usb/backups" {
		t.Fatalf("folder %q", tt.M.Plan.BackupDir)
	}
	// userdata row is disabled on a locked phone
	tt.Key("down")
	tt.Key("enter")
	if tt.M.Plan.Userdata {
		t.Fatal("userdata on a locked phone")
	}
	tt.Key("down")
	tt.Key("down")
	tt.Key("enter") // Continue
	if tt.M.Screen != app.ScreenConfirm {
		t.Fatalf("screen %v", tt.M.Screen)
	}
	v = view(tt)
	for _, s := range []string{"fastboot flashing unlock", "fastboot flash recovery <backup recovery", "/media/usb/backups", "FLASH NOW", "UNLOCKING ERASES"} {
		if !strings.Contains(v, s) {
			t.Fatalf("confirm misses %q\n%s", s, v)
		}
	}
	// Back, then forward again
	tt.Key("right")
	tt.Key("enter")
	if tt.M.Screen != app.ScreenOptions {
		t.Fatal("back")
	}
	tt.M.Continue()
	tt.cur = 0
	tt.Key("enter") // FLASH NOW -> final question, default No
	if !strings.Contains(view(tt), "Unlock the bootloader") {
		t.Fatal(view(tt))
	}
	tt.Key("enter") // No
	if tt.M.Screen != app.ScreenConfirm {
		t.Fatal("default must be No")
	}
	tt.Key("enter")
	tt.Key("y")
	if tt.M.Screen != app.ScreenProgress || len(tt.M.Steps) != 9 {
		t.Fatalf("progress %v %d", tt.M.Screen, len(tt.M.Steps))
	}
	feed(tt, app.Msg{Kind: app.MsgEvent, Event: flasher.Event{Kind: flasher.EvStepStart, Step: "backup-read"}})
	feed(tt, app.Msg{Kind: app.MsgProgress, Progress: app.Progress{Label: "reading [3/40] boot", Done: 12 << 20, Total: 24 << 20}})
	if v := view(tt); !strings.Contains(v, "[>] Backup 2/3") || !strings.Contains(v, " 50%  reading [3/40] boot (12 / 24 MB)") {
		t.Fatal(v)
	}
	// backup fails: the flasher asks, default No, y answers yes
	q2 := app.NewTestQuestion(app.QYesNo, "    Backup failed: continue flashing WITHOUT a backup?", false)
	feed(tt, app.Msg{Kind: app.MsgQuestion, Question: q2})
	if v := view(tt); !strings.Contains(v, "Backup failed: continue flashing WITHOUT a backup?") {
		t.Fatal(v)
	}
	tt.Key("y")
	if a := app.TestAnswer(q2); !a.Yes {
		t.Fatal("y")
	}
	// q while writing asks first
	tt.Key("q")
	if tt.Quit || !strings.Contains(view(tt), "Quit anyway?") {
		t.Fatal("quit while writing must ask")
	}
	tt.Key("enter") // default No
	if tt.Quit {
		t.Fatal("quit")
	}
	feed(tt, app.Msg{Kind: app.MsgEvent, Event: flasher.Event{Kind: flasher.EvBackupFailed, Detail: "no root", Path: "/media/usb/backups/backup-X-INCOMPLETE.zip"}})
	feed(tt, app.Msg{Kind: app.MsgFinished})
	v = view(tt)
	if !strings.Contains(v, "DONE") || !strings.Contains(v, "backup-X-INCOMPLETE.zip") || !strings.Contains(v, "/tmp/f.log") {
		t.Fatal(v)
	}
	tt.Key("enter")
	if !tt.Quit {
		t.Fatal("close")
	}
}

func TestTUITypedAnswerAndSmallTerminal(t *testing.T) {
	tt := newT()
	tt.w, tt.h = 80, 24
	tt.M.Prepared = true
	tt.M.Continue()
	q := app.NewTestQuestion(app.QText, "    Type the 4 letters you see (KEKA/UPCI), or Q to quit:", false)
	feed(tt, app.Msg{Kind: app.MsgQuestion, Question: q})
	for _, k := range []string{"K", "E", "K", "X", "backspace", "A", "enter"} {
		tt.Key(k)
	}
	if a := app.TestAnswer(q); a.Text != "KEKA" {
		t.Fatalf("%q", a.Text)
	}
	v := view(tt)
	if n := len(strings.Split(v, "\n")); n > 24 {
		t.Fatalf("view is %d lines on a 24-line terminal", n)
	}
	for _, l := range strings.Split(v, "\n") {
		if ansi.StringWidth(l) > 80 {
			t.Fatalf("line wider than 80: %q", l)
		}
	}
}
