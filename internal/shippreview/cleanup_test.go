package shippreview

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanupReviewAndApply(t *testing.T) {
	root, client := incrementalFixture(t)
	refreshPreviews(t, root, client, false, "alpha.dmm", "beta.dmm", "room.dmm", "room_blue.dmm")
	output := filepath.Join(root, "voidcrew/modules/ship_upgrades/previews")
	for _, name := range []string{"old.png", "edited.png", "new_ship.png", "unchecked.png", "notes.txt"} {
		write(t, filepath.Join(output, name), "keep a recovery copy: "+name)
	}
	plan, err := client.ScanCleanup(root)
	if err != nil || len(plan.Files) != 4 {
		t.Fatalf("review: %+v %v", plan, err)
	}
	for _, file := range plan.Files {
		if _, err := os.Stat(filepath.Join(output, file.Name)); err != nil {
			t.Fatal("review changed project files", err)
		}
	}
	selected := plan.Files[:0]
	for _, file := range plan.Files {
		if file.Name != "unchecked.png" {
			selected = append(selected, file)
		}
	}
	plan.Files = selected
	write(t, filepath.Join(output, "after-review.png"), "not approved")
	write(t, filepath.Join(output, "edited.png"), "edited after review")
	write(t, filepath.Join(root, "maps/new_ship.dmm"), "8 newly used")
	if err := client.RequestCleanup(root, filepath.Join(root, "selected.dme"), *plan); err != nil {
		t.Fatal(err)
	}
	until(t, func() bool { return client.Status(root).Phase == "complete" })
	if _, err := os.Stat(filepath.Join(output, "old.png")); !os.IsNotExist(err) {
		t.Fatal("approved unused file was not removed", err)
	}
	for _, name := range []string{"alpha.png", "beta.png", "room.png", "room_blue.png", "new_ship.png", "unchecked.png", "notes.txt", "after-review.png"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("protected file removed: %s %v", name, err)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(output, "edited.png")); string(data) != "edited after review" {
		t.Fatal("changed file was not preserved")
	}
	backups, _ := filepath.Glob(filepath.Join(client.CleanupBackups(root), "*", "old.png"))
	if len(backups) != 1 {
		t.Fatalf("missing recovery copy: %v", backups)
	}
	data, _ := os.ReadFile(backups[0])
	if string(data) != "keep a recovery copy: old.png" {
		t.Fatal("recovery copy is not identical")
	}
}

func TestCleanupFailureRetainsApprovalAndQueuedSaves(t *testing.T) {
	root, client := fixture(t)
	environment := filepath.Join(root, "selected.dme")
	client.Request(root, environment)
	until(t, func() bool { return client.Status(root).Phase == "complete" })
	output := filepath.Join(root, "voidcrew/modules/ship_upgrades/previews")
	write(t, filepath.Join(output, "old.png"), "recover me")
	plan, err := client.ScanCleanup(root)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "fail"), "")
	if err := client.RequestCleanup(root, environment, *plan); err != nil {
		t.Fatal(err)
	}
	until(t, func() bool { return client.Status(root).Phase == "failed" })
	if data, _ := os.ReadFile(filepath.Join(output, "old.png")); string(data) != "recover me" {
		t.Fatal("failed generation removed a preview")
	}
	if err := os.Remove(filepath.Join(root, "fail")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "hold"), "")
	t.Cleanup(func() { _ = os.Remove(filepath.Join(root, "hold")) })
	client.Request(root, environment)
	until(t, func() bool { data, _ := os.ReadFile(filepath.Join(root, "runs")); return string(data) == "3" })
	other := New(client.profile)
	write(t, filepath.Join(root, "input"), "queued save")
	other.Request(root, environment)
	until(t, func() bool { return other.Status(root).Queued })
	if err := os.Remove(filepath.Join(root, "hold")); err != nil {
		t.Fatal(err)
	}
	until(t, func() bool { return other.Status(root).Phase == "complete" })
	if _, err := os.Stat(filepath.Join(output, "old.png")); !os.IsNotExist(err) {
		t.Fatal("retry lost the reviewed cleanup", err)
	}
	if data, _ := os.ReadFile(filepath.Join(output, "ship_fixture.png")); string(data) != "queued save" {
		t.Fatal("cleanup lost the queued save")
	}
}

