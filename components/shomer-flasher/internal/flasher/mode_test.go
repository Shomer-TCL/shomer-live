package flasher

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type modeRunner struct {
	phone     *fakePhone
	overrides map[string]string
}

func (r modeRunner) Run(ctx context.Context, tool string, args []string, stream io.Writer) (string, error) {
	if out, ok := r.overrides[tool+" "+strings.Join(args, " ")]; ok {
		r.phone.calls = append(r.phone.calls, tool+" "+strings.Join(args, " "))
		if strings.Contains(out, "FAILED") {
			return out, errors.New("exit status 1")
		}
		return out, nil
	}
	return r.phone.Run(ctx, tool, args, stream)
}

func TestFastbootModeDoesNotInferFromUnsupportedOrFailedQuery(t *testing.T) {
	for _, tc := range []struct {
		out  string
		err  error
		want ConnectionMode
	}{
		{"is-userspace: yes\n", nil, ModeFastbootd},
		{"(bootloader) is-userspace: no\n", nil, ModeBootloader},
		{"is-userspace: true\n", nil, ModeUnknown},
		{"FAILED (remote: unknown variable)", nil, ModeUnknown},
		{"is-userspace: yes\n", errors.New("transport lost"), ModeUnknown},
		{"", nil, ModeUnknown},
	} {
		if got := parseFastbootMode(tc.out, tc.err); got != tc.want {
			t.Errorf("%q: got %s, want %s", tc.out, got, tc.want)
		}
	}
}

func TestUnknownModeNeverWritesSuper(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "fastboot", unlocked: true, product: "gflip6"}
	f, _ := newFlow(t, tp, ph, "")
	f.Run = modeRunner{ph, map[string]string{"fastboot getvar is-userspace": "FAILED (remote: unknown variable)"}}
	if err := f.flashPartition("super", tp.path, false); err == nil {
		t.Fatal("unknown mode permitted write")
	}
	if len(flashCalls(ph)) != 0 {
		t.Fatalf("wrote despite unknown mode: %v", ph.calls)
	}
}

func TestWrongModeAfterAcknowledgedRebootNeverWrites(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "fastboot", unlocked: true, product: "gflip6"}
	f, _ := newFlow(t, tp, ph, "")
	f.FastbootWait = time.Millisecond
	f.Run = modeRunner{ph, map[string]string{"fastboot reboot fastboot": "OKAY"}}
	if err := f.flashPartition("super", tp.path, false); err == nil {
		t.Fatal("unverified transition permitted write")
	}
	if len(flashCalls(ph)) != 0 {
		t.Fatal(ph.calls)
	}
}

func TestRecoveryPreflightFailureDoesNotClaimRecoveryInstalled(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	f, _ := newFlow(t, tp, ph, "")
	f.Run = modeRunner{ph, map[string]string{"fastboot getvar partition-size:recovery": "FAILED (remote: unknown variable)"}}
	if err := f.flashRecovery("dump"); err == nil {
		t.Fatal("missing partition evidence permitted recovery write")
	}
	if f.dumpRecoveryOn || f.recoveryWritten || len(flashCalls(ph)) != 0 {
		t.Fatalf("preflight changed recovery state: %v", ph.calls)
	}
}

func TestRecoveryWriteChangesFromFastbootdAndChecksPartition(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastbootd", true)
	f, _ := newFlow(t, tp, ph, "")
	if err := f.flashRecovery("dump"); err != nil {
		t.Fatal(err)
	}
	if ph.mode != "fastboot" || ph.flashed["recovery"] != DumpRecoverySHA256 {
		t.Fatal(ph.calls)
	}
	all := strings.Join(ph.calls, "\n")
	if !strings.Contains(all, "reboot bootloader") || !strings.Contains(all, "getvar partition-size:recovery") {
		t.Fatal(all)
	}
}

func TestRecoverySizeMismatchStopsBeforeWrite(t *testing.T) {
	tp := makePayload(t, false)
	ph := backupPhone("fastboot", true)
	f, _ := newFlow(t, tp, ph, "")
	f.Run = modeRunner{ph, map[string]string{"fastboot getvar partition-size:recovery": "partition-size:recovery: 0x1\n"}}
	if err := f.flashRecovery("stock"); err == nil {
		t.Fatal("size mismatch allowed")
	}
	if len(flashCalls(ph)) != 0 {
		t.Fatal(ph.calls)
	}
}

func TestRebootFailureIsNotReportedAsSuccessfulRun(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "fastboot", unlocked: true, product: "gflip6"}
	f, _ := newFlow(t, tp, ph, "KEKA\nn\nFLASH\n")
	f.Run = modeRunner{ph, map[string]string{"fastboot reboot": "FAILED (remote: unknown command)"}}
	if err := f.Execute(); err == nil || !strings.Contains(err.Error(), "reboot request failed") {
		t.Fatalf("got %v", err)
	}
}

func TestEraseFailureIsNotReportedAsSuccessfulRun(t *testing.T) {
	tp := makePayload(t, false)
	ph := &fakePhone{mode: "fastboot", unlocked: true, product: "gflip6"}
	f, _ := newFlow(t, tp, ph, "KEKA\ny\nFLASH\n")
	f.Run = modeRunner{ph, map[string]string{"fastboot erase userdata": "FAILED (remote: unknown command)"}}
	if err := f.Execute(); err == nil || !strings.Contains(err.Error(), "userdata erase failed") {
		t.Fatalf("got %v", err)
	}
}
