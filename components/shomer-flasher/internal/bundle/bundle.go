// Package bundle reads and writes the payload appended to the end of the
// shomer-flasher executable.
//
// Layout of a finished flasher file:
//
//	[ program (ELF or PE) ][ tar archive ][ trailer, 64 bytes ]
//
// The trailer is: magic "SHOMERFL" (8) | format version uint64 (8) |
// tar offset uint64 (8) | tar length uint64 (8) | sha256 of manifest.json (32).
// All integers are little endian. Operating systems ignore bytes after the
// program image, so the file still runs as a normal executable.
package bundle

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	Magic       = "SHOMERFL"
	Version     = 1
	TrailerSize = 64
)

// FileInfo describes one file inside the payload.
type FileInfo struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Manifest is manifest.json inside the payload.
type Manifest struct {
	Name      string              `json:"name"`
	Build     string              `json:"build"`
	ROMCommit string              `json:"rom_commit"`
	BuiltAt   string              `json:"built_at"`
	SuperGz   string              `json:"super_gz"`  // path inside the payload
	SuperRaw  FileInfo            `json:"super_raw"` // the decompressed super partition
	Files     map[string]FileInfo `json:"files"`     // every file in the payload except manifest.json
	VBMeta    []string            `json:"vbmeta"`    // partition names, file = "avb/<name>"
	Products  []string            `json:"products"`  // accepted `fastboot getvar product` prefixes
	Models    []string            `json:"models"`    // accepted ro.product.model values (substring)
	Builds    []string            `json:"builds"`    // accepted software builds (substring of ro.build.version.incremental)
	Tools     []string            `json:"tools"`     // paths inside the payload, extracted to the tools dir
	// PlatformTools is the Android platform-tools release fastboot/adb come from
	// (Pkg.Revision of its source.properties; pinned and SHA-256 checked by the release workflow).
	PlatformTools string `json:"platform_tools,omitempty"`
	// Recovery images for the backup-first step (absent in flashers built before Nov 2026).
	Recovery *RecoveryFiles `json:"recovery,omitempty"`
}

// RecoveryFiles names the two recovery images inside the payload.
type RecoveryFiles struct {
	Dump  string `json:"dump"`  // neutronscott's recovery2.img (su + busybox): only used to READ the phone
	Stock string `json:"stock"` // genuine stock TCL KEKA recovery: written back right after the backup
}

// Entry is one file's location inside the executable.
type Entry struct {
	Name   string
	Offset int64 // absolute offset in the executable
	Size   int64
}

// Payload is an opened payload.
type Payload struct {
	f           *os.File
	Manifest    Manifest
	Entries     map[string]Entry
	ManifestSHA string
}

