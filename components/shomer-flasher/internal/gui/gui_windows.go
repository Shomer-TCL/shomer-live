//go:build windows

package gui

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"github.com/Shomer-TCL/shomer-flasher/internal/app"
)

const SupportsNative = true

var (
	navy  = walk.RGB(0xF8, 0xF6, 0xEF)
	teal  = walk.RGB(0x07, 0x6C, 0x68)
	white = walk.RGB(0xFF, 0xFF, 0xFF)
	grey  = walk.RGB(0x58, 0x6B, 0x6A)
)

type window struct {
	subtitle, footer *walk.Label
	stageLabels      [6]*walk.Label
	identity         *walk.Composite
	identityValues   [4]*walk.Label
	cbLog            *walk.CheckBox
	m                *app.Model
	c                *app.Controller // nil in the smoke test

	mw                           *walk.MainWindow
	crumbs, title, progLabel     *walk.Label
	text, steps, log             *walk.TextEdit
	optComp, progComp            *walk.Composite
	cbBackup, cbUserdata, cbEras *walk.CheckBox
	dirEdit                      *walk.LineEdit
	browse, back, next, quit     *walk.PushButton
	help, phoneLbl               *walk.Label
	bar                          *walk.ProgressBar
	rendering                    bool
	quitting                     bool
}

func crlf(lines []string) string { return strings.Join(lines, "\r\n") }

