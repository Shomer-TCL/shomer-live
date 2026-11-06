package flasher

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ConnectionMode is observed evidence, never inferred from an unlocked flag or
// from merely appearing in `fastboot devices` (both fastboot modes appear there).
type ConnectionMode string

const (
	ModeUnknown    ConnectionMode = "unknown"
	ModeBootloader ConnectionMode = "bootloader fastboot"
	ModeFastbootd  ConnectionMode = "fastbootd"
	ModeAndroid    ConnectionMode = "Android ADB"
	ModeRecovery   ConnectionMode = "recovery ADB"
)

func parseFastbootMode(out string, err error) ConnectionMode {
	if err != nil || fastbootFailed(out) {
		return ModeUnknown
	}
	v, ok := parseGetvar(out, "is-userspace")
	if !ok {
		return ModeUnknown
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "yes":
		return ModeFastbootd
	case "no":
		return ModeBootloader
	default:
		return ModeUnknown
	}
}

func (f *Flow) fastbootMode() ConnectionMode {
	ds := f.fastbootDevices()
	if len(ds) != 1 {
		return ModeUnknown
	}
	out, err := f.fastboot(20*time.Second, false, "getvar", "is-userspace")
	mode := parseFastbootMode(out, err)
	f.UI.Log("connection mode: %s", mode)
	f.inFastbootd = mode == ModeFastbootd
	return mode
}

// requireMode verifies the current mode and, only from a known other fastboot
// mode, requests and verifies a transition. Unsupported queries fail closed.
func (f *Flow) requireMode(want ConnectionMode) error {
	got := f.fastbootMode()
	if got == want {
		return nil
	}
	if got == ModeUnknown {
		return refuse("cannot verify the phone's fastboot mode (is-userspace missing, unsupported or failed). No partition write was attempted by this step; keep the log and diagnose the mode")
	}
	target := "bootloader"
	if want == ModeFastbootd {
		target = "fastboot"
	}
	f.UI.Step("Changing from %s to %s", got, want)
	out, err := f.fastboot(time.Minute, true, "reboot", target)
	if err != nil || fastbootFailed(out) {
		return fmt.Errorf("could not request %s: %s", want, lastLines(out, 4))
	}
	wait := f.FastbootWait
	if wait <= 0 {
		wait = 3 * time.Minute
	}
	deadline := time.Now().Add(wait)
	for {
		if devices := f.fastbootDevices(); len(devices) > 1 {
			return refuse("more than one phone in fastboot after mode transition")
		}
		got = f.fastbootMode()
		if got == want {
			return nil
		}
		if time.Now().After(deadline) {
			return refuse("requested %s but observed %s; no partition write was attempted by this step", want, got)
		}
		f.sleep(500 * time.Millisecond)
	}
}

func (f *Flow) checkRecoveryPartition() error {
	out, err := f.fastboot(20*time.Second, false, "getvar", "partition-size:recovery")
	value, ok := parseGetvar(out, "partition-size:recovery")
	if err != nil || fastbootFailed(out) || !ok {
		return refuse("cannot verify recovery partition availability and size; no recovery write attempted: %s", lastLines(out, 4))
	}
	n, parseErr := strconv.ParseInt(strings.TrimSpace(value), 0, 64)
	if parseErr != nil || n != RecoverySize {
		return refuse("recovery partition size %q does not match the pinned %d-byte image; no recovery write attempted", value, RecoverySize)
	}
	return nil
}
