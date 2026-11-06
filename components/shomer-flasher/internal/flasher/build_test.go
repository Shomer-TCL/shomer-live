package flasher

import (
	"errors"
	"strings"
	"testing"
)

func TestParseBuild(t *testing.T) {
	builds := []string{"KEKA", "UPCI"}
	ok := map[string]string{
		"UPCI": "UPCI", "upci": "UPCI", " UPCI ": "UPCI", "\tUpCi\r\n": "UPCI", "\"UPCI\"": "UPCI", "\u201cupci\u201d": "UPCI",
		"keka": "KEKA", "KEKA.": "KEKA", "4058G_UPCI_05": "UPCI", "T408DL-KEKA-1.0": "KEKA", "Build number: KEKA 2.0": "KEKA",
		"ופבן": "UPCI", "לקלש": "KEKA", " ופבן ": "UPCI", // typed with the Hebrew keyboard layout still on
	}
	for in, want := range ok {
		got, good := ParseBuild(in, builds)
		if !good || got != want {
			t.Errorf("ParseBuild(%q) = %q, %v; want %q", in, got, good, want)
		}
	}
	for _, in := range []string{"", "   ", "\"\"", "UPC", "UPCIX", "KEE7", "UPBO", "KEKA UPCI", "xx", "q"} {
		if got, good := ParseBuild(in, builds); good {
			t.Errorf("ParseBuild(%q) = %q, want refused", in, got)
		}
	}
}

// Sam, Nov 4 2026: no phone connected, typed UPCI by hand. Small letters and
// spaces must work too, and the flow must go on to catching the phone.
func TestTypedBuildCaseInsensitiveWithSpaces(t *testing.T) {
	for _, typed := range []string{"UPCI", " upci ", "Upci", "ופבן"} {
		tp := makePayload(t, false)
		ph := &fakePhone{mode: "off", product: "gflip6", unlocked: true}
		f, out := newFlow(t, tp, ph, typed+"\n\n")
		if err := f.Prepare(); err != nil {
			t.Fatal(err)
		}
		p, err := f.Detect()
		if err != nil {
			t.Fatalf("%q: %v\n%s", typed, err, out)
		}
		if p.Build != "UPCI" {
			t.Fatalf("%q: build %q", typed, p.Build)
		}
		if !strings.Contains(out.String(), "OK - UPCI.") || !strings.Contains(out.String(), "Getting the phone into FASTBOOT mode") {
			t.Fatalf("%q: flow did not go on:\n%s", typed, out)
		}
		if len(flashCalls(ph)) != 0 {
			t.Fatal("Detect wrote to the phone")
		}
	}
}

func TestTypedBuildEmptyShowsAcceptedListAndRetries(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "off", product: "gflip6", unlocked: true}
	f, out := newFlow(t, tp, ph, "\nUPBO\n upci \n\n")
	if err := f.Prepare(); err != nil {
		t.Fatal(err)
	}
	p, err := f.Detect()
	if err != nil || p.Build != "UPCI" {
		t.Fatalf("third try must be accepted: %v %q\n%s", err, p.Build, out)
	}
	o := out.String()
	for _, want := range []string{"Nothing was typed. Accepted answers: KEKA, UPCI", `"UPBO" is not a supported software version. Accepted answers: KEKA, UPCI`, "2 of 3 tries left", "1 of 3 tries left"} {
		if !strings.Contains(o, want) {
			t.Errorf("missing %q in:\n%s", want, o)
		}
	}
}

func TestTypedBuildThreeWrongRefusedWithList(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "off", product: "gflip6"}
	// 1.4.3 (never block): three wrong answers warn, then ask for KEKA/UPCI once more; two more blanks stop
	f, out := newFlow(t, tp, ph, "\n\n\n\n\n")
	if err := f.Prepare(); err != nil {
		t.Fatal(err)
	}
	_, err := f.Detect()
	var r ErrRefused
	if !errors.As(err, &r) || !strings.Contains(r.Msg, "KEKA or UPCI") || !strings.Contains(out.String(), "Accepted answers: KEKA, UPCI") {
		t.Fatalf("want refusal naming the list, got %v\n%s", err, out)
	}
	if len(ph.calls) > 0 && len(flashCalls(ph)) != 0 {
		t.Fatal("wrote to phone")
	}
}

func TestTypedQuitStops(t *testing.T) {
	for _, q := range []string{"q", " Q ", "quit", "/"} {
		tp := makePayload(t, false)
		ph := &fakePhone{mode: "off", product: "gflip6"}
		f, _ := newFlow(t, tp, ph, q+"\n")
		if err := f.Prepare(); err != nil {
			t.Fatal(err)
		}
		_, err := f.Detect()
		var r ErrRefused
		if !errors.As(err, &r) || !strings.Contains(r.Msg, "stopped by you") {
			t.Fatalf("%q: want stop, got %v", q, err)
		}
	}
}
