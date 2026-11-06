package flashplan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const vb = "0f1c398d3a7699701616b19a8d572c99a574d8dfdcd0285ed6e767872d14d0f8"
const vs = "c482fe430b276d3fa26d7d08a9de8f6080baf276f1953609c9850946c886aa92"
const vv = "b77ed5c44d6046122ac5e9305ba7553499aa2eb27eecc4dfcca9ceb9f02ddb7c"
const raw = "1111111111111111111111111111111111111111111111111111111111111111"

var allowed = map[string]bool{"super": true, "vbmeta": true, "vbmeta_system": true, "vbmeta_vendor": true}
var names = []string{"vbmeta", "vbmeta_system", "vbmeta_vendor"}
var shas = map[string]string{"vbmeta": vb, "vbmeta_system": vs, "vbmeta_vendor": vv}

func plan(t *testing.T, edit func(string) string) *Plan {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "flash-plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := strings.Replace(string(b), "PER_BUILD", raw, 1)
	if edit != nil {
		s = edit(s)
	}
	p := filepath.Join(t.TempDir(), "p.json")
	os.WriteFile(p, []byte(s), 0o644)
	pl, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return pl
}

func TestTheShomerTemplateMatchesThisFlasher(t *testing.T) {
	if err := plan(t, nil).Check(names, shas, raw, allowed); err != nil {
		t.Fatal(err)
	}
}

func TestRefusesADifferentSuperImage(t *testing.T) {
	if err := plan(t, nil).Check(names, shas, strings.Repeat("2", 64), allowed); err == nil {
		t.Fatal("accepted another super")
	}
}

func TestRefusesAChangedVBMetaFile(t *testing.T) {
	bad := map[string]string{"vbmeta": vb, "vbmeta_system": vs, "vbmeta_vendor": strings.Repeat("3", 64)}
	if err := plan(t, nil).Check(names, bad, raw, allowed); err == nil {
		t.Fatal("accepted a changed vbmeta_vendor")
	}
}

func TestRefusesVBMetaOnlyPlanAgainstAThreeVBMetaFlasher(t *testing.T) {
	// the Oct 30 disagreement: a plan that writes only vbmeta must stop the build
	p := plan(t, nil)
	p.WriteOrder = append(p.WriteOrder[:1], p.WriteOrder[3:]...)
	if err := p.Check(names, shas, raw, allowed); err == nil || !strings.Contains(err.Error(), "writes") {
		t.Fatalf("got %v", err)
	}
}

func TestRefusesOtherFlagsOrOrder(t *testing.T) {
	p := plan(t, nil)
	p.VBMetaFlags = []string{"--disable-verity"}
	if err := p.Check(names, shas, raw, allowed); err == nil {
		t.Fatal("accepted other flags")
	}
	p = plan(t, nil)
	p.WriteOrder[0], p.WriteOrder[1] = p.WriteOrder[1], p.WriteOrder[0]
	if err := p.Check(names, shas, raw, allowed); err == nil {
		t.Fatal("accepted another order")
	}
}

func TestRefusesAnAllowListWiderThanThePlan(t *testing.T) {
	wide := map[string]bool{"super": true, "vbmeta": true, "vbmeta_system": true, "vbmeta_vendor": true, "boot": true}
	if err := plan(t, nil).Check(names, shas, raw, wide); err == nil {
		t.Fatal("accepted an allow-list with boot")
	}
}

const dumpSHA = "394ad6bb38321565b121dec8c5dff098cd238919b766f9cbc0c994f4f0a376ac"
const stockSHA = "4fa8dacf08c9fe2131616d377b6c4da5b44bb504df1c67274c37aa7c38b551d3"
const recSize = 25165824

func TestBackupFirstSectionMatchesThePinnedRecoveries(t *testing.T) {
	if err := plan(t, nil).CheckBackup("recovery", dumpSHA, stockSHA, recSize); err != nil {
		t.Fatal(err)
	}
}

func TestBackupFirstIsRequired(t *testing.T) {
	p := plan(t, nil)
	p.BackupFirst = nil
	if err := p.CheckBackup("recovery", dumpSHA, stockSHA, recSize); err == nil || !strings.Contains(err.Error(), "no backup_first") {
		t.Fatalf("plan without backup_first accepted: %v", err)
	}
}

func TestBackupFirstRefusesOtherRecoveryBytes(t *testing.T) {
	for _, sha := range []string{dumpSHA, stockSHA} {
		p := plan(t, func(s string) string { return strings.Replace(s, sha, strings.Repeat("9", 64), 1) })
		if err := p.CheckBackup("recovery", dumpSHA, stockSHA, recSize); err == nil {
			t.Fatalf("accepted a plan with %s... replaced", sha[:8])
		}
	}
}

func TestBackupFirstRefusesSwappedOrderWrongPartitionOrExtraWrite(t *testing.T) {
	p := plan(t, nil)
	w := p.BackupFirst.RecoveryWrites
	w[0], w[1] = w[1], w[0]
	if err := p.CheckBackup("recovery", dumpSHA, stockSHA, recSize); err == nil {
		t.Fatal("accepted stock-then-dump order")
	}
	p = plan(t, nil)
	p.BackupFirst.RecoveryWrites[1].Partition = "boot"
	if err := p.CheckBackup("recovery", dumpSHA, stockSHA, recSize); err == nil {
		t.Fatal("accepted a restore to boot")
	}
	p = plan(t, nil)
	p.BackupFirst.RecoveryWrites = append(p.BackupFirst.RecoveryWrites, p.BackupFirst.RecoveryWrites[1])
	if err := p.CheckBackup("recovery", dumpSHA, stockSHA, recSize); err == nil {
		t.Fatal("accepted a third recovery write")
	}
	p = plan(t, nil)
	p.BackupFirst.RecoveryWrites[0].Size = 1
	if err := p.CheckBackup("recovery", dumpSHA, stockSHA, recSize); err == nil {
		t.Fatal("accepted a wrong size")
	}
}

func TestBackupFirstRecoveryNeverInWriteOrderAndStaysNeverWritten(t *testing.T) {
	p := plan(t, nil)
	p.WriteOrder = append(p.WriteOrder, Step{Partition: "recovery"})
	if err := p.CheckBackup("recovery", dumpSHA, stockSHA, recSize); err == nil {
		t.Fatal("accepted recovery in write_order")
	}
	p = plan(t, nil)
	var keep []string
	for _, n := range p.Never {
		if !strings.HasPrefix(n, "recovery") {
			keep = append(keep, n)
		}
	}
	p.Never = keep
	if err := p.CheckBackup("recovery", dumpSHA, stockSHA, recSize); err == nil {
		t.Fatal("accepted a plan that dropped recovery from never_written")
	}
	// the general check still refuses a flasher whose general allow-list contains recovery
	a := map[string]bool{"super": true, "vbmeta": true, "vbmeta_system": true, "vbmeta_vendor": true, "recovery": true}
	if err := plan(t, nil).Check(names, shas, raw, a); err == nil {
		t.Fatal("general allow-list with recovery accepted")
	}
}