func (w *window) build() error {
	font := Font{Family: "Segoe UI", PointSize: 10}
	sidebar := []Widget{Label{Text: "SHOMER", Font: Font{Family: "Segoe UI", PointSize: 18, Bold: true}, TextColor: teal}, Label{Text: "Flip 2 Workbench", TextColor: grey}, VSpacer{Size: 24}}
	for i, name := range stages {
		sidebar = append(sidebar, Label{AssignTo: &w.stageLabels[i], Text: fmt.Sprintf("%d   %s", i+1, name), MinSize: Size{Height: 40}, TextColor: grey})
	}
	sidebar = append(sidebar, VSpacer{}, Label{Text: "Offline toolkit", TextColor: grey}, Label{Text: w.m.Version, TextColor: grey})
	cards := []Widget{}
	for i, name := range []string{"DETECTED IDENTITY", "SELECTED IDENTITY", "CONNECTION MODE", "UNLOCK STATE"} {
		cards = append(cards, Composite{Background: SolidColorBrush{Color: walk.RGB(0xF0, 0xF3, 0xEA)}, Layout: VBox{Margins: Margins{Left: 16, Top: 12, Right: 16, Bottom: 12}}, MinSize: Size{Height: 98}, Children: []Widget{Label{Text: name, Font: Font{Family: "Consolas", PointSize: 9, Bold: true}, TextColor: teal}, Label{AssignTo: &w.identityValues[i], Text: "Not verified", TextColor: walk.RGB(0x16, 0x36, 0x35)}}})
	}
	return MainWindow{AssignTo: &w.mw, Title: "Shomer Flip 2 Workbench " + w.m.Version, Size: Size{Width: 960, Height: 600}, MinSize: Size{Width: 840, Height: 540}, Font: font, Background: SolidColorBrush{Color: navy}, Layout: VBox{Margins: Margins{}, Spacing: 0}, Children: []Widget{
		ScrollView{HorizontalFixed: true, StretchFactor: 1, MinSize: Size{Height: 100}, Layout: HBox{Margins: Margins{}, Spacing: 0}, Children: []Widget{
			Composite{MinSize: Size{Width: 190}, MaxSize: Size{Width: 210}, Background: SolidColorBrush{Color: navy}, Layout: VBox{Margins: Margins{Left: 20, Top: 26, Right: 14, Bottom: 20}, Spacing: 8}, Children: sidebar},
			Composite{Layout: VBox{Margins: Margins{Left: 24, Top: 24, Right: 24, Bottom: 16}, Spacing: 12}, Children: []Widget{
				Label{AssignTo: &w.crumbs, TextColor: grey, Font: Font{Family: "Consolas", PointSize: 9}},
				Label{AssignTo: &w.title, Font: Font{Family: "Consolas", PointSize: 21, Bold: true}, TextColor: walk.RGB(0x16, 0x36, 0x35)},
				Label{AssignTo: &w.subtitle, TextColor: grey},
				Composite{AssignTo: &w.identity, Visible: false, Layout: Grid{Columns: 2, Spacing: 12}, Children: cards},
				TextEdit{AssignTo: &w.text, ReadOnly: true, VScroll: true, Font: font},
				Composite{AssignTo: &w.optComp, Visible: false, Layout: Grid{Columns: 3, Spacing: 10}, Children: []Widget{
					Label{AssignTo: &w.phoneLbl, ColumnSpan: 3},
					CheckBox{AssignTo: &w.cbBackup, Text: "Recovery backup first (fallback when BROM is unavailable)", ColumnSpan: 3, OnClicked: func() { w.toggle(app.OptBackup) }},
					Label{Text: "Backup folder"},
					LineEdit{AssignTo: &w.dirEdit, OnTextChanged: func() {
						if !w.rendering {
							w.m.SetBackupDir(w.dirEdit.Text())
							w.render()
						}
					}},
					PushButton{AssignTo: &w.browse, Text: "Choose folder...", OnClicked: w.pickFolder},
					CheckBox{AssignTo: &w.cbUserdata, Text: "Include encrypted phone data (already unlocked phones only)", ColumnSpan: 3, OnClicked: func() { w.toggle(app.OptUserdata) }},
					CheckBox{AssignTo: &w.cbEras, Text: "Erase phone data after installation", ColumnSpan: 3, OnClicked: func() { w.toggle(app.OptErase) }},
					Label{Text: "Prefer a read-only MTKClient BROM backup when supported. The Linux live toolkit provides that route before this recovery fallback.", ColumnSpan: 3, TextColor: teal},
					Label{AssignTo: &w.help, ColumnSpan: 3, TextColor: grey}, VSpacer{ColumnSpan: 3},
				}},
				Composite{AssignTo: &w.progComp, Visible: false, Layout: VBox{MarginsZero: true, Spacing: 10}, Children: []Widget{
					TextEdit{AssignTo: &w.steps, ReadOnly: true, Font: font, MinSize: Size{Height: 180}},
					ProgressBar{AssignTo: &w.bar, MaxValue: 100, MinSize: Size{Height: 20}}, Label{AssignTo: &w.progLabel},
					CheckBox{AssignTo: &w.cbLog, Text: "Show activity log", OnClicked: func() { w.log.SetVisible(w.cbLog.Checked()) }},
					TextEdit{AssignTo: &w.log, ReadOnly: true, VScroll: true, Font: Font{Family: "Consolas", PointSize: 9}},
				}},
			}},
		}},
		Composite{Background: SolidColorBrush{Color: navy}, Layout: HBox{Margins: Margins{Left: 20, Top: 14, Right: 24, Bottom: 16}, Spacing: 12}, Children: []Widget{
			Label{AssignTo: &w.footer, Text: "Works offline. Keep your backups private.", TextColor: grey}, HSpacer{},
			PushButton{AssignTo: &w.quit, Text: "Quit", OnClicked: func() { w.mw.Close() }},
			PushButton{AssignTo: &w.back, Text: "< Back", OnClicked: func() { w.m.Back(); w.render() }},
			PushButton{AssignTo: &w.next, Text: "Continue", MinSize: Size{Width: 220, Height: 38}, OnClicked: w.onNext},
		}},
	}}.Create()
}

func (w *window) toggle(id app.OptionID) {
	if w.rendering {
		return
	}
	w.m.Toggle(id)
	w.render()
}

func (w *window) pickFolder() {
	dlg := walk.FileDialog{Title: "Folder for the backup zip", InitialDirPath: w.m.Plan.BackupDir}
	if ok, err := dlg.ShowBrowseFolder(w.mw); err == nil && ok {
		w.m.SetBackupDir(dlg.FilePath)
		w.render()
	}
}

