//go:build !windows

package flasher

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
