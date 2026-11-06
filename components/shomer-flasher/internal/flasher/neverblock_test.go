package flasher

import (
	"errors"
	"strings"
	"testing"
)

// 1.4.3, Sam (Nov 4): "I don't want it to block ... I should always have a
// manual option". Every model / variant / version check warns and asks
// (default No); Yes goes on with the person's own KEKA/UPCI choice.
// For each check: warning + Yes continues, warning + No stops, closed input
// stops; and a normal KEKA phone shows no warning.

type nbCase struct {
	name   string
	phone  func() *fakePhone
	before string // answers before the warning (the typed version, if asked)
	choice string // the person's KEKA/UPCI choice after the warning ("" = already typed)
	want   string // in the warning
	after  string // answers after the yes (adb phones: Enter when the phone is off for the fastboot catch)
}

func nbCases() []nbCase {
	return []nbCase{
		{"model-over-adb", func() *fakePhone { return &fakePhone{mode: "adb", model: "4058W", build: "KEKA", product: "gflip6"} }, "", "UPCI\n", "4058W", "\n"},
		{"software-over-adb", func() *fakePhone { return &fakePhone{mode: "adb", model: "T408DL", build: "KEE7", product: "gflip6"} }, "", "KEKA\n", "KEE7", "\n"},
		{"typed-version-unknown", func() *fakePhone { return &fakePhone{mode: "fastboot", product: "gflip6"} }, "UPBO\nKEE7\nxx\n", "UPCI\n", "xx", ""},
		{"getvar-product-empty", func() *fakePhone { return &fakePhone{mode: "fastboot", product: ""} }, "KEKA\n", "", "could not detect", ""},
		{"getvar-product-other", func() *fakePhone { return &fakePhone{mode: "fastboot", product: "gflip65"} }, "KEKA\n", "", "gflip65", ""},
		{"getvar-all-other-variant", func() *fakePhone {
			return &fakePhone{mode: "fastboot", product: "gflip6", getvarAll: "(bootloader) variant: Gflip6_TMO\n"}
		}, "KEKA\n", "", "Gflip6_TMO", ""},
	}
}

func detectWith(t *testing.T, ph *fakePhone, input string, opts func(*Options)) (*Flow, error, string) {
	t.Helper()
	tp := makePayload(t, false)
	f, out := newFlow(t, tp, ph, input)
	if opts != nil {
		opts(&f.Opts)
	}
	_, err := f.Detect()
	return f, err, out.String()
}

func TestNeverBlockWarningYesContinues(t *testing.T) {
	for _, c := range nbCases() {
		t.Run(c.name, func(t *testing.T) {
			ph := c.phone()
			f, err, out := detectWith(t, ph, c.before+c.choice+"y\n"+c.after, nil)
			if err != nil {
				t.Fatalf("yes after the warning must go on, got %v\n%s", err, out)
			}
			if !strings.Contains(out, "WARNING") || !strings.Contains(out, c.want) || !strings.Contains(out, "Flash anyway as ") {
				t.Fatalf("no clear warning/question naming %q\n%s", c.want, out)
			}
			wantBuild := strings.TrimSpace(c.choice)
			if wantBuild == "" {
				wantBuild = strings.TrimSpace(c.before)
			}
			if f.buildOK != wantBuild || len(f.variantWarned) != 1 {
				t.Fatalf("build %q (want the person's choice %q), warned %v", f.buildOK, wantBuild, f.variantWarned)
			}
			if len(writeCalls(ph)) != 0 {
				t.Fatalf("Detect wrote: %v", writeCalls(ph))
			}
		})
	}
}

func TestNeverBlockWarningNoStops(t *testing.T) {
	for _, c := range nbCases() {
		for _, no := range []string{"n\n", "\n", "no\n"} { // Enter on its own is No
			ph := c.phone()
			_, err, out := detectWith(t, ph, c.before+c.choice+no, nil)
			var r ErrRefused
			if !errors.As(err, &r) || !strings.Contains(r.Msg, "stopped by you after the warning") || !strings.Contains(r.Msg, c.want) {
				t.Fatalf("%s %q: want a stop naming %q, got %v\n%s", c.name, no, c.want, err, out)
			}
			if len(writeCalls(ph)) != 0 {
				t.Fatalf("%s: wrote %v", c.name, writeCalls(ph))
			}
		}
	}
}

func TestNeverBlockClosedInputStops(t *testing.T) {
	for _, c := range nbCases() {
		// input ends at the yes/no question, and (when asked) at the choice
		inputs := []string{c.before + c.choice}
		if c.choice != "" {
			inputs = append(inputs, c.before)
		}
		for _, in := range inputs {
			ph := c.phone()
			_, err, out := detectWith(t, ph, in, nil)
			if !errors.Is(err, ErrInputClosed) {
				t.Fatalf("%s %q: closed input must stop, got %v\n%s", c.name, in, err, out)
			}
			if len(writeCalls(ph)) != 0 {
				t.Fatalf("%s: wrote %v", c.name, writeCalls(ph))
			}
		}
	}
	// Cancel in the TUI/window counts the same
	if !errors.Is(ErrCanceled, ErrInputClosed) {
		t.Fatal("Cancel is not a stop")
	}
}