// Open reads the trailer and the tar index of the payload appended to path.
func Open(path string) (*Payload, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if st.Size() < TrailerSize {
		f.Close()
		return nil, errors.New("this file has no Shomer ROM inside (too small)")
	}
	tr := make([]byte, TrailerSize)
	if _, err := f.ReadAt(tr, st.Size()-TrailerSize); err != nil {
		f.Close()
		return nil, err
	}
	if string(tr[:8]) != Magic {
		f.Close()
		return nil, errors.New("this file has no Shomer ROM inside (no payload marker). Download the full flasher from the GitHub Release")
	}
	ver := binary.LittleEndian.Uint64(tr[8:16])
	if ver != Version {
		f.Close()
		return nil, fmt.Errorf("payload format %d is not supported by this program", ver)
	}
	off := int64(binary.LittleEndian.Uint64(tr[16:24]))
	ln := int64(binary.LittleEndian.Uint64(tr[24:32]))
	if off < 0 || ln <= 0 || off+ln != st.Size()-TrailerSize {
		f.Close()
		return nil, errors.New("payload is damaged (bad offsets) - download the flasher again")
	}
	p := &Payload{f: f, Entries: map[string]Entry{}}
	sec := io.NewSectionReader(f, off, ln)
	// A tar reader over a counting reader lets us record data offsets.
	cr := &countingReader{r: sec}
	t := tar.NewReader(cr)
	var manifestBytes []byte
	for {
		h, err := t.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("payload is damaged: %v - download the flasher again", err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		e := Entry{Name: h.Name, Offset: off + cr.n, Size: h.Size}
		p.Entries[h.Name] = e
		if h.Name == "manifest.json" {
			manifestBytes, err = io.ReadAll(t)
			if err != nil {
				f.Close()
				return nil, err
			}
		}
	}
	if manifestBytes == nil {
		f.Close()
		return nil, errors.New("payload is damaged: no manifest - download the flasher again")
	}
	sum := sha256.Sum256(manifestBytes)
	p.ManifestSHA = hex.EncodeToString(sum[:])
	if !bytes.Equal(sum[:], tr[32:64]) {
		f.Close()
		return nil, errors.New("payload is damaged: manifest checksum mismatch - download the flasher again")
	}
	if err := json.Unmarshal(manifestBytes, &p.Manifest); err != nil {
		f.Close()
		return nil, err
	}
	return p, nil
}

func (p *Payload) Close() error { return p.f.Close() }

// Reader returns a reader over one payload file.
func (p *Payload) Reader(name string) (*io.SectionReader, error) {
	e, ok := p.Entries[name]
	if !ok {
		return nil, fmt.Errorf("payload is missing %s", name)
	}
	return io.NewSectionReader(p.f, e.Offset, e.Size), nil
}

// Verify checks size and sha256 of every file listed in the manifest.
// progress (may be nil) is called with bytes done / total.
func (p *Payload) Verify(progress func(done, total int64)) error {
	var total, done int64
	for _, fi := range p.Manifest.Files {
		total += fi.Size
	}
	for name, fi := range p.Manifest.Files {
		r, err := p.Reader(name)
		if err != nil {
			return err
		}
		if r.Size() != fi.Size {
			return fmt.Errorf("%s has the wrong size (%d, expected %d) - download the flasher again", name, r.Size(), fi.Size)
		}
		h := sha256.New()
		buf := make([]byte, 4<<20)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				h.Write(buf[:n])
				done += int64(n)
				if progress != nil {
					progress(done, total)
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
		}
		got := hex.EncodeToString(h.Sum(nil))
		if got != fi.SHA256 {
			return fmt.Errorf("CHECKSUM MISMATCH in %s: got %s, expected %s. The download is damaged or was changed. Download the flasher again", name, got, fi.SHA256)
		}
	}
	return nil
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.n += int64(n)
	return n, err
}

// Write appends a payload made of files (name -> source path) and manifest to
// out, which must already hold the program. Used by cmd/mkbundle.
func Write(out *os.File, m Manifest, files map[string]string, order []string) error {
	start, err := out.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	mb, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tw := tar.NewWriter(out)
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0644, Size: int64(len(mb)), Typeflag: tar.TypeReg, Format: tar.FormatPAX}); err != nil {
		return err
	}
	if _, err := tw.Write(mb); err != nil {
		return err
	}
	for _, name := range order {
		src := files[name]
		f, err := os.Open(src)
		if err != nil {
			return err
		}
		st, _ := f.Stat()
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: st.Size(), Typeflag: tar.TypeReg, Format: tar.FormatPAX}); err != nil {
			f.Close()
			return err
		}
		if _, err := io.Copy(tw, f); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
	if err := tw.Close(); err != nil {
		return err
	}
	end, err := out.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	tr := make([]byte, TrailerSize)
	copy(tr, Magic)
	binary.LittleEndian.PutUint64(tr[8:], Version)
	binary.LittleEndian.PutUint64(tr[16:], uint64(start))
	binary.LittleEndian.PutUint64(tr[24:], uint64(end-start))
	sum := sha256.Sum256(mb)
	copy(tr[32:], sum[:])
	_, err = out.Write(tr)
	return err
}
