package flasher

import (
	"path/filepath"
	"strings"
)

// BackupNeed is the free space a backup needs (first, cautious estimate).
func BackupNeed(userdata bool) int64 { return backupNeed(userdata) }

// FreeBytes is the free space of the disk that holds dir.
func FreeBytes(dir string) (int64, error) { return freeBytesFn(dir) }

// WorkNeed is the free space the unpacked ROM needs.
func (f *Flow) WorkNeed() int64 { return f.Payload.Manifest.SuperRaw.Size + 200<<20 }

// Phone is what Detect found.
func (f *Flow) Phone() PhoneInfo { return f.phone }

// SameDisk: a and b (existing folders) are on the same disk. False if unknown.
func SameDisk(a, b string) bool {
	x, y := diskIDFn(a), diskIDFn(b)
	return x != "" && x == y
}

// fsIsFATFn and diskIDFn are fsIsFAT and diskID; tests replace them.
var (
	fsIsFATFn = fsIsFAT
	diskIDFn  = diskID
)

// CleanPath tidies a folder the person typed or pasted: spaces around it and
// one pair of quotes around it are removed (Windows Explorer's "Copy as path"
// gives "C:\My Backups" WITH the quotes; Windows does not allow " in a
// folder name), and it is made a full path.
func CleanPath(s string) string {
	s = strings.TrimSpace(s)
	for _, q := range [][2]string{{`"`, `"`}, {"'", "'"}, {"“", "”"}, {"”", "”"}, {"‘", "’"}} {
		if len(s) >= len(q[0])+len(q[1]) && strings.HasPrefix(s, q[0]) && strings.HasSuffix(s, q[1]) {
			s = strings.TrimSpace(s[len(q[0]) : len(s)-len(q[1])])
			break
		}
	}
	s = strings.Trim(s, `"`) // an unmatched quote (pasted half a path) is never part of a Windows folder name
	if s == "" {
		return ""
	}
	if winAbs(s) || filepath.IsAbs(s) || strings.HasPrefix(s, "/") || strings.HasPrefix(s, `\`) {
		return s // already a full (or rooted) path, kept exactly as given
	}
	if abs, err := filepath.Abs(s); err == nil {
		return abs
	}
	return s
}

func winAbs(s string) bool {
	return len(s) >= 3 && (s[0]|0x20 >= 'a' && s[0]|0x20 <= 'z') && s[1] == ':' && (s[2] == '\\' || s[2] == '/') || strings.HasPrefix(s, `\\`)
}
