//go:build !windows && (!linux || !gtk)

package gui

import (
	"errors"
	"io"

	"github.com/Shomer-TCL/shomer-flasher/internal/app"
)

// ErrNoGUI: the window exists on Windows only (Linux has the TUI).
var ErrNoGUI = errors.New("the window interface is only built for Windows; use the TUI or --cli")

const SupportsNative = false

func Run(m *app.Model, c *app.Controller) error { return ErrNoGUI }

func Smoke(m *app.Model, dir string) error { return ErrNoGUI }

func SelfTest(m *app.Model, out io.Writer) error { return ErrNoGUI }

// ShowError: Windows only (a message box); elsewhere the console shows it.
func ShowError(title, text string) {}
