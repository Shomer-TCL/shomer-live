// Command mkbundle appends the Shomer ROM payload to a built flasher program.
//
//	mkbundle -stub shomer-flasher-stub -out shomer-flasher \
//	  -super super.raw.gz -super-raw-sha <sha> -super-raw-size <bytes> \
//	  -vbmeta-dir stock/KEKA -tools platform-tools-dir -tool-files fastboot,adb \
//	  -build 2026-10-30_220fdd6 -commit 220fdd6
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Shomer-TCL/shomer-flasher/internal/bundle"
	"github.com/Shomer-TCL/shomer-flasher/internal/flasher"
	"github.com/Shomer-TCL/shomer-flasher/internal/flashplan"
)

func sum(path string) (bundle.FileInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return bundle.FileInfo{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return bundle.FileInfo{SHA256: hex.EncodeToString(h.Sum(nil)), Size: n}, err
}

func main() {
	stub := flag.String("stub", "", "built flasher program without payload")
	out := flag.String("out", "", "output file")
	super := flag.String("super", "", "super.raw.gz")
	rawSHA := flag.String("super-raw-sha", "", "sha256 of the decompressed super")
	rawSize := flag.Int64("super-raw-size", 0, "size of the decompressed super")
	vbdir := flag.String("vbmeta-dir", "", "folder with stock vbmeta, vbmeta_system, vbmeta_vendor")
	vbsha := flag.String("vbmeta-sha", "", "comma list name=sha256 the vbmeta files must have")
	tools := flag.String("tools", "", "platform-tools folder")
	toolFiles := flag.String("tool-files", "fastboot,adb", "comma list of files to take from -tools")
	goos := flag.String("goos", "linux", "linux|windows")
	build := flag.String("build", "", "image build id")
	commit := flag.String("commit", "", "ROM repo commit")
	dumpRec := flag.String("dump-recovery", "", "neutronscott recovery2.img for the backup step (required; sha256 pinned in internal/flasher and in the plan's backup_first)")
	stockRec := flag.String("stock-recovery", "", "genuine stock TCL KEKA recovery written back after the backup (required; sha256 pinned)")
	planPath := flag.String("flash-plan", "", "flash-plan.json of the image build (required: the one source of truth for what gets written)")
	flag.Parse()

	die := func(f string, a ...any) { fmt.Fprintf(os.Stderr, "mkbundle: "+f+"\n", a...); os.Exit(1) }
	if *stub == "" || *out == "" || *super == "" || *rawSHA == "" || *rawSize == 0 || *vbdir == "" || *tools == "" || *planPath == "" {
		die("missing flags (-flash-plan is required since Nov 2 2026)")
	}
	m := bundle.Manifest{
		Name:      "Shomer OS",
		Build:     *build,
		ROMCommit: *commit,
		BuiltAt:   time.Now().UTC().Format(time.RFC3339),
		SuperGz:   "rom/super.raw.gz",
		SuperRaw:  bundle.FileInfo{SHA256: *rawSHA, Size: *rawSize},
		Files:     map[string]bundle.FileInfo{},
		VBMeta:    []string{"vbmeta", "vbmeta_system", "vbmeta_vendor"},
		Products:  []string{"gflip6"},
		Models:    []string{"T408DL", "4058G"},
		Builds:    []string{"KEKA", "UPCI"},
	}
	files := map[string]string{}
	var order []string
	add := func(name, src string) {
		fi, err := sum(src)
		if err != nil {
			die("%s: %v", src, err)
		}
		m.Files[name] = fi
		files[name] = src
		order = append(order, name)
	}
	want := map[string]string{}
	for _, kv := range strings.Split(*vbsha, ",") {
		if p := strings.SplitN(kv, "=", 2); len(p) == 2 {
			want[p[0]] = p[1]
		}
	}
	for _, t := range strings.Split(*toolFiles, ",") {
		name := "tools/" + *goos + "/" + t
		add(name, filepath.Join(*tools, t))
		m.Tools = append(m.Tools, name)
	}
	// which platform-tools release fastboot/adb are (shown by --info)
	if b, err := os.ReadFile(filepath.Join(*tools, "source.properties")); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(l), "Pkg.Revision="); ok {
				m.PlatformTools = strings.TrimSpace(v)
			}
		}
	}
	for _, v := range m.VBMeta {
		src := filepath.Join(*vbdir, v)
		add("avb/"+v, src)
		if w := want[v]; w != "" && m.Files["avb/"+v].SHA256 != w {
			die("%s sha mismatch: %s != %s", v, m.Files["avb/"+v].SHA256, w)
		}
	}
	// The flash plan published with the image build must describe exactly what this flasher will write.
	plan, err := flashplan.Load(*planPath)
	if err != nil {
		die("%v", err)
	}
	vbs := map[string]string{}
	for _, v := range m.VBMeta {
		vbs[v] = m.Files["avb/"+v].SHA256
	}
	if err := plan.Check(m.VBMeta, vbs, *rawSHA, flasher.AllowedPartitions); err != nil {
		die("flash plan check failed: %v", err)
	}
	add("flash-plan.json", *planPath)
	fmt.Println("flash plan OK: vbmeta, vbmeta_system, vbmeta_vendor (verification off) + super, as the image build says")
	// Since 1.4.0 the plan MUST carry backup_first naming exactly the two pinned recovery writes, and the
	// flasher MUST bundle both images: no flasher without backup-first, no recovery write the plan doesn't name.
	if err := plan.CheckBackup(flasher.RecoveryPartition, flasher.DumpRecoverySHA256, flasher.StockRecoverySHA256, flasher.RecoverySize); err != nil {
		die("flash plan check failed: %v", err)
	}
	if *dumpRec == "" || *stockRec == "" {
		die("the flash plan's backup_first section needs both -dump-recovery (recovery2.img) and -stock-recovery (genuine stock KEKA recovery)")
	}
	add("recovery/dump-recovery2.img", *dumpRec)
	add("recovery/stock-recovery-KEKA.img", *stockRec)
	if fi := m.Files["recovery/dump-recovery2.img"]; fi.SHA256 != flasher.DumpRecoverySHA256 || fi.Size != flasher.RecoverySize {
		die("dump recovery is not the pinned recovery2.img: sha256 %s, %d bytes", fi.SHA256, fi.Size)
	}
	if fi := m.Files["recovery/stock-recovery-KEKA.img"]; fi.SHA256 != flasher.StockRecoverySHA256 || fi.Size != flasher.RecoverySize {
		die("stock recovery is not the pinned stock KEKA recovery: sha256 %s, %d bytes", fi.SHA256, fi.Size)
	}
	m.Recovery = &bundle.RecoveryFiles{Dump: "recovery/dump-recovery2.img", Stock: "recovery/stock-recovery-KEKA.img"}
	fmt.Println("flash plan backup_first OK: recovery2 (dump) then genuine stock KEKA recovery (restore), both pinned, both bundled")
	add(m.SuperGz, *super) // last: biggest

	in, err := os.Open(*stub)
	if err != nil {
		die("%v", err)
	}
	o, err := os.OpenFile(*out, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o755)
	if err != nil {
		die("%v", err)
	}
	if _, err := io.Copy(o, in); err != nil {
		die("%v", err)
	}
	in.Close()
	if err := bundle.Write(o, m, files, order); err != nil {
		die("%v", err)
	}
	if err := o.Close(); err != nil {
		die("%v", err)
	}
	fi, _ := sum(*out)
	fmt.Printf("%s  %d bytes  sha256 %s\n", *out, fi.Size, fi.SHA256)
}
