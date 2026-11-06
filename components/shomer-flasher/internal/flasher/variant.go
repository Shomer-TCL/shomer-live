package flasher

// Stronger variant check (1.4.3).
//
// `fastboot getvar product` says "gflip6" for the whole TCL Flip 2 family, so
// when the phone is already in fastboot (or USB debugging is off) the only
// variant check used to be what the person typed. Two more checks now
// (each one warns and asks when it does not match; see overrideVariant):
//
//  1. checkFastbootFacts: the product must be exactly an accepted name (not
//     just start with it: "gflip65..." is another phone), and nothing in
//     `fastboot getvar all` may name a variant this ROM is not for.
//  2. While the backup copies super, the phone's own build fingerprints and
//     versions are read out of it (propScanner). Before anything else is
//     written, verifyVariant warns about a phone whose own software is
//     another variant, or when the variant was only typed and the phone's
//     files could not confirm it, and asks (default No).

import (
	"bytes"
	"sort"
	"strings"
)

// foreignVariants are names that only appear on TCL Flip 2 family phones this
// ROM is NOT for (codenames and stock build codes from the README's research;
// KEKA/UPCI phones never carry them). Matched case-insensitively as words.
var foreignVariants = []string{
	"Gflip6_TMO", "Gflip6_USCC", "Gflip65", "4058W", "4058L", "4058E", "4058R", "4058C", "4058P",
	"UPBO", "UPE6", "UPEJ", "LF91", "LF98", "PJ3R", "PJ3O", "KEE7", "KEEZ", "KEFH", "KEFS",
}

// findForeign returns the first foreign variant name in s, as a whole word.
func findForeign(s string) string {
	words := strings.FieldsFunc(strings.ToUpper(s), func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_')
	})
	for _, w := range words {
		for _, fv := range foreignVariants {
			u := strings.ToUpper(fv)
			if w == u || strings.HasPrefix(w, u+"_") || strings.HasSuffix(w, "_"+u) {
				return fv
			}
		}
	}
	return ""
}

// productAccepted: prod is exactly one of the accepted products (case does
// not matter), or the accepted name followed by "_" + a suffix the manifest
// lists itself. "gflip65" is NOT "gflip6".
func productAccepted(prod string, accepted []string) bool {
	p := strings.ToLower(strings.TrimSpace(prod))
	for _, a := range accepted {
		if a != "" && p == strings.ToLower(a) {
			return true
		}
	}
	return false
}

// getvarFacts reads `(bootloader) key: value` lines of `fastboot getvar all`.
func getvarFacts(out string) map[string]string {
	m := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "(bootloader)"))
		i := strings.Index(l, ":")
		if i <= 0 {
			continue
		}
		k, v := strings.TrimSpace(l[:i]), strings.TrimSpace(l[i+1:])
		if k == "" || v == "" || strings.ContainsAny(k, " \t") && !strings.HasPrefix(k, "partition") {
			continue
		}
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}
	return m
}

// propKeys are the build.prop keys read out of super.
var propKeys = [][]byte{[]byte("build.fingerprint="), []byte("build.version.incremental="), []byte("build.display.id=")}

// propScanner looks for "ro.*build.fingerprint=..." style lines in a byte
// stream (the phone's super while it is copied). It keeps up to 64 distinct
// "key=value" strings. Lines that straddle two writes are found too.
type propScanner struct {
	tail  []byte
	found map[string]bool
}

const propTail = 512

