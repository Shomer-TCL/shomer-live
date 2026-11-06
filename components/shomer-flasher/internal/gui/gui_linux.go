//go:build linux && gtk

package gui

/*
#cgo pkg-config: gtk+-3.0
#include <stdlib.h>
#include "gtk_linux.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"github.com/Shomer-TCL/shomer-flasher/internal/app"
)

const SupportsNative = true

func cs(s string) (*C.char, func()) {
	p := C.CString(s)
	return p, func() { C.free(unsafe.Pointer(p)) }
}
func yes(v bool) C.int {
	if v {
		return 1
	}
	return 0
}
func setText(u *C.ShomerUI, field int, s string) {
	p, free := cs(s)
	defer free()
	C.shomer_ui_text(u, C.int(field), p)
}
func create(m *app.Model) (*C.ShomerUI, error) {
	p, free := cs(m.Version)
	defer free()
	u := C.shomer_ui_new(p)
	if u == nil {
		return nil, errors.New("GTK could not open the desktop display")
	}
	return u, nil
}
func render(u *C.ShomerUI, m *app.Model) {
	p := present(m)
	setText(u, 0, fmt.Sprintf("STEP %d OF 6", m.Screen+1))
	setText(u, 1, p.Title)
	setText(u, 2, p.Subtitle)
	setText(u, 3, p.Body)
	setText(u, 4, activity(m, 200))
	setText(u, 5, p.Footer)
	for i, v := range p.Cards {
		setText(u, i+6, known(v))
	}
	label, free := cs(m.ContinueLabel())
	defer free()
	dir, freeDir := cs(m.Plan.BackupDir)
	defer freeDir()
	_, bromErr := exec.LookPath("shomer-workbench")
	C.shomer_ui_state(u, C.int(m.Screen), yes(m.CanContinue()), label, yes(m.Plan.Backup), yes(m.Plan.Userdata), yes(m.Plan.EraseData), yes(m.PhoneUnlocked()), dir, C.int(m.Progress.Percent()), yes(bromErr == nil && !m.Busy() && m.Screen == app.ScreenChecks))
}
func question(u *C.ShomerUI, kind app.QKind, body string, defaultYes bool) app.Answer {
	p, free := cs(body)
	defer free()
	var answer *C.char
	r := C.shomer_ui_question(u, C.int(kind), p, yes(defaultYes), &answer)
	a := app.Answer{Yes: r == C.GTK_RESPONSE_YES, Cancel: r == C.GTK_RESPONSE_CANCEL || r == C.GTK_RESPONSE_DELETE_EVENT}
	if answer != nil {
		a.Text = C.GoString(answer)
		C.g_free(C.gpointer(answer))
	}
	return a
}
func capture(u *C.ShomerUI, path string) error {
	p, free := cs(path)
	defer free()
	if C.shomer_ui_capture(u, p) == 0 {
		return fmt.Errorf("cannot capture GTK window: %s", path)
	}
	return nil
}
func pumpFor(d time.Duration) {
	until := time.Now().Add(d)
	for time.Now().Before(until) {
		C.shomer_ui_pump()
		time.Sleep(10 * time.Millisecond)
	}
}

func Run(m *app.Model, c *app.Controller) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	u, err := create(m)
	if err != nil {
		return err
	}
	defer C.shomer_ui_destroy(u)
	render(u, m)
	c.Start()
	for {
		C.shomer_ui_pump()
		select {
		case msg := <-c.Msgs():
			m.Apply(msg)
			render(u, m)
			if msg.Kind == app.MsgQuestion && m.Question != nil {
				q := m.Question
				m.Answer(question(u, q.Kind, strings.Join(q.Context, "\n")+"\n\n"+q.CleanPrompt(), q.Default))
				render(u, m)
			}
		default:
		}
		switch int(C.shomer_ui_event(u)) {
		case 1:
			switch m.Continue() {
			case app.ActDetect:
				c.Detect()
			case app.ActFlash:
				if question(u, app.QYesNo, m.FinalQuestion(), false).Yes {
					m.StartFlash()
					c.Flash(m.Plan)
				}
			case app.ActQuit:
				c.Stop()
				return nil
			}
			render(u, m)
		case 2:
			m.Back()
			render(u, m)
		case 3:
			if !m.QuitNeedsConfirm() || question(u, app.QYesNo, app.QuitWarning, false).Yes {
				c.Stop()
				return nil
			}
		case 4:
			p, free := cs(m.Plan.BackupDir)
			chosen := C.shomer_ui_folder(u, p)
			free()
			if chosen != nil {
				m.SetBackupDir(C.GoString(chosen))
				C.g_free(C.gpointer(chosen))
				render(u, m)
			}
		case 5:
			m.Toggle(app.OptBackup)
			render(u, m)
		case 6:
			m.Toggle(app.OptUserdata)
			render(u, m)
		case 7:
			m.Toggle(app.OptErase)
			render(u, m)
		case 8:
			if !m.Busy() {
				cmd := exec.Command("shomer-workbench")
				if err := cmd.Start(); err != nil {
					question(u, app.QEnter, err.Error(), false)
				} else {
					go cmd.Wait()
				}
			}
		case 9:
			m.SetBackupDir(C.GoString(C.shomer_ui_directory(u)))
			render(u, m)
		}
		time.Sleep(15 * time.Millisecond)
	}
}

func Smoke(m *app.Model, dir string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	u, err := create(m)
	if err != nil {
		return err
	}
	defer C.shomer_ui_destroy(u)
	if dir != "" {
		if err = os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	for i, s := range app.DemoScreens(m.Plan.BackupDir) {
		if s.Name == "backup-failed" {
			s.Setup(m)
			continue
		}
		s.Setup(m)
		render(u, m)
		pumpFor(180 * time.Millisecond)
		if dir != "" {
			if err = capture(u, filepath.Join(dir, fmt.Sprintf("gui-%d-%s.png", i+1, s.Name))); err != nil {
				return err
			}
		}
	}
	return nil
}
func SelfTest(m *app.Model, out io.Writer) error {
	if err := Smoke(m, ""); err != nil {
		return err
	}
	fmt.Fprintln(out, "PASS native GTK renders all shared controller states")
	return nil
}
func ShowError(title, text string) { fmt.Fprintln(os.Stderr, title+": "+text) }
