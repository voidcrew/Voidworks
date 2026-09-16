package dmi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Document struct {
	Path        string
	Icon, Saved *Icon
	Baseline    []byte
	Revision    uint64
	Backup      string
}

func Load(path string) (*Document, error) {
	path, e := filepath.Abs(path)
	if e != nil {
		return nil, e
	}
	i, e := Open(path)
	if e != nil {
		return nil, e
	}
	sum := sha256.Sum256(i.Original)
	return &Document{Path: path, Icon: i, Saved: i, Baseline: sum[:], Revision: 1}, nil
}
func Create(width, height int) (*Document, error) {
	i, e := New(width, height)
	if e != nil {
		return nil, e
	}
	return &Document{Icon: i, Revision: 1}, nil
}
func (d *Document) Modified() bool  { return d.Icon != d.Saved }
func (d *Document) Restore(i *Icon) { d.Icon = i; d.Revision++ }
func (d *Document) Apply(change func(*Icon) error) (before, after *Icon, err error) {
	before = d.Icon
	after = before.Clone()
	after.Changed = true
	if err = change(after); err != nil {
		return before, before, err
	}
	if err = after.Validate(); err != nil {
		return before, before, err
	}
	d.Restore(after)
	return before, after, nil
}
func readBaseline(path string) ([]byte, []byte, error) {
	info, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return nil, nil, nil
	}
	if e != nil {
		return nil, nil, e
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("save target must be a regular file")
	}
	if info.Size() > 256<<20 {
		return nil, nil, fmt.Errorf("save target is too large")
	}
	data, e := os.ReadFile(path)
	if e != nil {
		return nil, nil, e
	}
	sum := sha256.Sum256(data)
	return data, sum[:], nil
}
func (d *Document) ExternalChange() bool {
	if d.Path == "" {
		return false
	}
	_, sum, e := readBaseline(d.Path)
	return e != nil || !bytes.Equal(sum, d.Baseline)
}

// Save checks the original content twice, saves a verified recovery copy, then
// atomically replaces the target. A failed save leaves the document modified.
func (d *Document) Save(path, backupRoot string) error {
	path, e := filepath.Abs(path)
	if e != nil {
		return e
	}
	if !strings.EqualFold(filepath.Ext(path), ".dmi") {
		return fmt.Errorf("save DMI files with a .dmi extension")
	}
	old, sum, e := readBaseline(path)
	if e != nil {
		return e
	}
	same := SamePath(path, d.Path)
	if same {
		if !bytes.Equal(sum, d.Baseline) {
			return fmt.Errorf("the DMI changed outside Voidworks; reload it or save a separate copy")
		}
	} else if old != nil {
		return fmt.Errorf("this file already exists; choose a new filename")
	}
	model := d.Icon
	if d.Path != "" && !strings.EqualFold(filepath.Ext(d.Path), ".dmi") {
		model = model.Clone()
		model.Changed = true
	}
	data, e := Encode(model)
	if e != nil {
		return e
	}
	if _, e = Decode(data); e != nil {
		return fmt.Errorf("encoded DMI failed verification: %w", e)
	}
	if bytes.Equal(data, old) {
		d.Path = path
		d.Saved = d.Icon
		d.Baseline = sum
		return nil
	}
	if old != nil {
		if backupRoot == "" {
			return fmt.Errorf("a recovery backup folder is required")
		}
		key := sha256.Sum256([]byte(path))
		folder := filepath.Join(backupRoot, hex.EncodeToString(key[:8]))
		if e = os.MkdirAll(folder, 0700); e != nil {
			return e
		}
		f, e := os.CreateTemp(folder, time.Now().Format("20060102-150405")+"-*.dmi")
		if e != nil {
			return e
		}
		backup := f.Name()
		_, e = f.Write(old)
		if e == nil {
			e = f.Sync()
		}
		if ce := f.Close(); e == nil {
			e = ce
		}
		if e != nil {
			return e
		}
		verify, e := os.ReadFile(backup)
		if e != nil || !bytes.Equal(verify, old) {
			return fmt.Errorf("could not verify recovery backup")
		}
		d.Backup = backup
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".voidworks-dmi-*")
	if e != nil {
		return e
	}
	temp := f.Name()
	defer os.Remove(temp)
	_, e = f.Write(data)
	if e == nil {
		e = f.Sync()
	}
	if ce := f.Close(); e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if info, e := os.Stat(path); e == nil {
		if e = os.Chmod(temp, info.Mode().Perm()); e != nil {
			return e
		}
	}
	_, current, e := readBaseline(path)
	if e != nil {
		return e
	}
	if !bytes.Equal(current, sum) {
		return fmt.Errorf("file changed while saving; original was preserved")
	}
	if e = os.Rename(temp, path); e != nil {
		return e
	}
	hash := sha256.Sum256(data)
	d.Baseline = hash[:]
	d.Path = path
	d.Saved = d.Icon
	return nil
}

// SamePath compares normalized file names using the host's case rules.
func SamePath(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	a, _ = filepath.Abs(a)
	b, _ = filepath.Abs(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// Export writes a new file without replacing an existing document. A temporary
// file and hard link make the final creation exclusive, including concurrent
// writers. Refusing replacement keeps exports out of the document save path.
func Export(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".voidworks-export-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Link(f.Name(), path); err != nil {
		return fmt.Errorf("export could not create the destination; choose a new filename: %w", err)
	}
	return nil
}
