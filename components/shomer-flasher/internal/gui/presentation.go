package gui

import (
	"fmt"
	"strings"

	"github.com/Shomer-TCL/shomer-flasher/internal/app"
)

// Shared presentation is used by native GTK on Linux and native Win32 on Windows.
// Both screens consume the existing app.Model and Controller; neither flashes.
var stages = [...]string{"Check images", "Identify phone", "Back up", "Review", "Install", "Results"}

type presentation struct {
	Title, Subtitle, Body, Footer string
	Cards                         [4]string
}

func present(m *app.Model) presentation {
	p := presentation{Footer: "Works offline. Keep your phone-specific backups private."}
	switch m.Screen {
	case app.ScreenChecks:
		p.Title = "Check your images"
		p.Subtitle = "Verify the bundled ROM and this computer before connecting the phone."
		var lines []string
		for _, c := range m.Checks {
			lines = append(lines, c.Status.String()+"  "+c.Name+"\n"+c.Detail)
			if c.Fix != "" {
				lines = append(lines, "Next: "+c.Fix)
			}
		}
		p.Body = strings.Join(lines, "\n\n")
	case app.ScreenPhone:
		p.Title = "Identify your phone"
		p.Subtitle = "Detected identity and the version you entered are separate facts."
		if m.Phone == nil {
			p.Body = "Follow the connection instructions below. Nothing is written until you review and confirm.\n\n" + activity(m, 40)
		} else {
			ph := m.Phone
			p.Cards = [4]string{fmt.Sprintf("Model: %s\nBuild: %s\nSerial: %s", known(ph.Model), known(ph.DetectedBuild), known(ph.Serial)),
				fmt.Sprintf("Selected build: %s\nSource: %s", known(ph.Build), known(ph.BuildSource)),
				known(string(ph.Mode)), ph.Unlock.String()}
			p.Body = "The mode is queried independently from the unlock state. An unknown mode stops partition writes.\n\n" + m.PhoneSummary()
		}
	case app.ScreenOptions:
		p.Title = "Back up and choose options"
		p.Subtitle = "Prefer a read-only BROM backup before unlocking, when supported."
		p.Body = "The recovery fallback writes a temporary rooted recovery, copies readable partitions, then restores stock recovery. Unlocking erases existing phone data. An encrypted userdata dump is not a promise of usable personal data."
	case app.ScreenConfirm:
		p.Title = "Review every partition write"
		p.Subtitle = "Check the phone, image release and backup destination before confirming."
		p.Body = strings.Join(m.ConfirmLines(), "\n")
	case app.ScreenProgress:
		p.Title = "Installation in progress"
		p.Subtitle = "Keep the USB cable connected. Read any prompts before answering."
		var lines []string
		for _, s := range m.Steps {
			line := s.State.Mark() + "  " + s.Title
			if s.Detail != "" {
				line += "\n    " + s.Detail
			}
			lines = append(lines, line)
		}
		p.Body = strings.Join(lines, "\n\n")
	case app.ScreenDone:
		p.Title = "Installation finished"
		if !m.Result.OK {
			p.Title = "Stopped — review the result"
		}
		p.Subtitle = "Only completed operations are reported as successful."
		p.Body = strings.Join(m.DoneLines(), "\n")
	}
	return p
}

func known(s string) string {
	if strings.TrimSpace(s) == "" || s == "skipped" {
		return "Not verified"
	}
	return s
}
func activity(m *app.Model, n int) string {
	ls := m.Lines
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	var lines []string
	for _, l := range ls {
		lines = append(lines, l.Text)
	}
	return strings.Join(lines, "\n")
}
