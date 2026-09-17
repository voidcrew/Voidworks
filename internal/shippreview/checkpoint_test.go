package shippreview

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoppedRebuildKeepsUnchangedCheckoutPreviews(t *testing.T) {
	root, client := incrementalFixture(t)
	refreshPreviews(t, root, client, false, "alpha.dmm", "beta.dmm", "room.dmm", "room_blue.dmm")
	output := filepath.Join(root, "voidcrew/modules/ship_upgrades/previews")
	original, err := os.ReadFile(filepath.Join(output, "alpha.png"))
	if err != nil {
		t.Fatal(err)
	}
	old := time.Unix(1234567890, 0)
	if err := os.Chtimes(filepath.Join(output, "alpha.png"), old, old); err != nil {
		t.Fatal(err)
	}
	// A previous fleet rebuild finished some images with a different PNG
	// encoding before being stopped. Pixels and source maps are unchanged.
	write(t, filepath.Join(root, Script), strings.Replace(incrementalGenerator, ".save(output)", ".save(output, compress_level=0)", 1))
	write(t, filepath.Join(root, "fail"), "")
	environment := filepath.Join(root, "selected.dme")
	client.RequestFull(root, environment)
	until(t, func() bool { return client.Status(root).Phase == "failed" })
	checkpoint, err := os.ReadFile(filepath.Join(client.folder(root), "render-progress/alpha.png"))
	if err != nil || bytes.Equal(original, checkpoint) {
		t.Fatalf("fixture did not produce a different cached PNG: %v", err)
	}
	client.Stop(root, environment)
	until(t, func() bool { return client.Status(root).Phase == "stopped" })
	if err := os.Remove(filepath.Join(root, "fail")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "renders"), "")
	client.Resume(root, environment)
	until(t, func() bool { return client.Status(root).Phase == "complete" })
	if data, _ := os.ReadFile(filepath.Join(root, "renders")); len(data) != 0 {
		t.Fatalf("resume rendered unchanged maps: %s", data)
	}
	assertPreviewUnchanged(t, filepath.Join(output, "alpha.png"), original, old)
	write(t, filepath.Join(root, "maps/beta.dmm"), "12 edited other ship")
	refreshPreviews(t, root, client, false, "beta.dmm")
	assertPreviewUnchanged(t, filepath.Join(output, "alpha.png"), original, old)
}

func TestGitRestoredPreviewIsNotReplacedByCachedCopy(t *testing.T) {
	root, client := incrementalFixture(t)
	all := []string{"alpha.dmm", "beta.dmm", "room.dmm", "room_blue.dmm"}
	refreshPreviews(t, root, client, false, all...)
	path := filepath.Join(root, "voidcrew/modules/ship_upgrades/previews/alpha.png")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, Script), strings.Replace(incrementalGenerator, ".save(output)", ".save(output, compress_level=0)", 1))
	refreshPreviews(t, root, client, true, all...)
	if data, _ := os.ReadFile(path); bytes.Equal(data, original) {
		t.Fatal("fixture did not produce a different PNG encoding")
	}
	// Model discarding an unrelated preview diff in Git, without erasing the
	// editor's local cache or the matching source hash in the manifest.
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Unix(1234567890, 0)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "maps/beta.dmm"), "12 edited other ship")
	refreshPreviews(t, root, client, false, "beta.dmm")
	assertPreviewUnchanged(t, path, original, old)
	refreshPreviews(t, root, client, false)
	assertPreviewUnchanged(t, path, original, old)
	// Explicit rebuild remains able to replace even a valid checkout preview.
	refreshPreviews(t, root, client, true, all...)
	if data, _ := os.ReadFile(path); bytes.Equal(data, original) {
		t.Fatal("explicit rebuild did not refresh the restored preview")
	}
}

func assertPreviewUnchanged(t *testing.T, path string, expected []byte, modified time.Time) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, expected) {
		t.Fatalf("unchanged checkout preview was replaced by cached bytes: %s (%v)", path, err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.ModTime().Equal(modified) {
		t.Fatalf("unchanged checkout preview was rewritten: %s (%v)", path, err)
	}
}
