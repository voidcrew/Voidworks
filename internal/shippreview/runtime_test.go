package shippreview

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func runtimeArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestRuntimeInstallReuseAndRepair(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "Profile with spaces [test]")
	data := runtimeArchive(t, map[string]string{"python.exe": "python", "Lib/site-packages/PIL/image.py": "pillow"})
	var group sync.WaitGroup
	paths := make(chan string, 4)
	for i := 0; i < 4; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			path, err := prepareRuntime(profile, data)
			if err != nil {
				t.Error(err)
			}
			paths <- path
		}()
	}
	group.Wait()
	close(paths)
	first := <-paths
	for path := range paths {
		if path != first {
			t.Fatal("concurrent requests did not reuse one runtime")
		}
	}
	if err := os.Remove(filepath.Join(first, "Lib/site-packages/PIL/image.py")); err != nil {
		t.Fatal(err)
	}
	repaired, err := prepareRuntime(profile, data)
	if err != nil || repaired == first {
		t.Fatalf("missing files were not repaired independently of the old worker: %v", err)
	}
	again, err := prepareRuntime(profile, data)
	if err != nil || again != repaired {
		t.Fatalf("repaired runtime was not reused: %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(first, "python.exe")); err != nil || string(content) != "python" {
		t.Fatal("repair changed files in the existing runtime")
	}
}

func TestRuntimeRejectsUnsafeArchives(t *testing.T) {
	for _, name := range []string{"../outside", "/absolute", `C:/absolute`, `dir\escape`, "runtime-ready"} {
		if _, err := prepareRuntime(t.TempDir(), runtimeArchive(t, map[string]string{name: "bad"})); err == nil {
			t.Fatalf("accepted unsafe runtime path %s", name)
		}
	}
	if _, err := prepareRuntime(t.TempDir(), runtimeArchive(t, map[string]string{"Python.exe": "one", "python.exe": "two"})); err == nil {
		t.Fatal("accepted a case-insensitive filename collision")
	}
}

func withoutSystemTools(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("STRONGDMM_PYTHON", "missing-python.exe")
	t.Setenv("DMM_TOOLS", "missing-renderer.exe")
	t.Setenv("PYTHONHOME", "missing-python-home")
	t.Setenv("PYTHONPATH", "missing-python-packages")
}

func TestBundledPreviewWithoutSystemTools(t *testing.T) {
	if !Bundled() {
		t.Skip("requires -tags bundled_previews")
	}
	withoutSystemTools(t)
	root, client := fixture(t)
	write(t, filepath.Join(root, Script), `import json, os, subprocess
from pathlib import Path
from PIL import Image
OUTPUT_DIR = Path("wrong-output")
ENVIRONMENT = "wrong-environment"
def main():
    result = subprocess.run([os.environ["DMM_TOOLS"], "--version"], capture_output=True, text=True, check=True)
    assert "dmm-tools" in result.stdout
    assert Image.__version__ == "12.3.0"
    Image.new("RGBA", (32, 32), (19, 88, 120, 255)).save(OUTPUT_DIR / "fixture.png")
    (OUTPUT_DIR / "manifest.json").write_text(json.dumps({"hulls": {"fixture": {"png": "fixture.png"}}, "modules": {}}))
`)
	client.Request(root, filepath.Join(root, "selected.dme"))
	until(t, func() bool {
		status := client.Status(root)
		if status.Phase == "failed" || status.Phase == "error" {
			log, _ := os.ReadFile(status.Log)
			t.Fatalf("bundled preview failed: %s\n%s", status.Message, log)
		}
		return status.Phase == "complete"
	})
	file, err := os.Open(filepath.Join(root, "voidcrew/modules/ship_upgrades/previews/fixture.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	image, err := png.Decode(file)
	if err != nil || image.Bounds().Dx() != 32 || image.Bounds().Dy() != 32 {
		t.Fatalf("bundled Pillow did not produce a valid preview: %v", err)
	}
}

// Render actual ship art through the maintained game script without writing to
// the game checkout. The worker publishes only into its isolated test fixture.
func TestBundledGamePreview(t *testing.T) {
	dme := os.Getenv("SHIP_PREVIEW_TEST_DME")
	if !Bundled() || dme == "" {
		t.Skip("requires bundled_previews and SHIP_PREVIEW_TEST_DME")
	}
	withoutSystemTools(t)
	root, client := fixture(t)
	gameRoot := filepath.Dir(dme)
	source := filepath.Join(root, "ship_fixture.dmm")
	mapData, err := os.ReadFile(filepath.Join(gameRoot, "_maps/voidcrew/ships/ship_delta_a.dmm"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, source, string(mapData))
	script := fmt.Sprintf(`import hashlib, json, runpy, tempfile
from pathlib import Path
game = runpy.run_path(%q)
Dmm = game["Dmm"]
render = game["render"]
OUTPUT_DIR = Path("wrong-output")
ENVIRONMENT = "wrong-environment"
def main():
    game["render"].__globals__["ENVIRONMENT"] = ENVIRONMENT
    source = Path(%q)
    renderer = game["find_dmm_tools"]()
    with tempfile.TemporaryDirectory() as temp:
        render(renderer, source, OUTPUT_DIR / "ship_delta.png", Path(temp), Dmm(source))
    (OUTPUT_DIR / "manifest.json").write_text(json.dumps({"hulls": {"delta": {"png": "ship_delta.png", "src_md5": hashlib.md5(source.read_bytes()).hexdigest()}}, "modules": {}}))
	`, filepath.ToSlash(filepath.Join(gameRoot, Script)), filepath.ToSlash(source))
	write(t, filepath.Join(root, Script), script)
	client.Request(root, dme)
	until(t, func() bool {
		status := client.Status(root)
		if status.Phase == "failed" || status.Phase == "error" {
			log, _ := os.ReadFile(status.Log)
			t.Fatalf("game preview failed: %s\n%s", status.Message, log)
		}
		return status.Phase == "complete"
	})
	output := filepath.Join(root, "voidcrew/modules/ship_upgrades/previews/ship_delta.png")
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := png.Decode(bytes.NewReader(data))
	if err != nil || preview.Bounds().Dx() < 64 {
		t.Fatalf("invalid ship preview: %v", err)
	}
	if destination := os.Getenv("SHIP_PREVIEW_TEST_OUTPUT"); strings.TrimSpace(destination) != "" {
		if err := os.WriteFile(destination, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	client.Request(root, dme)
	until(t, func() bool {
		status := client.Status(root)
		if status.Phase == "failed" || status.Phase == "error" {
			log, _ := os.ReadFile(status.Log)
			t.Fatalf("incremental real render failed: %s\n%s", status.Message, log)
		}
		return status.Phase == "complete"
	})
	if !strings.Contains(client.Status(root).Message, "0 rendered, 1 reused") {
		t.Fatal("the unchanged real ship was rendered again: " + client.Status(root).Message)
	}
	again, _ := os.ReadFile(output)
	if !bytes.Equal(data, again) {
		t.Fatal("reusing a real ship changed its preview")
	}
}
