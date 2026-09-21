package shippreview

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const splitMetadataWriter = `
    for group, prefix in (("hulls", "hull"), ("modules", "module")):
        for key, entry in manifest[group].items():
            document = {"tile_px": 32, "hulls": {}, "modules": {}}
            document[group][key] = entry
            (OUTPUT_DIR / (prefix + "." + key + ".preview.json")).write_text(json.dumps(document))
`

func TestSplitMetadataMigrationIncrementalAndRemoval(t *testing.T) {
	root, client := incrementalFixture(t)
	refreshPreviews(t, root, client, false, "alpha.dmm", "beta.dmm", "room.dmm", "room_blue.dmm")
	output := filepath.Join(root, "voidcrew/modules/ship_upgrades/previews")
	legacy, _ := os.ReadFile(filepath.Join(output, "manifest.json"))
	script := strings.Replace(incrementalGenerator, `    (OUTPUT_DIR / "manifest.json").write_text(json.dumps(manifest))`, splitMetadataWriter, 1)
	write(t, filepath.Join(root, Script), script)
	// Changing metadata layout does not require rendering unchanged maps again.
	refreshPreviews(t, root, client, false)
	if _, err := os.Stat(filepath.Join(output, "manifest.json")); !os.IsNotExist(err) {
		t.Fatal("migration retained the shared manifest", err)
	}
	backups, _ := filepath.Glob(filepath.Join(client.CleanupBackups(root), "*", "manifest.json"))
	if len(backups) != 1 {
		t.Fatal("migration did not back up the original manifest")
	}
	if data, _ := os.ReadFile(backups[0]); !bytes.Equal(data, legacy) {
		t.Fatal("migration backup differs")
	}
	beta := filepath.Join(output, "hull.beta.preview.json")
	before, _ := os.ReadFile(beta)
	info, _ := os.Stat(beta)
	write(t, filepath.Join(root, "maps/alpha.dmm"), "16 changed alpha")
	refreshPreviews(t, root, client, false, "alpha.dmm")
	if data, _ := os.ReadFile(beta); !bytes.Equal(data, before) {
		t.Fatal("unrelated hull metadata changed")
	}
	if after, _ := os.Stat(beta); !after.ModTime().Equal(info.ModTime()) {
		t.Fatal("unchanged metadata was rewritten")
	}
	// A new local cache can reuse committed shards, just like the old manifest.
	other := New(t.TempDir())
	refreshPreviews(t, root, other, false)
	if err := os.Remove(filepath.Join(root, "maps/beta.dmm")); err != nil {
		t.Fatal(err)
	}
	refreshPreviews(t, root, other, false)
	if _, err := os.Stat(beta); !os.IsNotExist(err) {
		t.Fatal("deleted hull retained active metadata", err)
	}
	plan, err := other.ScanCleanup(root)
	if err != nil || len(plan.Files) != 0 {
		t.Fatalf("split metadata cleanup failed: %+v %v", plan, err)
	}
	// Switching back to an older checkout must not leave shards shadowing it.
	write(t, filepath.Join(root, Script), incrementalGenerator)
	refreshPreviews(t, root, other, false)
	shards, _ := filepath.Glob(filepath.Join(output, "*.preview.json"))
	if len(shards) != 0 {
		t.Fatal("old generator left active split metadata", shards)
	}
}
