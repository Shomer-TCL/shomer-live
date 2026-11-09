package flasher

import (
	"errors"
	"testing"
)

func TestNonpersistentBackupIsRefusedBeforeUnlockOrAnyWrite(t *testing.T) {
	tp := makePayload(t, false)
	phone := backupPhone("fastboot", false)
	f, _ := newFlow(t, tp, phone, "UNLOCK\nFLASH\n")
	f.Opts.Backup = true
	f.Opts.BackupDir = t.TempDir()
	old := backupDestinationCheckFn
	defer func() { backupDestinationCheckFn = old }()
	denied := errors.New("temporary live RAM destination")
	backupDestinationCheckFn = func(string) error { return denied }
	if err := f.WritePhone(); !errors.Is(err, denied) {
		t.Fatalf("got %v, want destination refusal", err)
	}
	if phone.unlocked {
		t.Fatal("phone was unlocked before destination validation")
	}
	if len(phone.calls) != 0 {
		t.Fatalf("phone commands before destination validation: %v", phone.calls)
	}
}
