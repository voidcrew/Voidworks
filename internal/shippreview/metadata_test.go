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

const nestedMetadataWriter = `
    for group in ("hulls", "modules"):
        for key, entry in manifest[group].items():
            document = {"tile_px": 32, "hulls": {}, "modules": {}}
            document[group][key] = entry
            directory = OUTPUT_DIR / group
            if group == "modules":
                directory = directory / "test" / "workshop"
            directory.mkdir(parents=True, exist_ok=True)
            (directory / (key.removesuffix(".dmm") + ".preview.json")).write_text(json.dumps(document))
`

func TestSplitMetadataMigrationIncrementalAndRemoval(t *testing.T) {
	for _, nested := range []bool{false, true} {
		name, writer, beta := "flat", splitMetadataWriter, "hull.beta.preview.json"
		if nested {
			name, writer, beta = "nested", nestedMetadataWriter, "hulls/beta.preview.json"
		}
		t.Run(name, func(t *testing.T) { testMetadataMigration(t, writer, beta) })
	}
}

func TestFlatToNestedMetadataReusesImages(t *testing.T) {
	root, client := incrementalFixture(t)
	flat := strings.Replace(incrementalGenerator, `    (OUTPUT_DIR / "manifest.json").write_text(json.dumps(manifest))`, splitMetadataWriter, 1)
	write(t, filepath.Join(root, Script), flat)
	refreshPreviews(t, root, client, false, "alpha.dmm", "beta.dmm", "room.dmm", "room_blue.dmm")
	output := filepath.Join(root, "voidcrew/modules/ship_upgrades/previews")
	before, err := os.ReadFile(filepath.Join(output, "module.room.dmm.preview.json"))
	if err != nil {
		t.Fatal(err)
	}
	nested := strings.Replace(incrementalGenerator, `    (OUTPUT_DIR / "manifest.json").write_text(json.dumps(manifest))`, nestedMetadataWriter, 1)
	write(t, filepath.Join(root, Script), nested)
	refreshPreviews(t, root, client, false)
	if data, _ := os.ReadFile(filepath.Join(output, "modules/test/workshop/room.preview.json")); !bytes.Equal(data, before) {
		t.Fatal("moving module metadata changed its contents")
	}
	flatFiles, _ := filepath.Glob(filepath.Join(output, "*.preview.json"))
	if len(flatFiles) != 0 {
		t.Fatal("migration left flattened metadata behind", flatFiles)
	}
	backups, _ := filepath.Glob(filepath.Join(client.CleanupBackups(root), "*", "module.room.dmm.preview.json"))
	if len(backups) != 1 {
		t.Fatal("flattened metadata has no recovery copy", backups)
	}
}

func testMetadataMigration(t *testing.T, writer, betaPath string) {
	root, client := incrementalFixture(t)
	refreshPreviews(t, root, client, false, "alpha.dmm", "beta.dmm", "room.dmm", "room_blue.dmm")
	output := filepath.Join(root, "voidcrew/modules/ship_upgrades/previews")
	legacy, _ := os.ReadFile(filepath.Join(output, "manifest.json"))
	script := strings.Replace(incrementalGenerator, `    (OUTPUT_DIR / "manifest.json").write_text(json.dumps(manifest))`, writer, 1)
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
	beta := filepath.Join(output, betaPath)
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
	backups, _ = filepath.Glob(filepath.Join(other.CleanupBackups(root), "*", betaPath))
	if len(backups) != 1 {
		t.Fatal("deleted metadata has no recovery copy", backups)
	}
	plan, err := other.ScanCleanup(root)
	if err != nil || len(plan.Files) != 0 {
		t.Fatalf("split metadata cleanup failed: %+v %v", plan, err)
	}
	// Switching back to an older checkout must not leave shards shadowing it.
	write(t, filepath.Join(root, Script), incrementalGenerator)
	refreshPreviews(t, root, other, false)
	err = filepath.Walk(output, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".preview.json") {
			t.Error("old generator left active split metadata", path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
