package ship

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"sdmm/internal/util"
)

func TestRenameShipFilesSaveReopenAndUndo(t *testing.T) {
	c, env := authorEnvironment(t)
	p, err := NewProject(c, env, "original", "Original", 16, 16)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.addRect(0, "cargo", "Cargo", util.Point{X: 3, Y: 3, Z: 1}, util.Point{X: 5, Y: 5, Z: 1}); err != nil {
		t.Fatal(err)
	}
	if err = p.AddTheme(0, "medical", "Medical", true); err != nil {
		t.Fatal(err)
	}
	if err = p.SetPartCosts("ship", PartCosts{"science": 7, "trade": 3}); err != nil {
		t.Fatal(err)
	}
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	before := p.Capture()
	original := map[string][]byte{}
	for _, path := range p.authoredPaths() {
		original[path], _ = os.ReadFile(path)
	}
	for path, d := range p.Documents {
		if d.Active {
			original[path], _ = os.ReadFile(path)
		}
	}
	if err = p.renameShipAndFiles("New Explorer"); err != nil {
		t.Fatal(err)
	}
	if p.Hull.Type != before.Hull.Type || p.Settings.ID != "original" {
		t.Fatal("rename changed game identity")
	}
	after := p.Capture()
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	if p.Modified() {
		t.Fatal("saved rename is still dirty")
	}
	for path := range original {
		if path == env.RootFile {
			continue
		}
		if _, err = os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("old file remains: %s (%v)", path, err)
		}
	}
	for _, theme := range p.Hull.Themes {
		if _, err = p.Assemble(theme, map[string]string{"cargo": "cargo_basic"}); err != nil {
			t.Fatal(err)
		}
	}
	reopened, err := OpenProject(c, env, p.Hull)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Modified() || reopened.Hull.Name != "New Explorer" {
		t.Fatal("renamed project did not reopen cleanly")
	}
	cost, err := reopened.PartCosts("ship")
	if err != nil || !cost.Equal(PartCosts{"science": 7, "trade": 3}) {
		t.Fatalf("rename lost ship price: %v, %v", cost, err)
	}
	p.Restore(before)
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	for path, want := range original {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("undo did not restore %s: %v", path, err)
		}
	}
	p.Restore(after)
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	if p.Modified() {
		t.Fatal("redo did not save cleanly")
	}
}

func TestRenameFilesRejectsCollisionWithoutMovingSource(t *testing.T) {
	c, env := authorEnvironment(t)
	p, err := NewProject(c, env, "original", "Original", 16, 16)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	want := []byte("another mapper's file")
	target := filepath.Join(c.Root, "_maps/voidcrew/ships/ship_original_new_name.dmm")
	if err = os.WriteFile(target, want, 0600); err != nil {
		t.Fatal(err)
	}
	if err = p.Rename("theme/standard", "New Name"); err == nil {
		t.Fatal("rename overwrote a destination")
	}
	if p.Modified() {
		t.Fatal("failed rename changed the ship")
	}
	got, _ := os.ReadFile(target)
	if !bytes.Equal(got, want) {
		t.Fatal("rename changed an existing destination")
	}
}

func TestSaveFleetWithRenamedShipDoesNotRestoreOldIncludes(t *testing.T) {
	c, env := authorEnvironment(t)
	p, err := NewProject(c, env, "first", "First", 16, 16)
	if err != nil {
		t.Fatal(err)
	}
	q, err := NewProject(c, env, "second", "Second", 16, 16)
	if err != nil {
		t.Fatal(err)
	}
	if err = SaveProjects([]*Project{p, q}); err != nil {
		t.Fatal(err)
	}
	if err = p.renameShipAndFiles("Explorer"); err != nil {
		t.Fatal(err)
	}
	if err = q.SetPartCosts("ship", PartCosts{"misc": 8}); err != nil {
		t.Fatal(err)
	}
	if err = SaveProjects([]*Project{q, p}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(env.RootFile)
	data = bytes.ReplaceAll(data, []byte(`\`), []byte("/")) // Registrations use BYOND's backslashes.
	if bytes.Contains(data, []byte("/first.dm")) {
		t.Fatal("fleet save restored the old include")
	}
	for _, name := range []string{"/second.dm", "/explorer.dm"} {
		if !bytes.Contains(data, []byte(name)) {
			t.Fatalf("fleet save lost %s", name)
		}
	}
}

func TestRepeatedShipRenameCanReturnToSavedFilename(t *testing.T) {
	c, env := authorEnvironment(t)
	p, err := NewProject(c, env, "original", "Original", 16, 16)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Explorer", "Original", "Explorer", "Final Name"} {
		if err = p.renameShipAndFiles(name); err != nil {
			t.Fatalf("rename to %s: %v", name, err)
		}
	}
	if err = p.Save(); err != nil {
		t.Fatal(err)
	}
	if p.Modified() {
		t.Fatal("saved repeated rename remains dirty")
	}
	for _, name := range []string{"original", "explorer"} {
		if _, err = os.Stat(filepath.Join(c.Root, "_maps/voidcrew/ships/ship_"+name+".dmm")); !os.IsNotExist(err) {
			t.Fatal("intermediate name remains on disk")
		}
	}
}

// Preserve coverage of a mapper explicitly changing both labels and files.
func (p *Project) renameShipAndFiles(name string) (err error) {
	before := p.Capture()
	defer func() {
		if err != nil {
			p.Restore(before)
		}
	}()
	if err = p.RenameShip(name); err != nil {
		return err
	}
	return p.RenameShipFiles(fileName(name))
}
