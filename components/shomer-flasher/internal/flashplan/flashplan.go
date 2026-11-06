// Package flashplan reads the Shomer flash plan (flash-plan.json, published with every image build in
// Shomer-TCL/shomer-images builds/<id>/, template in shomer-rom variants/v3-strip/flash-plan.json) and checks that
// what this flasher is about to bundle writes exactly what the plan says: the same partitions in the same order,
// the same fastboot flags on the vbmeta partitions, the same stock vbmeta files and the same super image.
// The plan is the one source of truth (Nov 2 2026); mkbundle refuses to build a flasher that disagrees with it.
package flashplan

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Step is one partition write.
type Step struct {
	Partition string `json:"partition"`
	Image     string `json:"image"`
	SHA256    string `json:"sha256"`
	Fastboot  string `json:"fastboot"`
}

// RecoveryWrite is one of the two backup-first writes to the recovery partition.
type RecoveryWrite struct {
	Step      int    `json:"step"`
	Partition string `json:"partition"`
	Role      string `json:"role"` // "dump" (recovery2, reads the phone) or "restore_stock" (genuine stock KEKA recovery back)
	Image     string `json:"image"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Fastboot  string `json:"fastboot"`
}

// Backup is the backup_first section (Nov 3 2026, flasher 1.4.0): before the normal write_order, the flasher
// writes recovery2 to recovery, reads every partition into a zip on the PC, then writes the genuine stock
// recovery back. These two pinned writes are the ONLY writes to recovery; write_order never contains it.
type Backup struct {
	Default        string          `json:"default"`
	RecoveryWrites []RecoveryWrite `json:"recovery_writes"`
}

// Plan is flash-plan.json.
type Plan struct {
	Schema      int      `json:"schema"`
	Unlocked    bool     `json:"requires_unlocked_bootloader"`
	WriteOrder  []Step   `json:"write_order"`
	VBMetaFlags []string `json:"vbmeta_flags"`
	Never       []string `json:"never_written"`
	BackupFirst *Backup  `json:"backup_first"`
}

// Load reads and parses a plan file.
func Load(path string) (*Plan, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Plan
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("flash plan %s: %v", path, err)
	}
	return &p, nil
}

// Flags is what this flasher passes to fastboot when it writes a vbmeta partition.
var Flags = []string{"--disable-verity", "--disable-verification"}

// Check compares the plan with what is being bundled.
//
//	vbmeta: the vbmeta partitions the flasher writes, in order (then super)
//	vbmetaSHA: sha256 of each bundled vbmeta file
//	superRawSHA: sha256 of the decompressed super partition being bundled
//	allowed: the flasher's partition allow-list
func (p *Plan) Check(vbmeta []string, vbmetaSHA map[string]string, superRawSHA string, allowed map[string]bool) error {
	if p.Schema != 1 {
		return fmt.Errorf("flash plan schema %d, this flasher knows 1", p.Schema)
	}
	if !p.Unlocked {
		return fmt.Errorf("flash plan does not say the bootloader must be unlocked")
	}
	want := append(append([]string{}, vbmeta...), "super")
	var got []string
	for _, s := range p.WriteOrder {
		got = append(got, s.Partition)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		return fmt.Errorf("flash plan writes %v, this flasher writes %v", got, want)
	}
	if strings.Join(p.VBMetaFlags, " ") != strings.Join(Flags, " ") {
		return fmt.Errorf("flash plan vbmeta flags %v, this flasher uses %v", p.VBMetaFlags, Flags)
	}
	for _, s := range p.WriteOrder {
		if !allowed[s.Partition] {
			return fmt.Errorf("flash plan writes %q, which this flasher refuses to write", s.Partition)
		}
		sha := strings.ToLower(s.SHA256)
		if s.Partition == "super" {
			if sha != strings.ToLower(superRawSHA) {
				return fmt.Errorf("flash plan super sha256 %s, bundled super %s", s.SHA256, superRawSHA)
			}
			continue
		}
		if !strings.Contains(s.Fastboot, strings.Join(Flags, " ")) {
			return fmt.Errorf("flash plan step %s does not carry %v", s.Partition, Flags)
		}
		if sha == "" || sha != strings.ToLower(vbmetaSHA[s.Partition]) {
			return fmt.Errorf("flash plan %s sha256 %s, bundled file %s", s.Partition, s.SHA256, vbmetaSHA[s.Partition])
		}
	}
	// every partition the flasher may write is in the plan (no silent extra writes)
	var extra []string
	for a := range allowed {
		found := false
		for _, s := range p.WriteOrder {
			if s.Partition == a {
				found = true
			}
		}
		if !found {
			extra = append(extra, a)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		return fmt.Errorf("this flasher may write %v, which the flash plan does not list", extra)
	}
	for _, n := range p.Never {
		name := strings.Fields(n)[0]
		if allowed[name] {
			return fmt.Errorf("flash plan says %q is never written, but this flasher allows it", name)
		}
	}
	return nil
}

// CheckBackup requires the backup_first section and checks it against the flasher's two pinned recovery images:
// exactly two writes, both to "recovery", recovery2 (dump) first and the genuine stock recovery (restore_stock)
// second, with these sha256s and sizes. recovery must also stay listed under never_written (no general write)
// and must not appear in write_order. Since flasher 1.4.0 a missing or different section stops the build.
func (p *Plan) CheckBackup(recoveryPartition, dumpSHA, stockSHA string, size int64) error {
	b := p.BackupFirst
	if b == nil {
		return fmt.Errorf("flash plan has no backup_first section, but this flasher writes %s twice (recovery2 %s..., then stock %s...); add it to the image build's flash-plan.json", recoveryPartition, dumpSHA[:12], stockSHA[:12])
	}
	if len(b.RecoveryWrites) != 2 {
		return fmt.Errorf("flash plan backup_first lists %d recovery writes, this flasher makes exactly 2", len(b.RecoveryWrites))
	}
	want := []struct{ role, sha string }{{"dump", dumpSHA}, {"restore_stock", stockSHA}}
	for i, w := range b.RecoveryWrites {
		if w.Step != i+1 {
			return fmt.Errorf("flash plan backup_first write %d has step %d", i+1, w.Step)
		}
		if w.Partition != recoveryPartition {
			return fmt.Errorf("flash plan backup_first write %d goes to %q, this flasher writes only %q there", i+1, w.Partition, recoveryPartition)
		}
		if w.Role != want[i].role {
			return fmt.Errorf("flash plan backup_first write %d is %q, this flasher's write %d is %q", i+1, w.Role, i+1, want[i].role)
		}
		if strings.ToLower(w.SHA256) != strings.ToLower(want[i].sha) {
			return fmt.Errorf("flash plan backup_first %s sha256 %s, this flasher pins %s", w.Role, w.SHA256, want[i].sha)
		}
		if w.Size != size {
			return fmt.Errorf("flash plan backup_first %s size %d, this flasher pins %d", w.Role, w.Size, size)
		}
	}
	for _, s := range p.WriteOrder {
		if s.Partition == recoveryPartition {
			return fmt.Errorf("flash plan write_order contains %q; it may only be written by backup_first", recoveryPartition)
		}
	}
	listed := false
	for _, n := range p.Never {
		if f := strings.Fields(n); len(f) > 0 && f[0] == recoveryPartition {
			listed = true
		}
	}
	if !listed {
		return fmt.Errorf("flash plan never_written must keep %q (only the two pinned backup_first writes touch it)", recoveryPartition)
	}
	return nil
}