func (w *window) onNext() {
	switch w.m.Continue() {
	case app.ActDetect:
		if w.c != nil {
			w.c.Detect()
		}
	case app.ActFlash:
		if walk.MsgBox(w.mw, "Shomer flasher", w.m.FinalQuestion()+"\r\n\r\nDo NOT unplug the cable while it runs.",
			walk.MsgBoxYesNo|walk.MsgBoxIconWarning|walk.MsgBoxDefButton2) == walk.DlgCmdYes {
			w.m.StartFlash()
			if w.c != nil {
				w.c.Flash(w.m.Plan)
			}
		}
	case app.ActQuit:
		w.mw.Close()
	}
	w.render()
}

func stepText(m *app.Model) string {
	var l []string
	for _, s := range m.Steps {
		line := s.State.Mark() + " " + s.Title
		if s.Detail != "" && (s.State == app.StepFailed || s.State == app.StepSkipped) {
			line += "  (" + s.Detail + ")"
		}
		l = append(l, line)
	}
	return crlf(l)
}

func checksText(m *app.Model) string {
	var l []string
	for _, c := range m.Checks {
		tag := map[app.CheckStatus]string{app.CheckRunning: "[ .. ]", app.CheckOK: "[ OK ]", app.CheckInfo: "[info]", app.CheckWarn: "[WARN]", app.CheckFail: "[FAIL]"}[c.Status]
		l = append(l, tag+" "+c.Name+": "+c.Detail)
		if c.Fix != "" {
			l = append(l, "       "+c.Fix)
		}
		if c.ID == "rom" && c.Status == app.CheckRunning && m.Progress.Total > 0 {
			l = append(l, fmt.Sprintf("       %d%% %s", m.Progress.Percent(), m.Progress.Label))
		}
	}
	if m.Prepared && m.PrepErr == nil && !m.ChecksBlock() {
		l = append(l, "", "Next: the program helps you get the phone into fastboot mode. Nothing is written until you confirm.")
	}
	return crlf(l)
}

func logText(m *app.Model, n int) string {
	ls := m.Lines
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	var l []string
	for _, x := range ls {
		l = append(l, strings.TrimRight(x.Text, " "))
	}
	return crlf(l)
}

// render shows the model. Called on the UI thread only.
func (w *window) render() {
	m := w.m
	w.rendering = true
	defer func() { w.rendering = false }()
	p := present(m)
	w.crumbs.SetText(fmt.Sprintf("STEP %d OF 6", m.Screen+1))
	w.title.SetText(p.Title)
	w.subtitle.SetText(p.Subtitle)
	w.footer.SetText(p.Footer)
	for i, name := range stages {
		w.stageLabels[i].SetText(fmt.Sprintf("%d   %s", i+1, name))
		color := grey
		if i == int(m.Screen) {
			color = teal
		}
		w.stageLabels[i].SetTextColor(color)
	}
	w.identity.SetVisible(m.Screen == app.ScreenPhone && m.Phone != nil)
	for i, value := range p.Cards {
		w.identityValues[i].SetText(known(value))
	}
	w.log.SetVisible(w.cbLog.Checked())

	w.text.SetVisible(m.Screen != app.ScreenProgress && m.Screen != app.ScreenOptions)
	w.optComp.SetVisible(m.Screen == app.ScreenOptions)
	w.progComp.SetVisible(m.Screen == app.ScreenProgress)
	switch m.Screen {
	case app.ScreenChecks:
		w.text.SetText(strings.ReplaceAll(p.Body, "\n", "\r\n"))
	case app.ScreenPhone:
		w.text.SetText(strings.ReplaceAll(p.Body, "\n", "\r\n"))
	case app.ScreenOptions:
		w.phoneLbl.SetText("Phone: " + m.PhoneSummary())
		rows := m.Options()
		w.cbBackup.SetChecked(m.Plan.Backup)
		if strings.TrimSpace(w.dirEdit.Text()) != m.Plan.BackupDir {
			w.dirEdit.SetText(m.Plan.BackupDir)
		}
		w.dirEdit.SetEnabled(rows[1].Enabled)
		w.browse.SetEnabled(rows[1].Enabled)
		w.cbUserdata.SetChecked(rows[2].Value == "ON")
		w.cbUserdata.SetEnabled(rows[2].Enabled)
		w.cbEras.SetChecked(rows[3].Value == "ON")
		w.cbEras.SetEnabled(rows[3].Enabled)
		w.help.SetText(rows[0].Help + "\r\n" + rows[2].Help + "\r\n" + rows[3].Help)
	case app.ScreenConfirm:
		w.text.SetText(crlf(m.ConfirmLines()))
	case app.ScreenProgress:
		w.steps.SetText(stepText(m))
		w.bar.SetValue(m.Progress.Percent())
		if m.Progress.Total > 0 {
			w.progLabel.SetText(fmt.Sprintf("%d%%  %s (%d / %d MB)", m.Progress.Percent(), m.Progress.Label, m.Progress.Done>>20, m.Progress.Total>>20))
		} else {
			w.progLabel.SetText("")
		}
		w.log.SetText(logText(m, 200))
		w.log.SendMessage(0x0115, 7, 0) // WM_VSCROLL, SB_BOTTOM
	case app.ScreenDone:
		w.text.SetText(crlf(m.DoneLines()))
	}
	w.next.SetText(m.ContinueLabel())
	w.next.SetEnabled(m.CanContinue())
	w.back.SetVisible(m.Screen == app.ScreenOptions || m.Screen == app.ScreenConfirm)
	w.quit.SetVisible(m.Screen != app.ScreenDone)
}