func (s *propScanner) Write(b []byte) (int, error) {
	if s.found == nil {
		s.found = map[string]bool{}
	}
	buf := b
	if len(s.tail) > 0 {
		buf = append(append([]byte{}, s.tail...), b...)
	}
	for _, k := range propKeys {
		off := 0
		for len(s.found) < 64 {
			i := bytes.Index(buf[off:], k)
			if i < 0 {
				break
			}
			i += off
			off = i + len(k)
			// key start: back to "ro." on the same line
			st := bytes.LastIndex(buf[max(0, i-64):i], []byte("ro."))
			if st < 0 {
				continue
			}
			st += max(0, i-64)
			end := off
			for end < len(buf) && end-off < 200 && buf[end] >= 0x20 && buf[end] < 0x7f {
				end++
			}
			if end == len(buf) && end-off < 200 {
				continue // the value goes on in the next write: found then (it is in the tail)
			}
			kv := string(buf[st:end])
			if !strings.ContainsAny(kv[:strings.Index(kv, "=")], " \t") && end > off {
				s.found[kv] = true
			}
		}
	}
	if len(buf) > propTail {
		s.tail = append(s.tail[:0], buf[len(buf)-propTail:]...)
	} else {
		s.tail = append(s.tail[:0], buf...)
	}
	return len(b), nil
}

// Found returns the distinct key=value strings, sorted.
func (s *propScanner) Found() []string {
	var l []string
	for k := range s.found {
		l = append(l, k)
	}
	sort.Strings(l)
	return l
}

// variantVerdict decides from the props read out of the phone's super.
// confirmed: one of them names an accepted build. foreign: one names another
// variant (and none an accepted build).
func variantVerdict(props, builds []string) (confirmed, foreign string) {
	for _, p := range props {
		v := p[strings.Index(p, "=")+1:]
		if containsAnyFold(v, builds) {
			if b, ok := ParseBuild(v, builds); ok {
				return b, ""
			}
			for _, b := range builds {
				if b != "" && strings.Contains(strings.ToUpper(v), strings.ToUpper(b)) {
					return b, ""
				}
			}
		}
	}
	for _, p := range props {
		if fv := findForeign(p[strings.Index(p, "=")+1:]); fv != "" {
			return "", p
		}
	}
	return "", ""
}

// verifyVariant runs after the backup step and before anything else is
// written. It uses what propScanner found in the phone's own super.
func (f *Flow) verifyVariant() error {
	builds := f.Payload.Manifest.Builds
	if len(f.phoneProps) == 0 {
		if f.buildSource == "typed" {
			f.UI.Say("    Note: the software version (%s) is what you typed; the phone itself could not be asked (no backup of its software was read).", f.buildOK)
		}
		return nil
	}
	confirmed, foreign := variantVerdict(f.phoneProps, builds)
	const stopped = "Only the backup ran (the stock recovery is back); Shomer was not written"
	switch {
	case confirmed != "":
		f.UI.Say("    OK - the phone's own software files say %s.", confirmed)
		if f.buildSource == "typed" && !strings.EqualFold(confirmed, f.buildOK) {
			f.UI.Say("    (You typed %s; the phone's files say %s. Both are supported.)", f.buildOK, confirmed)
		}
		return nil
	case foreign != "":
		return f.overrideVariant("the phone's own software (read during the backup) says "+foreign+", which is not T408DL KEKA / 4058G UPCI.",
			nil, stopped)
	}
	if f.buildSource == "adb" {
		return nil // read from the running phone itself; its files just did not name a build
	}
	detail := []string{"The phone's own software files say:"}
	for _, p := range firstN(f.phoneProps, 4) {
		detail = append(detail, "  "+p)
	}
	return f.overrideVariant("could not confirm the phone's software version from its own files (you chose "+f.buildOK+").", detail, stopped)
}

// ---------- never block (Sam, Nov 4) ----------
//
// "I don't want it to block (let's say I can't access ADB or whatever I
// should always have a manual option)". Every model / variant / version check
// therefore ends the same way when it does not match or cannot tell:
//  1. a clear WARNING: what was detected (or that nothing could be detected)
//     and what this ROM is for;
//  2. the person's own choice, KEKA or UPCI (typed, or --variant, or what they
//     already typed earlier);
//  3. an explicit question "Flash anyway as <choice>?" with default No. Enter
//     on its own, No, a closed console, Cancel or closing the box all stop.
//     --force-variant answers yes (the warning is still shown and logged).
// No other safety check goes through here: checksums, an incomplete backup,
// two phones, disk space, USB permission, a lost connection and a locked
// bootloader stop exactly as before.

