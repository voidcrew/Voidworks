package shippreview

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

const incrementalGenerator = `import hashlib, json, time, subprocess, sys
from pathlib import Path
from PIL import Image
REPO_ROOT = Path(__file__).resolve().parents[2]
OUTPUT_DIR = REPO_ROOT / "unused"
ENVIRONMENT = "wrong.dme"

class Dmm:
    def __init__(self, path):
        self.data = Path(path).read_text()
        self.width = int(self.data.split()[0])
        self.height = 8

def source_md5(path):
    return hashlib.md5(path.read_bytes()).hexdigest()

def render(tool, path, output, temp, dmm):
    calls = REPO_ROOT / "renders"
    with calls.open("a") as f:
        f.write(path.name + "\n")
    if (REPO_ROOT / "hold-child").exists() and path.name == "beta.dmm":
        subprocess.run([sys.executable, "-c", "import os,time; from pathlib import Path; Path('child-pid').write_text(str(os.getpid())); time.sleep(20)"], cwd=REPO_ROOT)
    deadline = time.monotonic() + 20
    while (REPO_ROOT / "hold").exists() and time.monotonic() < deadline:
        time.sleep(.02)
    color = tuple(hashlib.sha256(dmm.data.encode()).digest()[:3])
    Image.new("RGB", (dmm.width, dmm.height), color).save(output)
    if (REPO_ROOT / "fail").exists() and path.name == "room.dmm":
        raise RuntimeError("room renderer failed")

def main():
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    manifest = {"hulls": {}, "modules": {"room.dmm": {"themes": {}}}}
    for path in sorted((REPO_ROOT / "maps").glob("*.dmm")):
        dmm = Dmm(path)
        name = path.stem + ".png"
        render(REPO_ROOT / "renderer", path, OUTPUT_DIR / name, None, dmm)
        entry = {"png": name, "width": dmm.width, "height": dmm.height, "src_md5": source_md5(path)}
        if path.stem == "room":
            manifest["modules"]["room.dmm"].update(entry)
        elif path.stem == "room_blue":
            manifest["modules"]["room.dmm"]["themes"]["blue"] = entry
        else:
            manifest["hulls"][path.stem] = entry
    (OUTPUT_DIR / "manifest.json").write_text(json.dumps(manifest))
`