func dashed(s string) string {
	if s == "" || s == "skipped" {
		return "-"
	}
	return s
}

// ask shows a flasher question as a dialog (UI thread).
func (w *window) ask(q *app.Question) {
	body := strings.Join(trimAll(q.Context), "\r\n")
	if body != "" {
		body += "\r\n\r\n"
	}
	body += q.CleanPrompt()
	switch q.Kind {
	case app.QEnter:
		r := walk.MsgBox(w.mw, "Shomer flasher", body+"\r\n\r\nOK when done. Cancel stops the flasher.", walk.MsgBoxOKCancel|walk.MsgBoxIconInformation)
		w.m.Answer(app.Answer{Cancel: r != walk.DlgCmdOK})
	case app.QYesNo:
		style := walk.MsgBoxYesNo | walk.MsgBoxIconQuestion
		if !q.Default {
			style |= walk.MsgBoxDefButton2
		}
		if lp := strings.ToLower(q.Prompt); strings.Contains(lp, "failed") || strings.Contains(lp, "flash anyway") {
			style = walk.MsgBoxYesNo | walk.MsgBoxIconWarning | walk.MsgBoxDefButton2
		}
		w.m.Answer(app.Answer{Yes: walk.MsgBox(w.mw, "Shomer flasher", body, style) == walk.DlgCmdYes})
	default:
		text, ok := textDialog(w.mw, body)
		w.m.Answer(app.Answer{Text: text, Cancel: !ok})
	}
	w.render()
}

func trimAll(l []string) []string {
	var o []string
	for _, s := range l {
		o = append(o, strings.TrimSpace(s))
	}
	return o
}

// dialogHook, when set (tests only), is called once the text dialog exists and
// before it runs, with its edit box and buttons.
var dialogHook func(dlg *walk.Dialog, le *walk.LineEdit, ok, cancel *walk.PushButton)

// textDialog asks for a typed answer. 1.4.1 read the edit box after Run()
// returned, when Windows has already destroyed the dialog and its edit box,
// so the answer was always "" (Sam, Nov 4 2026: typed UPCI, got ""). The text
// is now read while the dialog is still open: on OK/Enter and, as a second
// net, on every change and when the dialog starts closing.
func textDialog(owner walk.Form, prompt string) (string, bool) {
	var dlg *walk.Dialog
	var le *walk.LineEdit
	var ok, cancel *walk.PushButton
	typed := ""
	accepted := false
	err := Dialog{
		AssignTo: &dlg, Title: "Shomer flasher", DefaultButton: &ok, CancelButton: &cancel,
		MinSize: Size{Width: 460, Height: 200}, Layout: VBox{},
		Children: []Widget{
			Label{Text: prompt},
			LineEdit{AssignTo: &le, OnTextChanged: func() { typed = le.Text() }},
			Composite{Layout: HBox{}, Children: []Widget{
				HSpacer{},
				PushButton{AssignTo: &ok, Text: "OK", OnClicked: func() {
					typed, accepted = le.Text(), true
					dlg.Accept()
				}},
				PushButton{AssignTo: &cancel, Text: "Cancel", OnClicked: func() { dlg.Cancel() }},
			}},
		},
	}.Create(owner)
	if err != nil {
		return "", false
	}
	dlg.Closing().Attach(func(_ *bool, _ walk.CloseReason) {
		if le != nil && !le.IsDisposed() {
			typed = le.Text()
		}
	})
	le.SetFocus()
	if dialogHook != nil {
		dialogHook(dlg, le, ok, cancel)
	}
	res := dlg.Run()
	return typed, accepted || res == walk.DlgCmdOK
}

