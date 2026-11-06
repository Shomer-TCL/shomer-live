//go:build windows

package gui

import (
	"fmt"
	"io"
	"strings"
	"syscall"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"

	"github.com/Shomer-TCL/shomer-flasher/internal/app"
	"github.com/Shomer-TCL/shomer-flasher/internal/flasher"
)

// SelfTest (--gui-selftest, for CI) opens the window's real question boxes on
// the Windows desktop and answers them through Windows messages: characters
// typed into the real edit box (WM_CHAR), the Enter and Esc keys (WM_KEYDOWN
// through the dialog's own message loop), clicks on the real buttons
// (BM_CLICK), the close box (WM_CLOSE), and the Yes/No/OK/Cancel buttons of
// the message boxes. Only the hardware input itself is simulated; the whole
// .exe is also driven with real keyboard and mouse input by
// test/e2e_gui_windows.py. It runs inside the shipped program because the
// window needs the program's Windows manifest (common controls 6).
func SelfTest(m *app.Model, out io.Writer) error {
	fails := 0
	check := func(ok bool, format string, a ...any) {
		s := fmt.Sprintf(format, a...)
		if ok {
			fmt.Fprintln(out, "PASS", s)
		} else {
			fails++
			fmt.Fprintln(out, "FAIL", s)
		}
	}
	w := &window{m: m}
	if err := w.build(); err != nil {
		return fmt.Errorf("window: %v", err)
	}
	defer w.mw.Dispose()
	w.render()

	type act func(dlg *walk.Dialog, le *walk.LineEdit, ok, cancel *walk.PushButton)
	timedOut := false
	runDialog := func(a act) (string, bool) {
		timedOut = false
		dialogHook = func(dlg *walk.Dialog, le *walk.LineEdit, ok, cancel *walk.PushButton) {
			h := dlg.Handle()
			go func() {
				time.Sleep(400 * time.Millisecond)
				a(dlg, le, ok, cancel)
				for i := 0; i < 100 && win.IsWindowVisible(h); i++ {
					time.Sleep(100 * time.Millisecond)
				}
				if win.IsWindowVisible(h) {
					timedOut = true
					win.PostMessage(h, win.WM_CLOSE, 0, 0)
				}
			}()
		}
		defer func() { dialogHook = nil }()
		return textDialog(w.mw, "Type the 4 letters you see (KEKA/UPCI), or Q to quit:")
	}
	typeInto := func(h win.HWND, s string) {
		for _, r := range s {
			win.PostMessage(h, win.WM_CHAR, uintptr(r), 0)
		}
		time.Sleep(200 * time.Millisecond)
	}
	key := func(h win.HWND, vk uintptr) {
		win.PostMessage(h, win.WM_KEYDOWN, vk, 0)
		win.PostMessage(h, win.WM_KEYUP, vk, 0xC0000001)
	}

	// --- the text box on its own ---
	t, ok := runDialog(func(_ *walk.Dialog, le *walk.LineEdit, _, _ *walk.PushButton) {
		typeInto(le.Handle(), "UPCI")
		key(le.Handle(), win.VK_RETURN)
	})
	check(ok && t == "UPCI" && !timedOut, "text box: type UPCI, Enter -> %q accepted=%v", t, ok)
	t, ok = runDialog(func(_ *walk.Dialog, le *walk.LineEdit, okb, _ *walk.PushButton) {
		typeInto(le.Handle(), " upci ")
		win.PostMessage(okb.Handle(), win.BM_CLICK, 0, 0)
	})
	check(ok && t == " upci " && !timedOut, "text box: type \" upci \", click OK -> %q accepted=%v", t, ok)
	t, ok = runDialog(func(dlg *walk.Dialog, le *walk.LineEdit, _, _ *walk.PushButton) {
		typeInto(le.Handle(), "KEKA")
		win.PostMessage(dlg.Handle(), win.WM_COMMAND, win.IDOK, 0) // what the dialog makes of Enter
	})
	check(ok && t == "KEKA" && !timedOut, "text box: type KEKA, dialog OK command -> %q accepted=%v", t, ok)
	_, ok = runDialog(func(_ *walk.Dialog, le *walk.LineEdit, _, _ *walk.PushButton) {
		typeInto(le.Handle(), "UPCI")
		key(le.Handle(), win.VK_ESCAPE)
	})
	check(!ok && !timedOut, "text box: Esc cancels")
	_, ok = runDialog(func(_ *walk.Dialog, le *walk.LineEdit, _, c *walk.PushButton) {
		typeInto(le.Handle(), "UPCI")
		win.PostMessage(c.Handle(), win.BM_CLICK, 0, 0)
	})
	check(!ok && !timedOut, "text box: Cancel button cancels")
	_, ok = runDialog(func(dlg *walk.Dialog, le *walk.LineEdit, _, _ *walk.PushButton) {
		typeInto(le.Handle(), "UPCI")
		win.PostMessage(dlg.Handle(), win.WM_CLOSE, 0, 0)
	})
	check(!ok && !timedOut, "text box: close box cancels")

	// --- the 1.4.1 way, to show the root cause on this desktop ---
	{
		var dlg *walk.Dialog
		var le *walk.LineEdit
		var okb, cb *walk.PushButton
		err := Dialog{AssignTo: &dlg, Title: "1.4.1 dialog", DefaultButton: &okb, CancelButton: &cb, Layout: VBox{},
			Children: []Widget{LineEdit{AssignTo: &le},
				PushButton{AssignTo: &okb, Text: "OK", OnClicked: func() { dlg.Accept() }},
				PushButton{AssignTo: &cb, Text: "Cancel", OnClicked: func() { dlg.Cancel() }}}}.Create(w.mw)
		if err != nil {
			check(false, "1.4.1 dialog: %v", err)
		} else {
			go func() {
				time.Sleep(400 * time.Millisecond)
				typeInto(le.Handle(), "UPCI")
				win.PostMessage(okb.Handle(), win.BM_CLICK, 0, 0)
			}()
			res := dlg.Run()
			got := le.Text()
			check(res == walk.DlgCmdOK && got == "", "root cause: the 1.4.1 code read the box after it closed and got %q (result %d)", got, res)
		}
	}

	// --- the window's question bridge (what the flasher asks, what it gets back) ---
	ask := func(q *app.Question, a func()) app.Answer {
		w.m.Apply(app.Msg{Kind: app.MsgQuestion, Question: q})
		go a()
		w.ask(q)
		select {
		case ans := <-answerOf(q):
			return ans
		case <-time.After(5 * time.Second):
			return app.Answer{Text: "<no answer>"}
		}
	}
	msgBox := func(id uintptr) func() {
		return func() {
			title, _ := syscall.UTF16PtrFromString("Shomer flasher")
			cls, _ := syscall.UTF16PtrFromString("#32770")
			for i := 0; i < 100; i++ {
				time.Sleep(100 * time.Millisecond)
				if h := win.FindWindow(cls, title); h != 0 && win.IsWindowVisible(h) {
					time.Sleep(200 * time.Millisecond)
					win.PostMessage(h, win.WM_COMMAND, id, 0)
					return
				}
			}
		}
	}
	textAnswer := func(s string) func() {
		return func() {
			title, _ := syscall.UTF16PtrFromString("Shomer flasher")
			for i := 0; i < 100; i++ {
				time.Sleep(100 * time.Millisecond)
				h := win.FindWindow(nil, title)
				if h == 0 || !win.IsWindowVisible(h) {
					continue
				}
				e := findEdit(h) // the edit box sits in the dialog's client composite
				if e == 0 {
					continue
				}
				typeInto(e, s)
				key(e, win.VK_RETURN)
				return
			}
		}
	}
	prompt := "    Type the 4 letters you see (KEKA/UPCI), or Q to quit:"
	q := app.NewTestQuestion(app.QText, prompt, false)
	a := ask(q, textAnswer(" upci "))
	b, good := flasher.ParseBuild(a.Text, []string{"KEKA", "UPCI"})
	check(!a.Cancel && a.Text == " upci " && good && b == "UPCI", "window question (software version, no phone): typed \" upci \" + Enter -> flasher gets %q -> %q", a.Text, b)
	check(w.m.Question == nil, "the question is closed after the answer")
	q = app.NewTestQuestion(app.QWord, "    Type FLASH and press Enter to start (anything else stops):", false)
	a = ask(q, textAnswer("FLASH"))
	check(!a.Cancel && a.Text == "FLASH", "window question (type a word): FLASH -> %q", a.Text)
	q = app.NewTestQuestion(app.QYesNo, "    Backup failed: continue flashing WITHOUT a backup?", false)
	a = ask(q, msgBox(win.IDYES))
	check(a.Yes && !a.Cancel, "window yes/no (backup failed, continue?): Yes -> yes=%v", a.Yes)
	q = app.NewTestQuestion(app.QYesNo, "    Backup failed: continue flashing WITHOUT a backup?", false)
	a = ask(q, msgBox(win.IDNO))
	check(!a.Yes && !a.Cancel, "window yes/no: No -> yes=%v", a.Yes)
	// 1.4.3 never block (Sam, Nov 4): the variant warning box shows what was detected, then
	// "Flash anyway as <choice>?" (default No). Yes goes on, No stops.
	for _, l := range []string{"WARNING - THE PHONE COULD NOT BE CONFIRMED AS A SUPPORTED ONE.",
		`this phone reports product "gflip65". This ROM is made for product gflip6 (TCL Flip 2 T408DL KEKA / 4058G UPCI).`} {
		w.m.Apply(app.Msg{Kind: app.MsgLine, Line: app.Line{Kind: app.LineWarn, Text: l}})
	}
	q = app.NewTestQuestion(app.QYesNo, "    Flash anyway as UPCI? (your choice; Enter = No, stop)", false)
	a = ask(q, msgBox(win.IDYES))
	check(a.Yes && !a.Cancel && strings.Contains(strings.Join(q.Context, " "), "gflip65"),
		"variant warning box (shows the detected gflip65): Yes -> yes=%v, cancel=%v", a.Yes, a.Cancel)
	q = app.NewTestQuestion(app.QYesNo, "    Flash anyway as UPCI? (your choice; Enter = No, stop)", false)
	a = ask(q, msgBox(win.IDNO))
	check(!a.Yes && !a.Cancel, "variant warning box: No -> yes=%v (the flasher stops)", a.Yes)
	q = app.NewTestQuestion(app.QYesNo, "    Try again?", true)
	a = ask(q, msgBox(win.IDYES))
	check(a.Yes, "window yes/no (try again?): Yes -> yes=%v", a.Yes)
	q = app.NewTestQuestion(app.QEnter, "    Press Enter when the phone is OFF and UNPLUGGED...", false)
	a = ask(q, msgBox(win.IDOK))
	check(!a.Cancel, "window OK box (phone off + unplugged): OK -> cancel=%v", a.Cancel)
	q = app.NewTestQuestion(app.QEnter, "    Press Enter when the phone is OFF and UNPLUGGED...", false)
	a = ask(q, msgBox(win.IDCANCEL))
	check(a.Cancel, "window OK box: Cancel -> cancel=%v", a.Cancel)

	// --- the backup folder box keeps spaces while typing ---
	w.m.Screen = app.ScreenOptions
	w.m.Plan.Backup = true
	w.m.SetBackupDir("")
	w.render()
	var typed1 string
	go func() {
		time.Sleep(600 * time.Millisecond)
		h := w.dirEdit.Handle()
		win.PostMessage(h, win.EM_SETSEL, 0, 0xFFFF)
		typeInto(h, `C:\My Backups\Shomer phone`)
		time.Sleep(400 * time.Millisecond)
		w.mw.Synchronize(func() { typed1 = w.m.Plan.BackupDir })
		time.Sleep(200 * time.Millisecond)
		// 1.4.3: a path pasted WITH quotes (Explorer's "Copy as path"), then Continue
		win.PostMessage(h, win.EM_SETSEL, 0, 0xFFFF)
		typeInto(h, `"D:\Pasted Folder\Shomer"`)
		time.Sleep(400 * time.Millisecond)
		w.mw.Synchronize(func() { w.onNext() }) // what the Continue button does
		time.Sleep(400 * time.Millisecond)
		w.mw.Synchronize(func() { w.quitting = true; w.mw.Close() })
	}()
	w.mw.Run()
	check(typed1 == `C:\My Backups\Shomer phone`, "backup folder typed with spaces -> %q", typed1)
	check(w.m.Plan.BackupDir == `D:\Pasted Folder\Shomer` && w.m.Screen == app.ScreenConfirm,
		"backup folder pasted with quotes, Continue -> %q (screen %v)", w.m.Plan.BackupDir, w.m.Screen)

	if fails > 0 {
		return fmt.Errorf("%d check(s) failed", fails)
	}
	return nil
}

func answerOf(q *app.Question) <-chan app.Answer {
	c := make(chan app.Answer, 1)
	go func() { c <- app.TestAnswer(q) }()
	return c
}

// findEdit finds the first Edit control anywhere under h.
func findEdit(h win.HWND) win.HWND {
	var found win.HWND
	cb := syscall.NewCallback(func(c win.HWND, _ uintptr) uintptr {
		buf := make([]uint16, 64)
		win.GetClassName(c, &buf[0], len(buf))
		if strings.EqualFold(syscall.UTF16ToString(buf), "Edit") {
			found = c
			return 0
		}
		return 1
	})
	win.EnumChildWindows(h, cb, 0)
	return found
}
