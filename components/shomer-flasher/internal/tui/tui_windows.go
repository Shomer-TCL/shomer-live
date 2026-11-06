//go:build windows

// Package tui is the full-screen text interface (Linux only; Windows has the window).
package tui

import (
	"errors"

	"github.com/Shomer-TCL/shomer-flasher/internal/app"
)

// Run is not built on Windows (the window is the interface there).
func Run(m *app.Model, c *app.Controller) error {
	return errors.New("the text interface is built for Linux only")
}
