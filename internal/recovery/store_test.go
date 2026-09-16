package recovery

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDiscoverAllMissingSourcesAndLiveSessions(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"deleted.dmi", "untitled-dmi", "still-open.dmi"} {
		s, err := Open(root, filepath.Join(t.TempDir(), name))
		if err != nil {
			t.Fatal(err)
		}
		if err = s.Save([]byte(name), []string{name}, time.Now()); err != nil {
			t.Fatal(err)
		}
		if name == "still-open.dmi" {
			defer s.Close()
		} else {
			s.Close()
		}
	}
	entries, err := PendingAll(root)
	if err != nil || len(entries) != 2 {
		t.Fatal("missing inactive drafts or exposed active editor", len(entries), err)
	}
	for _, entry := range entries {
		if entry.Err != nil || string(entry.Snapshot.Data) == "still-open.dmi" {
			t.Fatal("incorrect recovery discovery", entry.Err)
		}
	}
	second, err := PendingAll(root)
	if err != nil || len(second) != 0 {
		t.Fatal("claimed drafts offered a second time", err)
	}
	for _, entry := range entries {
		entry.Store.Close()
	}
}

func TestAtomicGenerationsLiveSessionsAndCorruptionFallback(t *testing.T) {
	root := t.TempDir()
	environment := filepath.Join(t.TempDir(), "game.dme")
	s, err := Open(root, environment)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	for n := 0; n < 3; n++ {
		if err = s.Save([]byte{byte(n)}, []string{"Unsaved ship"}, now.Add(time.Duration(n))); err != nil {
			t.Fatal(err)
		}
	}
	files, _ := snapshots(s.folder)
	if len(files) != 2 {
		t.Fatalf("expected two generations, got %d", len(files))
	}
	if HasPending(root, environment) {
		t.Fatal("offered a live editor's recovery")
	}
	if err = os.WriteFile(filepath.Join(s.folder, ".writing-interrupted"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(files[0], []byte("damaged newest snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	s.Close()
	entries, err := Pending(root, environment)
	if err != nil || len(entries) != 1 {
		t.Fatalf("missing recovery: %v, %v", entries, err)
	}
	defer entries[0].Store.Close()
	if entries[0].Err != nil || !bytes.Equal(entries[0].Snapshot.Data, []byte{1}) {
		t.Fatal("failed to fall back to intact generation")
	}
	if HasPending(root, environment) {
		t.Fatal("same recovery offered to two editors")
	}
	if HasPending(root, filepath.Join(filepath.Dir(environment), "other.dme")) {
		t.Fatal("recovery crossed environments")
	}
	other, err := Open(root, environment)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err = other.Save([]byte("other"), nil, now); err != nil {
		t.Fatal(err)
	}
	if err = entries[0].Store.Discard(); err != nil {
		t.Fatal(err)
	}
	files, _ = snapshots(other.folder)
	if len(files) != 1 {
		t.Fatal("discard deleted another session")
	}
}
func TestRecoverySurvivesKilledProcess(t *testing.T) {
	if os.Getenv("VOIDWORKS_RECOVERY_CHILD") == "1" {
		s, err := Open(os.Getenv("VOIDWORKS_RECOVERY_ROOT"), "crash.dme")
		if err != nil {
			os.Exit(2)
		}
		if err = s.Save([]byte("unfinished ship edits"), []string{"Crash fixture"}, time.Now()); err != nil {
			os.Exit(3)
		}
		if err = os.WriteFile(os.Getenv("VOIDWORKS_RECOVERY_READY"), []byte("ready"), 0600); err != nil {
			os.Exit(4)
		}
		for {
			time.Sleep(time.Second)
		}
	}
	root := t.TempDir()
	ready := filepath.Join(root, "ready")
	cmd := exec.Command(os.Args[0], "-test.run=^TestRecoverySurvivesKilledProcess$")
	cmd.Env = append(os.Environ(), "VOIDWORKS_RECOVERY_CHILD=1", "VOIDWORKS_RECOVERY_ROOT="+root, "VOIDWORKS_RECOVERY_READY="+ready)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("crash fixture did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if HasPending(root, "crash.dme") {
		t.Fatal("live child was recoverable")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	entries, err := Pending(root, "crash.dme")
	if err != nil || len(entries) != 1 {
		t.Fatalf("crash recovery missing: %v", err)
	}
	defer entries[0].Store.Close()
	if entries[0].Err != nil || string(entries[0].Snapshot.Data) != "unfinished ship edits" {
		t.Fatal("crash lost the snapshot")
	}
}
func TestFailedWritePreservesPreviousSnapshot(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root, "project.dme")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Save([]byte("first"), nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err = s.Save([]byte("second"), nil, time.Now()); err == nil {
		t.Fatal("write to closed session succeeded")
	}
	entries, err := Pending(root, "project.dme")
	if err != nil || len(entries) != 1 {
		t.Fatal("previous snapshot missing")
	}
	defer entries[0].Store.Close()
	if string(entries[0].Snapshot.Data) != "first" {
		t.Fatal("failed write damaged previous snapshot")
	}
}