func TestNeverBlockForceVariantAndFlag(t *testing.T) {
	for _, c := range nbCases() {
		ph := c.phone()
		// --variant answers the choice (and the version question), --force-variant the yes/no: no input at all
		f, err, out := detectWith(t, ph, c.after, func(o *Options) { o.Variant = " upci "; o.ForceVariant = true })
		if err != nil {
			t.Fatalf("%s: --variant UPCI --force-variant must go on, got %v\n%s", c.name, err, out)
		}
		if c.name == "typed-version-unknown" {
			// --variant answers the version question itself: nothing unknown was typed, no warning
			if strings.Contains(out, "WARNING") || f.buildOK != "UPCI" {
				t.Fatalf("%s: %q\n%s", c.name, f.buildOK, out)
			}
			continue
		}
		if !strings.Contains(out, "WARNING") || !strings.Contains(out, "--force-variant") || strings.Contains(out, "Flash anyway as") {
			t.Fatalf("%s: warning must still show, question must not\n%s", c.name, out)
		}
		if f.buildOK != "UPCI" {
			t.Fatalf("%s: build %q", c.name, f.buildOK)
		}
	}
	// --variant alone: the question is still asked
	_, err, _ := detectWith(t, &fakePhone{mode: "fastboot", product: "gflip65"}, "", func(o *Options) { o.Variant = "KEKA" })
	if !errors.Is(err, ErrInputClosed) {
		t.Fatalf("--variant alone must still ask, got %v", err)
	}
	// a wrong --variant is refused with the list
	_, err, _ = detectWith(t, &fakePhone{mode: "fastboot", product: "gflip6"}, "", func(o *Options) { o.Variant = "KEE7" })
	var r ErrRefused
	if !errors.As(err, &r) || !strings.Contains(r.Msg, "KEKA, UPCI") {
		t.Fatalf("bad --variant: %v", err)
	}
	// no adb and no phone answer: --variant is the manual option (no question at all)
	f, err, out := detectWith(t, &fakePhone{mode: "fastboot", product: "gflip6"}, "", func(o *Options) { o.Variant = "UPCI" })
	if err != nil || f.buildOK != "UPCI" || strings.Contains(out, "WARNING") {
		t.Fatalf("--variant UPCI on a normal phone: %v\n%s", err, out)
	}
}

func TestGetvarEmptyAnswerStaysEmpty(t *testing.T) {
	for _, out := range []string{"product: \nFinished. Total time: 0.002s\n", "(bootloader) product:\r\nFinished.\n", "product:\n"} {
		if v, ok := parseGetvar(out, "product"); !ok || v != "" {
			t.Fatalf("%q -> %q %v (the next line was taken as the product)", out, v, ok)
		}
	}
	if v, _ := parseGetvar("(bootloader) product: gflip6\r\nFinished.\n", "product"); v != "gflip6" {
		t.Fatalf("got %q", v)
	}
	if v, _ := parseGetvar("unlocked: yes\nFinished.", "unlocked"); v != "yes" {
		t.Fatal(v)
	}
}

func TestNeverBlockNormalPhoneNoWarning(t *testing.T) {
	for _, ph := range []*fakePhone{
		{mode: "fastboot", product: "gflip6"}, // typed
		{mode: "fastboot", product: "GFLIP6", getvarAll: "(bootloader) version-bootloader: KEKA-0001\n"},
	} {
		f, err, out := detectWith(t, ph, "KEKA\n", nil)
		if err != nil || strings.Contains(out, "WARNING") || strings.Contains(out, "Flash anyway") || len(f.variantWarned) != 0 {
			t.Fatalf("normal phone warned: %v\n%s", err, out)
		}
	}
	// a normal KEKA phone read over adb, through the whole flash (no question about the variant)
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	f, out := newFlow(t, tp, ph, "KEKA\nn\nFLASH\n")
	if err := f.Execute(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(out.String(), "WARNING - THE PHONE") || ph.flashed["super"] != tp.rawSHA {
		t.Fatalf("normal phone: warned or not written\n%s", out)
	}
}

// After a warning + yes, the flash goes all the way (whole flow, not just Detect),
// and the end of the run says the person chose to go on.
func TestNeverBlockYesFlashesWholeFlow(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	ph.product = "gflip65"
	f, out := newFlow(t, tp, ph, "UPCI\ny\nn\nFLASH\n")
	if err := f.Execute(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if ph.flashed["super"] != tp.rawSHA || !strings.Contains(out.String(), "you chose to go on as UPCI after 1 warning") {
		t.Fatalf("not flashed / no note\n%s", out)
	}
}

// The other safety stops are NOT affected: --force-variant does not get past a bad checksum.
func TestNeverBlockForceDoesNotSkipOtherChecks(t *testing.T) {
	tp := makePayload(t, true) // bad checksum
	ph := &fakePhone{mode: "fastboot", product: "gflip65", unlocked: true}
	f, out := newFlow(t, tp, ph, "")
	f.Opts.Variant, f.Opts.ForceVariant = "KEKA", true
	err := f.Execute()
	var r ErrRefused
	if err == nil || len(flashCalls(ph)) != 0 {
		t.Fatalf("bad checksum went through with --force-variant: %v\n%s", err, out)
	}
	_ = r
}
