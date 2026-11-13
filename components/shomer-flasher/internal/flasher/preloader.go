package flasher

import (
	"errors"
	"io"
	"strings"
	"time"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// The TCL Flip 2 family (MT6739) does not go to fastboot with
// `adb reboot bootloader`. The only way in is the preloader: on every power-up
// it shows a USB serial port (MediaTek VID 0e8d, PID 2000) for about 2-3
// seconds. If the 8 ASCII bytes "FASTBOOT" arrive on that port in time, it
// answers "READYTOO" and starts the bootloader in fastboot mode.
// Sources: neutronscott/flip2 autobooter.py; jackharvest/tcl-flip-macos-unlock tools/bootseq.py.

const (
	PreloaderVID = "0E8D"
	PreloaderPID = "2000"
	BootSeq      = "FASTBOOT"
)

// ExpectedReply is "READY" + the last 3 characters of the boot sequence reversed.
func ExpectedReply(seq string) string {
	r := []rune(seq[len(seq)-3:])
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return "READY" + string(r)
}

// Preloader finds the preloader port and sends the boot sequence.
type Preloader interface {
	// Ports lists candidate serial ports right now.
	Ports() []string
	// Handshake opens port, sends seq, and returns the reply (8 bytes or fewer).
	Handshake(port, seq string, timeout time.Duration) (string, error)
}

// SerialPreloader is the real implementation.
type SerialPreloader struct {
	Fixed string // if set, only this port is used (--port)
}

func (s *SerialPreloader) Ports() []string {
	if s.Fixed != "" {
		return []string{s.Fixed}
	}
	list, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil
	}
	var out []string
	for _, p := range list {
		if p.IsUSB && strings.EqualFold(p.VID, PreloaderVID) && strings.EqualFold(p.PID, PreloaderPID) {
			out = append(out, p.Name)
		}
	}
	return out
}

func (s *SerialPreloader) Handshake(port, seq string, timeout time.Duration) (string, error) {
	p, err := serial.Open(port, &serial.Mode{BaudRate: 115200})
	if err != nil {
		return "", err
	}
	defer p.Close()
	_ = p.SetReadTimeout(100 * time.Millisecond)
	if _, err := p.Write([]byte(seq)); err != nil {
		return "", err
	}
	return collectPreloaderReply(p, timeout)
}

// collectPreloaderReply tolerates serial drivers that return a timeout error
// alongside a partial read. The reply is eight bytes, and CDC-ACM may split it
// across reads; returning on the first timeout can hand the caller an incomplete
// reply for its expected-response check.
func collectPreloaderReply(r io.Reader, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	got := make([]byte, 0, 8)
	buf := make([]byte, 16)
	for len(got) < 8 && time.Now().Before(deadline) {
		n, err := r.Read(buf)
		if n > 0 {
			need := 8 - len(got)
			if n > need {
				n = need
			}
			got = append(got, buf[:n]...)
		}
		if len(got) >= 8 {
			break
		}
		if err != nil && len(got) == 0 {
			if errors.Is(err, io.EOF) {
				return "", errors.New("no answer from preloader")
			}
			return "", err
		}
		if n == 0 {
			time.Sleep(time.Millisecond)
		}
	}
	if len(got) == 0 {
		return "", errors.New("no answer from preloader")
	}
	return string(got), nil
}

// enterFastbootViaPreloader polls for the preloader port until timeout and sends FASTBOOT.
func (f *Flow) enterFastbootViaPreloader(timeout time.Duration) error {
	want := ExpectedReply(BootSeq)
	deadline := time.Now().Add(timeout)
	seen := map[string]bool{}
	lastDot := time.Now()
	for time.Now().Before(deadline) {
		ports := f.Pre.Ports()
		for _, port := range ports {
			if !seen[port] {
				f.UI.Log("preloader port seen: %s", port)
				seen[port] = true
			}
			reply, err := f.Pre.Handshake(port, BootSeq, 1500*time.Millisecond)
			if err != nil {
				f.UI.Log("handshake on %s: %v", port, err)
				if strings.Contains(strings.ToLower(err.Error()), "permission") {
					f.permissionHint = true
				}
				continue
			}
			if reply == want {
				f.UI.Say("    Phone answered %q on %s - it is starting FASTBOOT mode.", reply, port)
				return nil
			}
			f.UI.Log("unexpected preloader reply %q on %s", reply, port)
		}
		if time.Since(lastDot) > 5*time.Second {
			f.UI.Raw(".")
			lastDot = time.Now()
		}
		f.sleep(50 * time.Millisecond)
	}
	return errPreloaderTimeout
}

var errPreloaderTimeout = errors.New("the phone's preloader port was not caught in time")