// overrideVariant shows the warning, gets the person's KEKA/UPCI choice and
// asks. It returns nil to continue, or the reason to stop (stopped is said
// last: what was and was not changed).
func (f *Flow) overrideVariant(what string, detail []string, stopped string) error {
	builds := f.Payload.Manifest.Builds
	lines := []string{"WARNING - THE PHONE COULD NOT BE CONFIRMED AS A SUPPORTED ONE.", what}
	lines = append(lines, detail...)
	lines = append(lines,
		"This ROM is only for the TCL Flip 2 T408DL with software KEKA and the TCL 4058G with",
		"software UPCI (the same phone). On any other phone or version it can lose its cell service.",
		"You can still go on with your own choice ("+strings.Join(builds, " or ")+") if you are sure.")
	f.UI.Warn(lines...)
	f.UI.Log("variant warning: %s", what)
	choice, err := f.manualChoice(stopped)
	if err != nil {
		return err
	}
	if f.Opts.ForceVariant {
		f.UI.Say("    --force-variant: going on as %s (your choice) without asking.", choice)
		f.variantWarned = append(f.variantWarned, what)
		return nil
	}
	yes, err := f.UI.YesNo("    Flash anyway as "+choice+"? (your choice; Enter = No, stop)", false)
	if err != nil {
		return err
	}
	if !yes {
		return refuse("stopped by you after the warning: %s %s", what, stopped)
	}
	f.UI.Say("    OK - going on as %s because you said yes.", choice)
	f.UI.Log("variant warning overridden by the person as %s", choice)
	f.variantWarned = append(f.variantWarned, what)
	return nil
}

// manualChoice is the person's own KEKA/UPCI choice: what they already typed,
// --variant, or a new question (one more try if the answer is not a version).
func (f *Flow) manualChoice(stopped string) (string, error) {
	builds := f.Payload.Manifest.Builds
	if (f.buildSource == "typed" || f.buildSource == "skipped") && f.buildOK != "" {
		return f.buildOK, nil
	}
	if ok, err := f.variantFromFlag(); err != nil {
		return "", err
	} else if ok {
		return f.buildOK, nil
	}
	for i := 0; i < 2; i++ {
		ans, err := f.UI.Ask("    To go on anyway, type the version to flash it as (" + strings.Join(builds, " or ") + "), or Q to stop:")
		if err != nil {
			return "", err
		}
		if isQuit(ans) {
			return "", refuse("stopped by you. %s", stopped)
		}
		if b, ok := ParseBuild(ans, builds); ok {
			f.buildOK, f.buildSource = b, "typed"
			return b, nil
		}
		f.UI.Say("    %q is not one of: %s.", strings.TrimSpace(ans), strings.Join(builds, ", "))
	}
	return "", refuse("stopped: no version (%s) was chosen. %s", strings.Join(builds, " or "), stopped)
}

// variantFromFlag uses --variant when it was given. A value that is not a
// supported version is an error (the accepted list is shown).
func (f *Flow) variantFromFlag() (bool, error) {
	if strings.TrimSpace(f.Opts.Variant) == "" {
		return false, nil
	}
	builds := f.Payload.Manifest.Builds
	b, ok := ParseBuild(f.Opts.Variant, builds)
	if !ok {
		return false, refuse("--variant %q is not a version this ROM is for. Accepted: %s. Nothing was changed on the phone", f.Opts.Variant, strings.Join(builds, ", "))
	}
	f.buildOK, f.buildSource = b, "typed"
	f.UI.Say("    Software version from --variant: %s.", b)
	return true, nil
}

func firstN(l []string, n int) []string {
	if len(l) > n {
		return l[:n]
	}
	return l
}
