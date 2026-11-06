package flasher

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Runner runs fastboot and adb. Real runs use ExecRunner; tests use fakes.
type Runner interface {
	// Run runs tool (fastboot|adb) with args and returns combined output.
	// stream (may be nil) receives output as it arrives.
	Run(ctx context.Context, tool string, args []string, stream io.Writer) (string, error)
}

// toolWaitDelay: how long Run waits for the output pipes to close after the
// tool itself has ended or been killed (a var so tests can shorten it).
var toolWaitDelay = 10 * time.Second

// ExecRunner runs real binaries.
type ExecRunner struct {
	Paths map[string]string // "fastboot" -> path, "adb" -> path
}

func (r *ExecRunner) Run(ctx context.Context, tool string, args []string, stream io.Writer) (string, error) {
	p, ok := r.Paths[tool]
	if !ok {
		return "", fmt.Errorf("unknown tool %s", tool)
	}
	cmd := exec.CommandContext(ctx, p, args...)
	hideWindow(cmd)
	var buf bytes.Buffer
	var w io.Writer = &buf
	if stream != nil {
		w = io.MultiWriter(&buf, stream)
	}
	cmd.Stdout = w
	cmd.Stderr = w
	// A child the tool leaves behind (adb starting its server is the classic
	// case) can keep the output pipe open; without WaitDelay Run would wait for
	// it forever and the timeout above would never end the call.
	cmd.WaitDelay = toolWaitDelay
	err := cmd.Run()
	return buf.String(), err
}

// RunStream runs a tool with its standard output going straight to w (binary safe).
func (r *ExecRunner) RunStream(ctx context.Context, tool string, args []string, w io.Writer) (string, error) {
	p, ok := r.Paths[tool]
	if !ok {
		return "", fmt.Errorf("unknown tool %s", tool)
	}
	cmd := exec.CommandContext(ctx, p, args...)
	hideWindow(cmd)
	var errb bytes.Buffer
	cmd.Stdout = w
	cmd.Stderr = &errb
	cmd.WaitDelay = toolWaitDelay
	err := cmd.Run()
	return errb.String(), err
}

// Device is one line of `fastboot devices` / `adb devices`.
type Device struct {
	Serial string
	State  string
}

var spaceRe = regexp.MustCompile(`\s+`)

func parseFastbootDevices(out string) []Device {
	var ds []Device
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "*") || strings.HasPrefix(l, "<") {
			continue
		}
		f := spaceRe.Split(l, -1)
		if len(f) < 2 {
			continue
		}
		if f[1] == "fastboot" || f[1] == "no" || f[1] == "fastbootd" {
			st := f[1]
			if f[1] == "no" { // "no permissions; see ..."
				st = "no-permissions"
			}
			ds = append(ds, Device{Serial: f[0], State: st})
		}
	}
	return ds
}

func parseAdbDevices(out string) []Device {
	var ds []Device
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "List of devices") || strings.HasPrefix(l, "*") || strings.HasPrefix(l, "adb ") {
			continue
		}
		f := spaceRe.Split(l, -1)
		if len(f) < 2 {
			continue
		}
		st := f[1]
		if st == "no" {
			st = "no-permissions"
		}
		ds = append(ds, Device{Serial: f[0], State: st})
	}
	return ds
}

// parseGetvar finds "name: value" in fastboot getvar output (fastboot prints to stderr).
func parseGetvar(out, name string) (string, bool) {
	// [ \t]* (not \s*): an empty answer ("product: " then a new line) must stay
	// empty, not take the next line ("Finished. Total time...") as the value.
	re := regexp.MustCompile(`(?mi)^[ \t]*\(?(?:bootloader\)[ \t]*)?` + regexp.QuoteMeta(name) + `:[ \t]*(.*?)[ \t\r]*$`)
	m := re.FindStringSubmatch(out)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[1]), true
}

// UnlockState is what the bootloader says about itself.
type UnlockState int

const (
	UnlockUnknown UnlockState = iota
	Locked
	Unlocked
)

func (s UnlockState) String() string {
	switch s {
	case Locked:
		return "LOCKED"
	case Unlocked:
		return "UNLOCKED"
	}
	return "unknown"
}

func parseUnlocked(getvarOut, deviceInfoOut string) UnlockState {
	if v, ok := parseGetvar(getvarOut, "unlocked"); ok {
		switch strings.ToLower(v) {
		case "yes", "true", "1":
			return Unlocked
		case "no", "false", "0":
			return Locked
		}
	}
	// MediaTek/Qualcomm style: "(bootloader) Device unlocked: true"
	re := regexp.MustCompile(`(?mi)device unlocked:\s*(\w+)`)
	if m := re.FindStringSubmatch(deviceInfoOut); m != nil {
		switch strings.ToLower(m[1]) {
		case "true", "yes":
			return Unlocked
		case "false", "no":
			return Locked
		}
	}
	return UnlockUnknown
}

func withTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
