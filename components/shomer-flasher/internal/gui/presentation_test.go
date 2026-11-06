package gui

import (
	"github.com/Shomer-TCL/shomer-flasher/internal/app"
	"github.com/Shomer-TCL/shomer-flasher/internal/flasher"
	"strings"
	"testing"
)

func TestEnteredIdentityIsNeverPresentedAsDetectedIdentity(t *testing.T) {
	m := app.NewModel(app.DemoManifest(), "test", flasher.Plan{}, "")
	m.Screen = app.ScreenPhone
	m.Phone = &flasher.PhoneInfo{Build: "UPCI", BuildSource: "typed", Mode: flasher.ModeUnknown}
	p := present(m)
	if strings.Contains(p.Cards[0], "UPCI") || !strings.Contains(p.Cards[0], "Not verified") {
		t.Fatal(p.Cards)
	}
	if !strings.Contains(p.Cards[1], "typed") || !strings.Contains(p.Cards[1], "UPCI") {
		t.Fatal(p.Cards)
	}
	if p.Cards[2] != "unknown" {
		t.Fatal(p.Cards)
	}
}
func TestFailureDoesNotUseSuccessfulResultHeading(t *testing.T) {
	m := app.NewModel(app.DemoManifest(), "test", flasher.Plan{}, "")
	m.Screen = app.ScreenDone
	if strings.Contains(present(m).Title, "finished") {
		t.Fatal(present(m).Title)
	}
	m.Result.OK = true
	if present(m).Title != "Installation finished" {
		t.Fatal(present(m).Title)
	}
}