func TestAutomaticCleanupKeepsUnknownAndEditedImages(t *testing.T) {
	root, client := incrementalFixture(t)
	refreshPreviews(t, root, client, false, "alpha.dmm", "beta.dmm", "room.dmm", "room_blue.dmm")
	output := filepath.Join(root, "voidcrew/modules/ship_upgrades/previews")
	before, _ := os.ReadFile(filepath.Join(output, "room_blue.png"))
	write(t, filepath.Join(output, "unknown.png"), "untracked artwork")
	write(t, filepath.Join(output, "beta.png"), "manually edited artwork")
	for _, name := range []string{"beta", "room_blue"} {
		if err := os.Remove(filepath.Join(root, "maps", name+".dmm")); err != nil {
			t.Fatal(err)
		}
	}
	refreshPreviews(t, root, client, false)
	if _, err := os.Stat(filepath.Join(output, "room_blue.png")); !os.IsNotExist(err) {
		t.Fatal("obsolete generated preview remains", err)
	}
	for name, want := range map[string]string{"unknown.png": "untracked artwork", "beta.png": "manually edited artwork"} {
		if data, _ := os.ReadFile(filepath.Join(output, name)); string(data) != want {
			t.Fatal("automatic cleanup touched unknown/edited artwork", name)
		}
	}
	backups, _ := filepath.Glob(filepath.Join(client.CleanupBackups(root), "*", "room_blue.png"))
	if len(backups) != 1 {
		t.Fatal("missing automatic cleanup backup")
	}
	if data, _ := os.ReadFile(backups[0]); !bytes.Equal(data, before) {
		t.Fatal("backup content changed")
	}
}

// Exercise filesystem failures directly in the real Python worker, so a failed
// backup, partial removal, invalid manifest or link can never silently pass.
func TestCleanupFilesystemSafety(t *testing.T) {
	_, client := fixture(t)
	helper, err := workerPath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "cleanup_safety.py")
	write(t, script, cleanupSafetyTests)
	executable, args, environment, err := client.previewCommand()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, append(args, "-B", script, helper)...)
	cmd.Env = environment
	configureProcess(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("filesystem safety tests: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "OK") {
		t.Fatal(string(output))
	}
	t.Log(string(output))
}

