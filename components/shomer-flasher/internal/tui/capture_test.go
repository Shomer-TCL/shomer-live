//go:build !windows

package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/Shomer-TCL/shomer-flasher/internal/app"
	"github.com/Shomer-TCL/shomer-flasher/internal/flasher"
)

// TestDemoScreens renders every screen with example data. With
// CAPTURE_DIR set it writes them as text (docs/tui-*.txt for the README).
func TestDemoScreens(t *testing.T) {
	m := app.NewModel(app.DemoManifest(), "1.4.1", flasher.Plan{Backup: true, BackupDir: "/home/sam/shomer-backups", EraseData: true},
		"/home/sam/Downloads/shomer-flasher-20261103-101500.log")
	tt := New(m, nil)
	tt.w, tt.h = 100, 34
	dir := os.Getenv("CAPTURE_DIR")
	for i, s := range app.DemoScreens("/home/sam/shomer-backups") {
		s.Setup(m)
		if s.Name == "options" || s.Name == "confirm" {
			tt.cur = 0
		}
		v := ansi.Strip(tt.View())
		lines := strings.Split(v, "\n")
		if len(lines) > 34 {
			t.Fatalf("%s: %d lines", s.Name, len(lines))
		}
		for _, l := range lines {
			if ansi.StringWidth(l) > 100 {
				t.Fatalf("%s: line too wide: %q", s.Name, l)
			}
		}
		if dir != "" {
			var out []string
			for _, l := range lines {
				out = append(out, strings.TrimRight(l, " "))
			}
			txt := strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("tui-%d-%s.txt", i+1, s.Name)), []byte(txt), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}
