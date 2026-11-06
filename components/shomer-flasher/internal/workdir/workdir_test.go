package workdir

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Bug 4: --workdir never deletes what the flasher did not create.
func TestCleanupKeepsThePersonsFiles(t *testing.T) {
	base := t.TempDir()
	// the person's own "tools" folder right in the chosen --workdir (like D:\tools)
	os.MkdirAll(filepath.Join(base, "tools", "myproject"), 0o755)
	os.WriteFile(filepath.Join(base, "tools", "my-notes.txt"), []byte("mine"), 0o644)
	os.WriteFile(filepath.Join(base, "tools", "myproject", "a.c"), []byte("int main;"), 0o644)
	d, err := Prepare(base)
	if err != nil {
		t.Fatal(err)
	}
	if d.Path != filepath.Join(base, SubName) || d.ToolDir() == filepath.Join(base, "tools") {
		t.Fatalf("work folder %s tools %s", d.Path, d.ToolDir())
	}
	os.MkdirAll(d.ToolDir(), 0o755)
	for _, n := range []string{"fastboot", "adb"} {
		os.WriteFile(filepath.Join(d.ToolDir(), n), []byte("x"), 0o755)
		d.AddTool(n)
	}
	os.WriteFile(filepath.Join(d.Path, "super.raw"), []byte("rom"), 0o644)
	d.Cleanup(false)
	for _, p := range []string{"tools/my-notes.txt", "tools/myproject/a.c"} {
		if _, err := os.Stat(filepath.Join(base, p)); err != nil {
			t.Fatalf("%s deleted", p)
		}
	}
	if _, err := os.Stat(d.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("own work folder left behind")
	}
	if _, err := os.Stat(base); err != nil {
		t.Fatal("the chosen folder itself was removed")
	}
}

func TestCleanupNeverRemovesForeignFilesInsideOwnFolder(t *testing.T) {
	base := t.TempDir()
	d, _ := Prepare(base)
	os.MkdirAll(d.ToolDir(), 0o755)
	os.WriteFile(filepath.Join(d.ToolDir(), "keep.txt"), []byte("x"), 0o644) // put there while it ran
	os.WriteFile(filepath.Join(d.Path, "other.txt"), []byte("x"), 0o644)
	d.Cleanup(false)
	for _, p := range []string{filepath.Join(d.ToolDir(), "keep.txt"), filepath.Join(d.Path, "other.txt")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s deleted", p)
		}
	}
}

func TestPrepareRefusesFolderWithForeignFiles(t *testing.T) {
	base := t.TempDir()
	os.MkdirAll(filepath.Join(base, SubName), 0o755)
	os.WriteFile(filepath.Join(base, SubName, "thesis.docx"), []byte("x"), 0o644)
	if _, err := Prepare(base); err == nil {
		t.Fatal("took over a folder with the person's files")
	}
	// a folder left by an older flasher (only its own files) is taken over
	base2 := t.TempDir()
	os.MkdirAll(filepath.Join(base2, SubName, "tools"), 0o755)
	os.WriteFile(filepath.Join(base2, SubName, "super.raw"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(base2, SubName, "tools", "adb"), []byte("x"), 0o644)
	if _, err := Prepare(base2); err != nil {
		t.Fatal(err)
	}
}

func TestBaseCreatedIsRemovedOnlyIfEmpty(t *testing.T) {
	base := filepath.Join(t.TempDir(), "new")
	d, err := Prepare(base)
	if err != nil {
		t.Fatal(err)
	}
	d.Cleanup(false)
	if _, err := os.Stat(base); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("created base folder left behind")
	}
}

// Bug 5: a second copy cannot take the lock while the first holds it.
func TestLockRefusesSecondCopy(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	l1, err := Acquire(p)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Acquire(p)
	var b ErrBusy
	if !errors.As(err, &b) || b.PID != os.Getpid() {
		t.Fatalf("second Acquire: %v", err)
	}
	l1.Release()
	l2, err := Acquire(p)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	l2.Release()
}
