package shippreview

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the actual cache and publication paths with Cloud Files metadata.
// The files and images are real; only Windows' lstat attributes are simulated.
func TestCloudPreviewReuse(t *testing.T) {
	root, client := incrementalFixture(t)
	refreshPreviews(t, root, client, false, "alpha.dmm", "beta.dmm", "room.dmm", "room_blue.dmm")
	helper, err := workerPath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "cloud_previews.py")
	write(t, script, cloudPreviewTests)
	executable, args, environment, err := client.previewCommand()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, append(args, "-B", script, helper, root)...)
	cmd.Env = environment
	configureProcess(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "OK") {
		t.Fatalf("cloud preview tests: %v\n%s", err, output)
	}
	t.Log(string(output))
}

const cloudPreviewTests = `import contextlib, importlib.util, io, shutil, stat, sys, tempfile, unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch
fixture = Path(sys.argv.pop())
spec = importlib.util.spec_from_file_location('worker', sys.argv.pop())
w = importlib.util.module_from_spec(spec)
spec.loader.exec_module(w)

class CloudStat:
    def __init__(self, original, tag):
        self.original = original
        self.st_file_attributes = getattr(original, 'st_file_attributes', 0) | 0x400
        self.st_reparse_tag = tag

    def __getattr__(self, name):
        return getattr(self.original, name)

class CloudPreviews(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / 'project'
        shutil.copytree(fixture, self.root)
        self.folder = Path(self.temp.name) / 'profile'
        self.folder.mkdir()
        self.output = self.root / w.PREVIEW_DIRECTORY

    def cloud(self, directories=False):
        original = Path.lstat
        def metadata(path, *args, **kwargs):
            info = original(path, *args, **kwargs)
            if path.is_relative_to(self.output.parent) and (directories or stat.S_ISREG(info.st_mode)):
                return CloudStat(info, 0x9000001A)
            return info
        return patch.object(Path, 'lstat', metadata)

    def generate(self, expected, force=False):
        (self.root / 'renders').write_text('')
        before = {p.name: (p.read_bytes(), p.stat().st_mtime_ns) for p in self.output.glob('*.png')}
        log = io.StringIO()
        with contextlib.redirect_stdout(log):
            w.generate(self.root, self.root / 'selected.dme', self.folder, force=force)
        self.assertEqual((self.root / 'renders').read_text().splitlines(), expected)
        for name, signature in before.items():
            if Path(name).with_suffix('.dmm').name not in expected:
                image = self.output / name
                self.assertEqual((image.read_bytes(), image.stat().st_mtime_ns), signature)
        return log.getvalue()

    def test_existing_cloud_images_are_reused_without_local_cache(self):
        with self.cloud():
            self.generate([])

    def test_cloud_directories_are_usable(self):
        with self.cloud(directories=True):
            self.generate([])

    def test_changed_map_only_then_unchanged_save(self):
        with self.cloud():
            self.generate([])
            (self.root / 'maps/beta.dmm').write_text('12 changed beta')
            self.generate(['beta.dmm'])
            self.generate([])

    def test_log_explains_rebuild_decisions(self):
        self.assertIn('Preview refresh: incremental;', self.generate([]))
        (self.root / 'maps/beta.dmm').write_text('12 changed beta')
        self.assertIn('beta.png (map content or source path changed)', self.generate(['beta.dmm']))
        shutil.rmtree(self.folder / 'render-progress')
        (self.output / 'beta.png').unlink()
        self.assertIn('beta.png (preview file missing)', self.generate(['beta.dmm']))
        shutil.rmtree(self.folder / 'render-progress')
        (self.output / 'beta.png').write_bytes(b'damaged image')
        self.assertIn('beta.png (preview image changed on disk)', self.generate(['beta.dmm']))
        all_maps = ['alpha.dmm', 'beta.dmm', 'room.dmm', 'room_blue.dmm']
        log = self.generate(all_maps, force=True)
        self.assertIn('Preview refresh: full rebuild requested.', log)
        self.assertEqual(log.count('(full rebuild requested)'), 4)
        (self.root / 'renderer').write_text('changed renderer')
        log = self.generate([])
        self.assertIn('Use Rebuild all previews to apply rendering changes', log)

    def test_cloud_tags_do_not_allow_links_or_unknown_reparse_points(self):
        for tag in [0x9000001A | (n << 12) for n in range(16)]:
            with self.subTest(tag=hex(tag)):
                info = SimpleNamespace(st_mode=stat.S_IFREG, st_file_attributes=0x400, st_reparse_tag=tag)
                with patch.object(Path, 'lstat', return_value=info):
                    self.assertFalse(w.linked(Path('cloud.png')))
        for tag in (0, 0xA0000003, 0xA000000C, 0xA000001D, 0x8000001B, 0x9001001A):
            with self.subTest(tag=hex(tag)):
                info = SimpleNamespace(st_mode=stat.S_IFREG, st_file_attributes=0x400, st_reparse_tag=tag)
                with patch.object(Path, 'lstat', return_value=info):
                    self.assertTrue(w.linked(Path('linked.png')))
        for info in (SimpleNamespace(st_mode=stat.S_IFLNK),
                     SimpleNamespace(st_mode=stat.S_IFREG, st_file_attributes=0x400)):
            with patch.object(Path, 'lstat', return_value=info):
                self.assertTrue(w.linked(Path('linked.png')))

unittest.main(verbosity=2)
`