const cleanupSafetyTests = `import importlib.util, json, os, shutil, subprocess, sys, tempfile, unittest
from pathlib import Path
from unittest.mock import patch
spec = importlib.util.spec_from_file_location("worker", sys.argv.pop())
w = importlib.util.module_from_spec(spec)
spec.loader.exec_module(w)

GENERATOR = '''import json
from pathlib import Path
REPO_ROOT = Path(__file__).resolve().parents[2]
OUTPUT_DIR = None
def main():
    manifest = json.loads((REPO_ROOT / 'next.json').read_text())
    (OUTPUT_DIR / 'current.png').write_bytes(b'new current')
    (OUTPUT_DIR / 'manifest.json').write_text(json.dumps(manifest))
'''

class Safety(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / 'project'
        self.root.mkdir()
        self.folder = Path(self.temp.name) / 'profile'
        self.folder.mkdir()
        self.output = self.root / w.PREVIEW_DIRECTORY
        self.output.mkdir(parents=True)
        self.manifest = {'hulls': {'current': {'png': 'current.png'}}, 'modules': {}}
        self.output.joinpath('manifest.json').write_text(json.dumps(self.manifest))
        for name in ('current.png', 'old.png', 'other.png'):
            self.output.joinpath(name).write_bytes(name.encode())
        script = self.root / 'tools/ship_previews/generate_ship_previews.py'
        script.parent.mkdir(parents=True)
        script.write_text(GENERATOR)
        self.root.joinpath('next.json').write_text(json.dumps(self.manifest))
        self.approved = {f['name']: f['sha256'] for f in w.cleanup_plan(self.root)['files']}
        self.before = {p.name: p.read_bytes() for p in self.output.iterdir()}

    def generate(self):
        w.generate(self.root, self.root / 'game.dme', self.folder, approved=self.approved)

    def unchanged(self):
        self.assertEqual(self.before, {p.name: p.read_bytes() for p in self.output.iterdir()})

    def test_failed_backup_never_removes_originals(self):
        original = shutil.copy2
        def fail(source, dest, *args, **kwargs):
            if Path(source).name == 'other.png':
                raise PermissionError('backup disk failure')
            return original(source, dest, *args, **kwargs)
        with patch.object(w.shutil, 'copy2', fail):
            with self.assertRaises(PermissionError):
                self.generate()
        self.unchanged()

    def test_partial_cleanup_rolls_back_and_retains_backups(self):
        original = Path.unlink
        def fail(path, *args, **kwargs):
            if path == self.output / 'other.png':
                raise PermissionError('locked file')
            return original(path, *args, **kwargs)
        with patch.object(Path, 'unlink', fail):
            with self.assertRaises(PermissionError):
                self.generate()
        self.unchanged()
        for name in self.approved:
            copies = list((self.folder / 'cleanup-backups').glob('*/' + name))
            self.assertEqual(len(copies), 1)
            self.assertEqual(copies[0].read_bytes(), self.before[name])

    def test_publish_failure_preserves_files(self):
        original = os.replace
        def fail(source, target):
            if Path(target) == self.output / 'manifest.json':
                raise PermissionError('manifest locked')
            return original(source, target)
        self.manifest['version'] = 2
        self.root.joinpath('next.json').write_text(json.dumps(self.manifest))
        with patch.object(w.os, 'replace', fail):
            with self.assertRaises(PermissionError):
                self.generate()
        self.unchanged()

    def test_missing_image_and_unsafe_manifest_fail_closed(self):
        for name in ('missing.png', '../outside.png', 'C:outside.png', 'folder/image.png', 'CURRENT.png'):
            self.root.joinpath('next.json').write_text(json.dumps({'hulls': {'x': {'png': name}, 'y': {'png': 'current.png'}}, 'modules': {}}))
            with self.assertRaises(RuntimeError):
                self.generate()
            self.unchanged()

    def test_empty_fleet_cannot_erase_previews(self):
        self.root.joinpath('next.json').write_text(json.dumps({'hulls': {}, 'modules': {}}))
        self.generate()
        for name in self.approved:
            self.assertEqual(self.output.joinpath(name).read_bytes(), self.before[name])

    def test_invalid_existing_manifest_disables_scan(self):
        for value in ('{', '[]', '{"hulls":{},"modules":{}}'):
            self.output.joinpath('manifest.json').write_text(value)
            with self.assertRaises((RuntimeError, ValueError)):
                w.cleanup_plan(self.root)
        for name in self.approved:
            self.assertEqual(self.output.joinpath(name).read_bytes(), self.before[name])

    def test_backup_inside_project_is_rejected(self):
        self.folder = self.root / 'profile'
        self.folder.mkdir()
        with self.assertRaises(RuntimeError):
            self.generate()
        self.unchanged()

    def test_explicit_selection_does_not_expand_automatically(self):
        previous = {'images': {name: {'png_sha256': digest} for name, digest in self.approved.items()}}
        selected = {'old.png': self.approved['old.png']}
        candidates = w.cleanup_candidates(self.output, {'current.png': 'current.png'}, previous,
                                          {name: name for name in self.approved}, selected)
        self.assertEqual(candidates, selected)

    def test_unrelated_files_subdirectories_and_paths_are_kept(self):
        self.output.joinpath('notes.txt').write_text('keep')
        self.output.joinpath('sub').mkdir()
        self.output.joinpath('sub/old.png').write_text('keep')
        outside = self.root / 'outside.png'
        outside.write_text('keep')
        self.approved['../../../../outside.png'] = w.file_hash(outside)
        self.generate()
        for path in (outside, self.output / 'notes.txt', self.output / 'sub/old.png'):
            self.assertEqual(path.read_text(), 'keep')

    def test_linked_directory_is_rejected(self):
        target = self.root / 'outside'
        self.output.rename(target)
        if os.name == 'nt':
            subprocess.run(['cmd', '/c', 'mklink', '/J', str(self.output), str(target)], check=True, capture_output=True, creationflags=subprocess.CREATE_NO_WINDOW)
        else:
            self.output.symlink_to(target, target_is_directory=True)
        with self.assertRaises(RuntimeError):
            w.cleanup_plan(self.root)
        with self.assertRaises(RuntimeError):
            self.generate()
        self.assertEqual(self.before, {p.name: p.read_bytes() for p in target.iterdir()})

    def test_linked_image_is_never_a_cleanup_candidate(self):
        target = self.root / 'outside.png'
        target.write_text('keep')
        try:
            self.output.joinpath('linked.png').symlink_to(target)
        except OSError:
            self.skipTest('File symlinks unavailable on this machine')
        self.assertNotIn('linked.png', {f['name'] for f in w.cleanup_plan(self.root)['files']})
        self.approved['linked.png'] = w.file_hash(target)
        self.generate()
        self.assertTrue(self.output.joinpath('linked.png').is_symlink())
        self.assertEqual(target.read_text(), 'keep')

    def use_split_generator(self):
        script = self.root / 'tools/ship_previews/generate_ship_previews.py'
        text = script.read_text().replace("(OUTPUT_DIR / 'manifest.json').write_text(json.dumps(manifest))",
            "(OUTPUT_DIR / 'hull.current.preview.json').write_text(json.dumps(dict(manifest, tile_px=32)))")
        script.write_text(text)

    def test_migration_cleanup_failure_restores_old_index_and_art(self):
        self.use_split_generator()
        original = Path.unlink
        def fail(path, *args, **kwargs):
            if path == self.output / 'manifest.json':
                raise PermissionError('old manifest locked')
            return original(path, *args, **kwargs)
        with patch.object(Path, 'unlink', fail):
            with self.assertRaises(PermissionError):
                self.generate()
        self.unchanged()

    def use_nested_generator(self):
        script = self.root / 'tools/ship_previews/generate_ship_previews.py'
        writer = '''for group, key, name in (("hulls", "current", "hulls/current.preview.json"),
                              ("modules", "current.dmm", "modules/test/workshop/current.preview.json")):
        document = {"tile_px": 32, "hulls": {}, "modules": {}}
        document[group][key] = manifest["hulls"]["current"]
        path = OUTPUT_DIR / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(document))'''
        script.write_text(GENERATOR.replace("(OUTPUT_DIR / 'manifest.json').write_text(json.dumps(manifest))", writer))

    def snapshot(self):
        return {p.relative_to(self.output).as_posix(): p.read_bytes()
                for p in self.output.rglob('*') if p.is_file()}

    def test_nested_publish_failure_restores_same_named_files(self):
        self.use_nested_generator()
        self.generate()
        before = self.snapshot()
        self.manifest['hulls']['current']['width'] = 64
        self.root.joinpath('next.json').write_text(json.dumps(self.manifest))
        original = os.replace
        def fail(source, target):
            if Path(target) == self.output / 'modules/test/workshop/current.preview.json':
                raise PermissionError('module metadata locked')
            return original(source, target)
        with patch.object(w.os, 'replace', fail):
            with self.assertRaises(PermissionError):
                self.generate()
        self.assertEqual(before, self.snapshot())

    def test_nested_removal_failure_restores_deleted_metadata(self):
        self.use_nested_generator()
        self.generate()
        before = self.snapshot()
        (self.root / 'tools/ship_previews/generate_ship_previews.py').write_text(GENERATOR)
        original = Path.unlink
        def fail(path, *args, **kwargs):
            if path == self.output / 'modules/test/workshop/current.preview.json':
                raise PermissionError('module metadata locked')
            return original(path, *args, **kwargs)
        with patch.object(Path, 'unlink', fail):
            with self.assertRaises(PermissionError):
                self.generate()
        self.assertEqual(before, self.snapshot())
        copies = list((self.folder / 'cleanup-backups').glob('*/hulls/current.preview.json'))
        self.assertEqual(len(copies), 1)
        self.assertEqual(copies[0].read_bytes(), before['hulls/current.preview.json'])

    def test_linked_metadata_directory_is_rejected(self):
        target = self.root / 'outside'
        target.mkdir()
        sentinel = target / 'keep.preview.json'
        sentinel.write_text('do not follow')
        link = self.output / 'modules'
        if os.name == 'nt':
            subprocess.run(['cmd', '/c', 'mklink', '/J', str(link), str(target)], check=True, capture_output=True, creationflags=subprocess.CREATE_NO_WINDOW)
        else:
            link.symlink_to(target, target_is_directory=True)
        with self.assertRaises(RuntimeError):
            w.read_manifest(self.output)
        self.use_nested_generator()
        with self.assertRaises(RuntimeError):
            self.generate()
        self.assertEqual(sentinel.read_text(), 'do not follow')
        self.assertEqual((self.output / 'current.png').read_bytes(), self.before['current.png'])

    def test_nested_paths_cannot_escape_output(self):
        for name in ('../outside.preview.json', '/outside.preview.json', 'C:/outside.preview.json',
                     'modules/../../outside.preview.json', 'modules\\outside.preview.json'):
            with self.subTest(name=name), self.assertRaises(RuntimeError):
                w.preview_file(self.output, name, create_parents=True)
        self.unchanged()

    def test_split_index_validates_duplicates_geometry_and_images(self):
        self.use_split_generator()
        self.generate()
        current = self.output / 'hull.current.preview.json'
        self.assertEqual(w.checked_manifest(self.output), {'current.png': 'current.png'})
        duplicate = self.output / 'duplicate.preview.json'
        duplicate.write_bytes(current.read_bytes())
        with self.assertRaisesRegex(RuntimeError, 'Duplicate'):
            w.checked_manifest(self.output)
        duplicate.unlink()
        document = json.loads(current.read_text())
        for value in (dict(document, tile_px=16), dict(document, hulls={}),
                      dict(document, hulls={'current': {'png': '../outside.png'}})):
            current.write_text(json.dumps(value))
            with self.assertRaises(RuntimeError):
                w.checked_manifest(self.output)

unittest.main(verbosity=2)
`