func incrementalFixture(t *testing.T) (string, *Client) {
	t.Helper()
	root, client := fixture(t)
	python, args, env, err := client.previewCommand()
	if err != nil {
		t.Fatal(err)
	}
	probe := exec.Command(python, append(args, "-c", "from PIL import Image")...)
	probe.Env = env
	if err := probe.Run(); err != nil {
		t.Skip("Pillow is required for incremental preview tests")
	}
	if err := os.Mkdir(filepath.Join(root, "maps"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "beta", "room", "room_blue"} {
		write(t, filepath.Join(root, "maps", name+".dmm"), "8 "+name)
	}
	write(t, filepath.Join(root, Script), incrementalGenerator)
	write(t, filepath.Join(root, "renderer"), "fixture renderer")
	return root, client
}

func refreshPreviews(t *testing.T, root string, client *Client, full bool, want ...string) {
	t.Helper()
	write(t, filepath.Join(root, "renders"), "")
	if full {
		client.RequestFull(root, filepath.Join(root, "selected.dme"))
	} else {
		client.Request(root, filepath.Join(root, "selected.dme"))
	}
	until(t, func() bool {
		status := client.Status(root)
		if status.Phase == "failed" || status.Phase == "error" {
			log, _ := os.ReadFile(status.Log)
			t.Fatalf("preview generation failed: %s\n%s", status.Message, log)
		}
		return status.Phase == "complete"
	})
	data, err := os.ReadFile(filepath.Join(root, "renders"))
	if err != nil || !slices.Equal(strings.Fields(string(data)), want) {
		t.Fatalf("rendered %q, expected %q: %v", data, want, err)
	}
}

func TestIncrementalPreviewsRenderOnlyChangedOrMissingMaps(t *testing.T) {
	root, client := incrementalFixture(t)
	all := []string{"alpha.dmm", "beta.dmm", "room.dmm", "room_blue.dmm"}
	refreshPreviews(t, root, client, false, all...)
	output := filepath.Join(root, "voidcrew/modules/ship_upgrades/previews")
	old := time.Unix(1234567890, 0)
	for _, name := range []string{"alpha.png", "beta.png", "room.png", "room_blue.png", "manifest.json"} {
		if err := os.Chtimes(filepath.Join(output, name), old, old); err != nil {
			t.Fatal(err)
		}
	}
	refreshPreviews(t, root, client, false)
	for _, name := range []string{"alpha.png", "beta.png", "room.png", "room_blue.png", "manifest.json"} {
		info, err := os.Stat(filepath.Join(output, name))
		if err != nil || !info.ModTime().Equal(old) {
			t.Fatalf("unchanged preview was rewritten: %s (%v)", name, err)
		}
	}
	// Existing committed hashes also work before this editor has a local cache.
	if err := os.Remove(filepath.Join(client.folder(root), "render-cache.json")); err != nil {
		t.Fatal(err)
	}
	refreshPreviews(t, root, client, false)
	write(t, filepath.Join(root, "maps/room.dmm"), "12 expanded room")
	refreshPreviews(t, root, client, false, "room.dmm")
	write(t, filepath.Join(root, "maps/room_blue.dmm"), "10 blue variant")
	refreshPreviews(t, root, client, false, "room_blue.dmm")
	write(t, filepath.Join(root, "maps/beta.dmm"), "16 expanded hull")
	refreshPreviews(t, root, client, false, "beta.dmm")
	if err := os.Remove(filepath.Join(output, "alpha.png")); err != nil {
		t.Fatal(err)
	}
	refreshPreviews(t, root, client, false) // Restore the verified checkpoint.
	write(t, filepath.Join(output, "room.png"), "damaged image")
	refreshPreviews(t, root, client, false) // Restore damaged output without rendering.
	if err := os.Remove(filepath.Join(root, "maps/room_blue.dmm")); err != nil {
		t.Fatal(err)
	}
	refreshPreviews(t, root, client, false)
	manifest, _ := os.ReadFile(filepath.Join(output, "manifest.json"))
	if bytes.Contains(manifest, []byte("room_blue")) || !bytes.Contains(manifest, []byte(`"width": 16`)) {
		t.Fatalf("manifest did not update removed variants or changed geometry: %s", manifest)
	}
	write(t, filepath.Join(root, "maps/new_ship.dmm"), "14 new hull")
	refreshPreviews(t, root, client, false, "new_ship.dmm")
	refreshPreviews(t, root, client, true, "alpha.dmm", "beta.dmm", "new_ship.dmm", "room.dmm")
	write(t, filepath.Join(root, "renderer"), "new renderer version")
	refreshPreviews(t, root, client, false, "alpha.dmm", "beta.dmm", "new_ship.dmm", "room.dmm")
}

func TestIncrementalFailurePreservesImagesManifestAndCache(t *testing.T) {
	root, client := incrementalFixture(t)
	refreshPreviews(t, root, client, false, "alpha.dmm", "beta.dmm", "room.dmm", "room_blue.dmm")
	output := filepath.Join(root, "voidcrew/modules/ship_upgrades/previews")
	before := map[string][]byte{}
	for _, path := range []string{filepath.Join(output, "manifest.json"), filepath.Join(output, "alpha.png"), filepath.Join(output, "room.png"), filepath.Join(client.folder(root), "render-cache.json")} {
		before[path], _ = os.ReadFile(path)
	}
	write(t, filepath.Join(root, "maps/alpha.dmm"), "12 changed hull")
	write(t, filepath.Join(root, "maps/room.dmm"), "12 changed room")
	write(t, filepath.Join(root, "fail"), "")
	client.Request(root, filepath.Join(root, "selected.dme"))
	until(t, func() bool { return client.Status(root).Phase == "failed" })
	for path, data := range before {
		after, _ := os.ReadFile(path)
		if !bytes.Equal(data, after) {
			t.Fatalf("failed refresh changed %s", path)
		}
	}
	if err := os.Remove(filepath.Join(root, "fail")); err != nil {
		t.Fatal(err)
	}
	refreshPreviews(t, root, client, false, "room.dmm") // alpha completed before the failure.
	// Retrying a failed full rebuild must not silently fall back to cached art.
	write(t, filepath.Join(root, "fail"), "")
	client.RequestFull(root, filepath.Join(root, "selected.dme"))
	until(t, func() bool { return client.Status(root).Phase == "failed" })
	if err := os.Remove(filepath.Join(root, "fail")); err != nil {
		t.Fatal(err)
	}
	refreshPreviews(t, root, client, false, "room.dmm", "room_blue.dmm")
}

func TestIncrementalLineEndingsAndPartialCache(t *testing.T) {
	root, client := incrementalFixture(t)
	for _, name := range []string{"alpha", "beta", "room", "room_blue"} {
		write(t, filepath.Join(root, "maps", name+".dmm"), "8 "+name+"\n")
	}
	refreshPreviews(t, root, client, false, "alpha.dmm", "beta.dmm", "room.dmm", "room_blue.dmm")
	// A Windows checkout changes bytes, not map contents. Test both existing
	// local records and adoption of committed previews without a local cache.
	for _, name := range []string{"alpha", "beta", "room", "room_blue"} {
		write(t, filepath.Join(root, "maps", name+".dmm"), "8 "+name+"\r\n")
	}
	refreshPreviews(t, root, client, false)
	cachePath := filepath.Join(client.folder(root), "render-cache.json")
	data, _ := os.ReadFile(cachePath)
	var cache map[string]any
	if err := json.Unmarshal(data, &cache); err != nil {
		t.Fatal(err)
	}
	// Existing releases stored absolute paths and raw CRLF hashes. Migrating
	// those records must not itself trigger a full rebuild.
	normalize := func(path string) string {
		if runtime.GOOS == "windows" {
			return strings.ToLower(path)
		}
		return path
	}
	cache["context"].(map[string]any)["environment"] = normalize(filepath.Join(root, "selected.dme"))
	for _, value := range cache["images"].(map[string]any) {
		record := value.(map[string]any)
		path := filepath.Join(root, record["source"].(string))
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		record["source"] = normalize(path)
		record["src_md5"] = fmt.Sprintf("%x", md5.Sum(raw))
	}
	delete(cache["images"].(map[string]any), "beta.png")
	data, _ = json.Marshal(cache)
	write(t, cachePath, string(data))
	if err := os.RemoveAll(filepath.Join(client.folder(root), "render-progress")); err != nil {
		t.Fatal(err)
	}
	refreshPreviews(t, root, client, false)
	// A new checkout/profile should adopt the committed previews too.
	copyRoot := filepath.Join(t.TempDir(), "relocated")
	if err := os.CopyFS(copyRoot, os.DirFS(root)); err != nil {
		t.Fatal(err)
	}
	refreshPreviews(t, copyRoot, New(client.profile), false)
	write(t, filepath.Join(copyRoot, "maps/beta.dmm"), "12 actually changed\r\n")
	refreshPreviews(t, copyRoot, New(client.profile), false, "beta.dmm")
}

func TestIncrementalQueuedSaveUsesLatestMap(t *testing.T) {
	root, client := incrementalFixture(t)
	refreshPreviews(t, root, client, false, "alpha.dmm", "beta.dmm", "room.dmm", "room_blue.dmm")
	hold := filepath.Join(root, "hold")
	write(t, hold, "")
	t.Cleanup(func() { _ = os.Remove(hold) })
	write(t, filepath.Join(root, "renders"), "")
	write(t, filepath.Join(root, "maps/alpha.dmm"), "12 earlier save")
	client.Request(root, filepath.Join(root, "selected.dme"))
	until(t, func() bool { data, _ := os.ReadFile(filepath.Join(root, "renders")); return len(data) > 0 })
	write(t, filepath.Join(root, "maps/alpha.dmm"), "16 latest save")
	client.Request(root, filepath.Join(root, "selected.dme"))
	until(t, func() bool { return client.Status(root).Queued })
	if err := os.Remove(hold); err != nil {
		t.Fatal(err)
	}
	until(t, func() bool { return client.Status(root).Phase == "complete" })
	data, _ := os.ReadFile(filepath.Join(root, "renders"))
	if !slices.Equal(strings.Fields(string(data)), []string{"alpha.dmm", "alpha.dmm"}) {
		t.Fatalf("queued save rerendered unrelated maps: %s", data)
	}
	var manifest struct {
		Hulls map[string]struct{ Width int }
	}
	data, _ = os.ReadFile(filepath.Join(root, "voidcrew/modules/ship_upgrades/previews/manifest.json"))
	if err := json.Unmarshal(data, &manifest); err != nil || manifest.Hulls["alpha"].Width != 16 {
		t.Fatalf("queued save published stale geometry: %s %v", data, err)
	}
	refreshPreviews(t, root, client, false)
}
