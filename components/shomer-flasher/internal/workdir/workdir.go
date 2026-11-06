// Package workdir owns the flasher's work folder and the lock that keeps two
// copies of the flasher from running at the same time.
//
// The work folder is always the flasher's OWN subfolder (SubName) of the
// folder the person chose with --workdir (or of the program's folder), marked
// with a Marker file. Cleanup removes only the files the flasher itself puts
// there, one by one, and folders only when they are empty: nothing the person
// put anywhere is ever deleted (1.4.2 deleted <workdir>/tools with everything
// in it).
package workdir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	SubName  = "shomer-flasher-work"  // the flasher's own folder inside the chosen one
	Marker   = ".shomer-flasher-work" // "this folder was made by the Shomer flasher"
	LockName = ".shomer-flasher.lock"
	ToolsDir = "tools"
)

// ownFiles are the only names the flasher writes into its work folder.
var ownFiles = []string{"super.raw", "vbmeta.img", "vbmeta_system.img", "vbmeta_vendor.img",
	"recovery-dump.img", "recovery-stock.img", Marker, LockName}

func isOwn(name string) bool {
	if name == ToolsDir {
		return true
	}
	for _, n := range ownFiles {
		if n == name {
			return true
		}
	}
	return false
}

// Dir is a prepared work folder.
type Dir struct {
	Path        string // <base>/shomer-flasher-work
	base        string
	baseCreated bool // the base folder did not exist before: removed at the end if empty
	tools       []string
}

// ToolDir is where fastboot/adb are unpacked.
func (d *Dir) ToolDir() string { return filepath.Join(d.Path, ToolsDir) }

// AddTool records a file the flasher unpacked into ToolDir (removed at Cleanup).
func (d *Dir) AddTool(name string) { d.tools = append(d.tools, filepath.Base(name)) }

// Prepare makes (or takes over) <base>/shomer-flasher-work. A folder of that
// name that holds anything else than the flasher's own files is refused.
func Prepare(base string) (*Dir, error) {
	if base == "" {
		return nil, errors.New("no work folder")
	}
	if abs, err := filepath.Abs(base); err == nil {
		base = abs
	}
	d := &Dir{base: base, Path: filepath.Join(base, SubName)}
	if _, err := os.Stat(base); errors.Is(err, os.ErrNotExist) {
		d.baseCreated = true
	}
	st, err := os.Stat(d.Path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(d.Path, 0o755); err != nil {
			return nil, fmt.Errorf("cannot create the work folder %s: %v", d.Path, err)
		}
	case err != nil:
		return nil, err
	case !st.IsDir():
		return nil, fmt.Errorf("%s exists and is not a folder; choose another --workdir", d.Path)
	default:
		if _, err := os.Stat(filepath.Join(d.Path, Marker)); err != nil {
			// no marker: take it over only if it holds nothing but the flasher's own
			// files (a folder left by 1.4.2 or older, e.g. after --keep)
			if foreign := foreignEntries(d.Path); len(foreign) > 0 {
				return nil, fmt.Errorf("the folder %s already exists and holds files that are not the flasher's (%s). Nothing was changed. Choose another --workdir, or move those files", d.Path, strings.Join(foreign, ", "))
			}
		}
	}
	if err := os.WriteFile(filepath.Join(d.Path, Marker), []byte("Made by the Shomer flasher. Only its own files are kept here; it removes them when it is done.\n"), 0o644); err != nil {
		return nil, fmt.Errorf("cannot write in the work folder %s: %v", d.Path, err)
	}
	return d, nil
}

func foreignEntries(dir string) []string {
	var f []string
	es, _ := os.ReadDir(dir)
	for _, e := range es {
		if !isOwn(e.Name()) {
			f = append(f, e.Name())
		}
	}
	if es, err := os.ReadDir(filepath.Join(dir, ToolsDir)); err == nil {
		for _, e := range es {
			if !isToolName(e.Name()) {
				f = append(f, filepath.Join(ToolsDir, e.Name()))
			}
		}
	}
	sort.Strings(f)
	if len(f) > 5 {
		f = append(f[:5], "...")
	}
	return f
}

// isToolName: the files the flasher unpacks into tools/ (platform-tools).
func isToolName(n string) bool {
	switch strings.ToLower(n) {
	case "fastboot", "adb", "fastboot.exe", "adb.exe", "adbwinapi.dll", "adbwinusbapi.dll", "libwinpthread-1.dll":
		return true
	}
	return false
}

// Cleanup removes the flasher's own files and then each folder only if empty.
// keep: leave the unpacked ROM and tools (--keep).
func (d *Dir) Cleanup(keep bool) {
	if d == nil || keep {
		return
	}
	for _, t := range d.tools {
		if isToolName(t) {
			os.Remove(filepath.Join(d.ToolDir(), t))
		}
	}
	os.Remove(d.ToolDir()) // only if empty
	for _, n := range ownFiles {
		if n != LockName { // the lock file goes with the lock (Release)
			os.Remove(filepath.Join(d.Path, n))
		}
	}
	os.Remove(d.Path) // only if empty
	if d.baseCreated {
		os.Remove(d.base) // only if empty
	}
}

// ErrBusy: another copy of the flasher holds the lock.
type ErrBusy struct {
	Path string
	PID  int
}

func (e ErrBusy) Error() string {
	who := ""
	if e.PID > 0 {
		who = fmt.Sprintf(" (process %d)", e.PID)
	}
	return "another copy of the Shomer flasher is already running on this computer" + who +
		". Two copies at once break each other (one deletes the other's files and stops its USB tools). Close the other window first, then start again"
}

// Lock is held for the whole run; the OS frees it if the program dies.
type Lock struct {
	f    *os.File
	path string
}

// Acquire takes the lock file at path, or returns ErrBusy.
func Acquire(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("cannot open the lock file %s: %v", path, err)
	}
	if err := lockFile(f); err != nil {
		pid := 0
		if b, rerr := os.ReadFile(path); rerr == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
		f.Close()
		if errors.Is(err, errLocked) {
			return nil, ErrBusy{Path: path, PID: pid}
		}
		return nil, fmt.Errorf("cannot lock %s: %v", path, err)
	}
	f.Truncate(0)
	f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	return &Lock{f: f, path: path}, nil
}

// Release frees the lock and removes its file.
func (l *Lock) Release() {
	if l == nil || l.f == nil {
		return
	}
	unlockFile(l.f)
	l.f.Close()
	os.Remove(l.path)
	l.f = nil
}

var errLocked = errors.New("locked by another process")
