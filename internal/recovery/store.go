// Package recovery keeps crash-safe drafts separately from project files.
package recovery

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Snapshot struct {
	Version     int
	Environment string
	Created     time.Time
	Names       []string
	Data        []byte
	Checksum    string
}
type Store struct {
	folder, environment string
	lock                *os.File
}
type Entry struct {
	Store    *Store
	Snapshot *Snapshot
	Err      error
}

func projectFolder(root, environment string) string {
	path, _ := filepath.Abs(environment)
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	sum := sha256.Sum256([]byte(path))
	return filepath.Join(root, hex.EncodeToString(sum[:]))
}
func Open(root, environment string) (*Store, error) {
	parent := projectFolder(root, environment)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(parent, "session-")
	if err != nil {
		return nil, err
	}
	lock, err := lockSession(filepath.Join(dir, "active.lock"))
	if err != nil {
		return nil, err
	}
	return &Store{dir, environment, lock}, nil
}
func (s *Store) Close() {
	if s != nil && s.lock != nil {
		_ = s.lock.Close()
		s.lock = nil
	}
}
func (s *Store) Folder() string { return s.folder }
func snapshots(folder string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(folder, "snapshot-*.json.gz"))
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	return paths, err
}

// Save publishes only a fully written, synced snapshot. Two generations survive
// so an interrupted write or a damaged newest copy cannot erase the last draft.
func (s *Store) Save(data []byte, names []string, now time.Time) error {
	if s.lock == nil {
		return fmt.Errorf("recovery session is closed")
	}
	sum := sha256.Sum256(data)
	snapshot := Snapshot{1, s.environment, now, names, data, hex.EncodeToString(sum[:])}
	temp, err := os.CreateTemp(s.folder, ".writing-")
	if err != nil {
		return err
	}
	path := temp.Name()
	defer os.Remove(path)
	gz := gzip.NewWriter(temp)
	err = json.NewEncoder(gz).Encode(snapshot)
	if closeErr := gz.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	generation := now.UnixNano()
	previous, _ := snapshots(s.folder)
	if len(previous) > 0 {
		if n, e := strconv.ParseInt(strings.Split(filepath.Base(previous[0]), "-")[1], 10, 64); e == nil && n >= generation {
			generation = n + 1
		}
	}
	final := filepath.Join(s.folder, fmt.Sprintf("snapshot-%020d-%s.json.gz", generation, filepath.Base(path)))
	if err = os.Rename(path, final); err != nil {
		return err
	}
	files, err := snapshots(s.folder)
	if err != nil {
		return err
	}
	for _, old := range files[min(2, len(files)):] {
		if err = os.Remove(old); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) Clear() error {
	files, err := snapshots(s.folder)
	if err != nil {
		return err
	}
	for _, p := range files {
		if err = os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
func (s *Store) Discard() error {
	if err := s.Clear(); err != nil {
		return err
	}
	s.Close()
	return nil
}
func read(path, environment string) (*Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	// Reading to EOF verifies gzip's trailer as well as the snapshot checksum.
	data, err := io.ReadAll(io.LimitReader(gz, 512<<20+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 512<<20 {
		return nil, fmt.Errorf("recovery copy is too large")
	}
	var v Snapshot
	if err = json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	if v.Version != 1 || projectFolder("", v.Environment) != projectFolder("", environment) {
		return nil, fmt.Errorf("recovery belongs to a different project or version")
	}
	sum := sha256.Sum256(v.Data)
	if hex.EncodeToString(sum[:]) != v.Checksum {
		return nil, fmt.Errorf("recovery checksum does not match")
	}
	return &v, nil
}

// Pending claims only inactive sessions. The OS releases locks after a crash;
// another running editor's snapshots are never offered or cleared.
func Pending(root, environment string) ([]Entry, error) {
	dirs, err := filepath.Glob(filepath.Join(projectFolder(root, environment), "session-*"))
	if err != nil {
		return nil, err
	}
	var result []Entry
	for _, dir := range dirs {
		files, e := snapshots(dir)
		if e != nil {
			return nil, e
		}
		if len(files) == 0 {
			continue
		}
		lock, e := lockSession(filepath.Join(dir, "active.lock"))
		if e != nil {
			continue
		}
		store := &Store{dir, environment, lock}
		entry := Entry{Store: store}
		for _, file := range files {
			entry.Snapshot, entry.Err = read(file, environment)
			if entry.Err == nil {
				break
			}
		}
		result = append(result, entry)
	}
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i].Snapshot, result[j].Snapshot
		if a == nil {
			return false
		}
		if b == nil {
			return true
		}
		return a.Created.After(b.Created)
	})
	return result, nil
}
func HasPending(root, environment string) bool {
	entries, err := Pending(root, environment)
	for _, e := range entries {
		e.Store.Close()
	}
	return err == nil && len(entries) > 0
}
