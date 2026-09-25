package shippreview

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func waitForPreviews(t *testing.T, root string, client *Client, request func(), want ...string) Status {
	t.Helper()
	write(t, filepath.Join(root, "renders"), "")
	request()
	var status Status
	until(t, func() bool {
		status = client.Status(root)
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
	return status
}

func TestSavesKeepOutdatedPreviewsOfOtherMaps(t *testing.T) {
	root, client := incrementalFixture(t)
	ships := filepath.Join(root, "_maps/voidcrew/ships")
	if err := os.MkdirAll(ships, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "beta", "room", "room_blue"} {
		if err := os.Rename(filepath.Join(root, "maps", name+".dmm"), filepath.Join(ships, name+".dmm")); err != nil {
			t.Fatal(err)
		}
	}
	generator := strings.Replace(incrementalGenerator, `(REPO_ROOT / "maps")`, `(REPO_ROOT / "_maps/voidcrew/ships")`, 1)
	generator = strings.Replace(generator, `    (OUTPUT_DIR / "manifest.json").write_text(json.dumps(manifest))`, nestedMetadataWriter, 1)
	write(t, filepath.Join(root, Script), generator)
	environment := filepath.Join(root, "selected.dme")
	all := func() { client.Request(root, environment) }
	waitForPreviews(t, root, client, all, "alpha.dmm", "beta.dmm", "room.dmm", "room_blue.dmm")

	output := filepath.Join(root, "voidcrew/modules/ship_upgrades/previews")
	committed := map[string][]byte{}
	for _, name := range []string{"beta.png", "hulls/beta.preview.json", "room_blue.png", "modules/test/workshop/room.preview.json"} {
		committed[name], _ = os.ReadFile(filepath.Join(output, name))
	}
	// Someone else changed these maps without committing new previews.
	write(t, filepath.Join(ships, "beta.dmm"), "12 changed upstream")
	write(t, filepath.Join(ships, "room_blue.dmm"), "12 changed upstream")

	alpha := filepath.Join(ships, "alpha.dmm")
	write(t, alpha, "10 my ship")
	save := func(paths ...string) func() {
		return func() { client.RequestSaved(root, environment, paths...) }
	}
	status := waitForPreviews(t, root, client, save(alpha, filepath.Join(root, "code/ship.dm")), "alpha.dmm")
	for _, name := range []string{"beta.png", "hulls/beta.preview.json", "room_blue.png"} {
		if data, _ := os.ReadFile(filepath.Join(output, name)); !bytes.Equal(data, committed[name]) {
			t.Fatalf("saving another ship changed %s", name)
		}
	}
	if status.Outdated != 2 || !strings.Contains(status.Message, "2 previews of other maps are outdated") {
		t.Fatalf("outdated previews were not reported: %+v", status)
	}
	// Saving room refreshes room but keeps its blue variant's committed hash,
	// so the game's preview test still flags that image as outdated.
	room := filepath.Join(ships, "room.dmm")
	write(t, room, "10 my room")
	waitForPreviews(t, root, client, save(room), "room.dmm")
	var shard struct {
		Modules map[string]struct {
			SrcMD5 string `json:"src_md5"`
			Themes map[string]struct {
				SrcMD5 string `json:"src_md5"`
			}
		}
	}
	data, _ := os.ReadFile(filepath.Join(output, "modules/test/workshop/room.preview.json"))
	if err := json.Unmarshal(data, &shard); err != nil {
		t.Fatal(err)
	}
	var old struct {
		Modules map[string]struct {
			Themes map[string]struct {
				SrcMD5 string `json:"src_md5"`
			}
		}
	}
	if err := json.Unmarshal(committed["modules/test/workshop/room.preview.json"], &old); err != nil {
		t.Fatal(err)
	}
	if shard.Modules["room.dmm"].Themes["blue"].SrcMD5 != old.Modules["room.dmm"].Themes["blue"].SrcMD5 {
		t.Fatalf("kept variant was marked fresh: %s", data)
	}
	// A save that changes no maps still leaves the outdated previews alone.
	waitForPreviews(t, root, client, save())
	// The explicit command brings everything up to date.
	status = waitForPreviews(t, root, client, func() { client.RefreshOutdated(root, environment) }, "beta.dmm", "room_blue.dmm")
	if status.Outdated != 0 {
		t.Fatalf("refresh left outdated previews: %+v", status)
	}
	if data, _ := os.ReadFile(filepath.Join(output, "hulls/beta.preview.json")); bytes.Equal(data, committed["hulls/beta.preview.json"]) {
		t.Fatal("refresh did not update outdated metadata")
	}
	waitForPreviews(t, root, client, save(alpha))
}

func TestStoppedSaveKeepsItsMapsForResume(t *testing.T) {
	root, client := incrementalFixture(t)
	ships := filepath.Join(root, "_maps/voidcrew/ships")
	if err := os.MkdirAll(ships, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "beta", "room", "room_blue"} {
		if err := os.Rename(filepath.Join(root, "maps", name+".dmm"), filepath.Join(ships, name+".dmm")); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(root, Script), strings.Replace(incrementalGenerator, `(REPO_ROOT / "maps")`, `(REPO_ROOT / "_maps/voidcrew/ships")`, 1))
	environment := filepath.Join(root, "selected.dme")
	waitForPreviews(t, root, client, func() { client.Request(root, environment) }, "alpha.dmm", "beta.dmm", "room.dmm", "room_blue.dmm")
	hold := filepath.Join(root, "hold")
	write(t, hold, "")
	t.Cleanup(func() { _ = os.Remove(hold) })
	beta := filepath.Join(ships, "beta.dmm")
	write(t, beta, "12 saved then stopped")
	write(t, filepath.Join(root, "renders"), "")
	client.RequestSaved(root, environment, beta)
	until(t, func() bool { data, _ := os.ReadFile(filepath.Join(root, "renders")); return len(data) > 0 })
	client.Stop(root, environment)
	until(t, func() bool { return client.Status(root).Phase == "stopped" })
	if err := os.Remove(hold); err != nil {
		t.Fatal(err)
	}
	waitForPreviews(t, root, client, func() { client.Resume(root, environment) }, "beta.dmm")
}
