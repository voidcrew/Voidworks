package shippreview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fakeGenerator = `import json, time
from pathlib import Path
REPO_ROOT = Path(__file__).resolve().parents[2]
OUTPUT_DIR = REPO_ROOT / "unused"
ENVIRONMENT = "wrong.dme"
def main():
    counter = REPO_ROOT / "runs"
    n = int(counter.read_text()) + 1 if counter.exists() else 1
    counter.write_text(str(n))
    source = (REPO_ROOT / "input").read_text()
    print("Generating fixture " + str(n), flush=True)
    deadline = time.monotonic() + 20
    while (REPO_ROOT / "hold").exists() and time.monotonic() < deadline:
        time.sleep(.05)
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    (OUTPUT_DIR / "ship_fixture.png").write_text(source)
    if (REPO_ROOT / "fail").exists():
        raise RuntimeError("fixture render failure")
    (OUTPUT_DIR / "manifest.json").write_text(json.dumps({"hulls": {"fixture": {"png": "ship_fixture.png", "source": source}}, "modules": {}, "environment": ENVIRONMENT}))
`

func fixture(t *testing.T) (string, *Client) {
	t.Helper()
	if _, _, err := python(); err != nil && !Bundled() {
		t.Skip("Python is needed for the preview worker integration tests")
	}
	root := filepath.Join(t.TempDir(), "Project with spaces & brackets [test]")
	if err := os.MkdirAll(filepath.Join(root, "tools/ship_previews"), 0700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, Script), fakeGenerator)
	write(t, filepath.Join(root, "input"), "first")
	write(t, filepath.Join(root, "selected.dme"), "")
	return root, New(t.TempDir())
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func until(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for preview worker")
}

func TestWorkerCoalescesSavesAcrossEditorsAndSurvivesClient(t *testing.T) {
	root, client := fixture(t)
	hold := filepath.Join(root, "hold")
	write(t, hold, "")
	t.Cleanup(func() { _ = os.Remove(hold) })
	client.Request(root, filepath.Join(root, "selected.dme"))
	until(t, func() bool { _, err := os.Stat(filepath.Join(root, "runs")); return err == nil })
	// Requests during a render must produce exactly one subsequent generation.
	write(t, filepath.Join(root, "input"), "latest")
	other := New(client.profile)
	other.Request(root, filepath.Join(root, "selected.dme"))
	until(t, func() bool { return other.Status(root).Queued })
	client = nil // The first editor need not remain open.
	if err := os.Remove(hold); err != nil {
		t.Fatal(err)
	}
	until(t, func() bool { return other.Status(root).Phase == "complete" })
	data, _ := os.ReadFile(filepath.Join(root, "runs"))
	if string(data) != "2" {
		t.Fatalf("expected two serial generations, got %s", data)
	}
	manifest, err := os.ReadFile(filepath.Join(root, "voidcrew/modules/ship_upgrades/previews/manifest.json"))
	if err != nil || !strings.Contains(string(manifest), "latest") || !strings.Contains(string(manifest), "selected.dme") {
		t.Fatalf("new save or selected environment lost: %s %v", manifest, err)
	}
}

func TestWorkerFailureKeepsOldPreviewsAndRetries(t *testing.T) {
	root, client := fixture(t)
	environment := filepath.Join(root, "selected.dme")
	client.Request(root, environment)
	until(t, func() bool { return client.Status(root).Phase == "complete" })
	output := filepath.Join(root, "voidcrew/modules/ship_upgrades/previews")
	before, _ := os.ReadFile(filepath.Join(output, "manifest.json"))
	write(t, filepath.Join(root, "input"), "changed")
	write(t, filepath.Join(root, "fail"), "")
	client.Request(root, environment)
	until(t, func() bool { return client.Status(root).Phase == "failed" })
	after, _ := os.ReadFile(filepath.Join(output, "manifest.json"))
	png, _ := os.ReadFile(filepath.Join(output, "ship_fixture.png"))
	if string(before) != string(after) || string(png) != "first" {
		t.Fatal("failed generation replaced published previews")
	}
	if err := os.Remove(filepath.Join(root, "fail")); err != nil {
		t.Fatal(err)
	}
	client.Request(root, environment)
	until(t, func() bool { return client.Status(root).Phase == "complete" })
	png, _ = os.ReadFile(filepath.Join(output, "ship_fixture.png"))
	if string(png) != "changed" {
		t.Fatal("retry did not publish the new image")
	}
}

func TestMissingGeneratorAndPython(t *testing.T) {
	client := New(t.TempDir())
	root := t.TempDir()
	client.Request(root, filepath.Join(root, "tgstation.dme"))
	until(t, func() bool { return client.Status(root).Phase == "error" })
	if !strings.Contains(client.Status(root).Message, "missing") {
		t.Fatal(client.Status(root))
	}
	root, client = fixture(t)
	t.Setenv("STRONGDMM_PYTHON", filepath.Join(root, "not-python"))
	client.Request(root, filepath.Join(root, "selected.dme"))
	if Bundled() {
		until(t, func() bool { return client.Status(root).Phase == "complete" })
	} else {
		until(t, func() bool { return client.Status(root).Phase == "error" })
	}
}

func TestShipMapPaths(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"_maps/voidcrew/ships/ship_test.dmm", "_maps/voidcrew/ship_modules/nested/room.dmm"} {
		if !IsShipMap(root, filepath.Join(root, path)) {
			t.Fatal(path)
		}
	}
	for _, path := range []string{"_maps/voidcrew/ships-old/a.dmm", "_maps/voidcrew/ships/a.dm", "../ships/a.dmm", "_maps/ruins/a.dmm"} {
		if IsShipMap(root, filepath.Join(root, path)) {
			t.Fatal(path)
		}
	}
}

func TestPreviewProgressDistinguishesScanningFromRendering(t *testing.T) {
	log := filepath.Join(t.TempDir(), "generation.log")
	write(t, log, "Preview progress: 0 rendered, 42 reused (ship.png).\nhull fixture: 24x24\n")
	if got := previewProgress(log); !strings.Contains(got, "0 rendered, 42 reused") {
		t.Fatal(got)
	}
	write(t, log, "Preview progress: 0 rendered, 42 reused (ship.png).\nRendering changed or missing preview: changed.png\n")
	if got := previewProgress(log); !strings.Contains(got, "changed.png") {
		t.Fatal(got)
	}
}