func (w *window) onClosing(canceled *bool, _ walk.CloseReason) {
	if w.quitting {
		return
	}
	if w.m.QuitNeedsConfirm() && walk.MsgBox(w.mw, "Shomer flasher", app.QuitWarning, walk.MsgBoxYesNo|walk.MsgBoxIconWarning|walk.MsgBoxDefButton2) != walk.DlgCmdYes {
		*canceled = true
		return
	}
	w.quitting = true
	if w.c != nil {
		w.c.Stop()
	}
}

// Run shows the window until it is closed.
// ShowError shows a stopping problem before the window exists (the console
// of a double-clicked exe is hidden, so printing it would show nothing).
func ShowError(title, text string) {
	walk.MsgBox(nil, title, strings.ReplaceAll(text, "\n", "\r\n"), walk.MsgBoxOK|walk.MsgBoxIconError)
}

func Run(m *app.Model, c *app.Controller) error {
	w := &window{m: m, c: c}
	if err := w.build(); err != nil {
		return err
	}
	w.mw.Closing().Attach(w.onClosing)
	w.render()
	c.Start()
	go func() {
		for msg := range c.Msgs() {
			msg := msg
			w.mw.Synchronize(func() {
				if w.quitting {
					return
				}
				w.m.Apply(msg)
				w.render()
				if msg.Kind == app.MsgQuestion && w.m.Question != nil {
					w.ask(w.m.Question)
				}
			})
		}
	}()
	w.mw.Run()
	return nil
}

// Smoke opens the window, shows every demo screen and closes again. With dir
// set, each screen is saved there as a PNG (README screenshots). For CI.
func Smoke(m *app.Model, dir string) error {
	w := &window{m: m}
	if err := w.build(); err != nil {
		return err
	}
	w.render()
	errc := make(chan error, 1)
	go func() {
		var err error
		time.Sleep(800 * time.Millisecond)
		for i, s := range app.DemoScreens(m.Plan.BackupDir) {
			s := s
			if s.Name == "backup-failed" { // a dialog: not part of the window capture
				w.mw.Synchronize(func() { s.Setup(w.m) })
				continue
			}
			done := make(chan struct{})
			w.mw.Synchronize(func() {
				s.Setup(w.m)
				w.render()
				close(done)
			})
			<-done
			time.Sleep(500 * time.Millisecond)
			if dir != "" {
				cap := make(chan error, 1)
				name := filepath.Join(dir, fmt.Sprintf("gui-%d-%s.png", i+1, s.Name))
				w.mw.Synchronize(func() { cap <- capture(w.mw, name) })
				if e := <-cap; e != nil && err == nil {
					err = e
				}
			}
		}
		w.mw.Synchronize(func() { w.quitting = true; w.mw.Close() })
		errc <- err
	}()
	w.mw.Run()
	return <-errc
}

func capture(mw *walk.MainWindow, path string) error {
	bmp, err := walk.NewBitmapFromWindow(mw)
	if err != nil {
		return err
	}
	defer bmp.Dispose()
	img, err := bmp.ToImage()
	if err != nil {
		return err
	}
	out := image.NewRGBA(img.Bounds())
	for i := 0; i < len(img.Pix); i += 4 {
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = img.Pix[i], img.Pix[i+1], img.Pix[i+2], 255
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, out)
}
