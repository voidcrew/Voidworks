package wsruin

import (
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/ruin"
)

func TestPreviewOnlyForAReadableSelection(t *testing.T) {
	ws := &WsRuin{}
	if ws.previewVisible() {
		t.Fatal("previewing with nothing selected")
	}
	ws.selected = &ruin.Template{File: "ruin.dmm"}
	if !ws.previewVisible() {
		t.Fatal("selected ruin has no preview")
	}
	ws.viewOnPlanet = true
	if ws.previewVisible() {
		t.Fatal("two previews beside each other")
	}
	ws.viewOnPlanet, ws.creating = false, true
	if ws.previewVisible() {
		t.Fatal("previewing the previous ruin over the creation wizard")
	}
	ws.creating = false
	ws.selected.Problem = "outside the project"
	if ws.previewVisible() {
		t.Fatal("previewing a ruin with a known problem")
	}
}

// clickLibraryRow presses the mouse over the middle of the results list, where
// a row or the gap between two rows must be, and returns the frame's duration.
func clickLibraryRow(t *testing.T, ws *WsRuin, render func()) time.Duration {
	t.Helper()
	io := imgui.CurrentIO()
	defer io.SetMousePosition(imgui.Vec2{X: -1000, Y: -1000})
	for y := float32(450); y < 600; y += 8 {
		io.SetMousePosition(imgui.Vec2{X: 120, Y: y})
		render()
		io.SetMouseButtonDown(0, true)
		render()
		io.SetMouseButtonDown(0, false)
		start := time.Now()
		render()
		if ws.selected != nil {
			return time.Since(start)
		}
	}
	t.Fatal("clicking the library selected no ruin")
	return 0
}

func testLibraryPreview(t *testing.T, ws *WsRuin, render func(), capture func(string), setWidth func(float32)) {
	t.Helper()
	dme := ws.catalog.Dme
	// The parse started with the workshop, so this click may arrive before it ends.
	first := clickLibraryRow(t, ws, render)
	t.Logf("First click showed %s in %v", ws.selected.Name, first)
	if first > time.Second {
		t.Fatalf("the first click waited %v", first)
	}
	if ws.ruinPreview == nil || ws.previewError != "" {
		t.Fatal("clicked ruin has no preview", ws.previewError)
	}
	if ws.project == nil || ws.message != "" {
		t.Fatal("preview disturbed the ruin's properties", ws.message)
	}
	if _, env := ws.ruinPreview.SpriteContext(); env == dme {
		capture("ruin-preview-editor-sprites")
	}
	shown := *ws.selected
	deadline := time.Now().Add(2 * time.Minute)
	for {
		if _, env := ws.ruinPreview.SpriteContext(); env != dme {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("preview never moved to compiled appearances")
		}
		render()
		time.Sleep(10 * time.Millisecond)
	}
	if ws.selected.Type != shown.Type {
		t.Fatal("moving to compiled appearances changed the selection")
	}
	capture("ruin-preview")

	var clicks []time.Duration
	for i := 0; i < len(ws.catalog.Templates); i += 10 {
		item := ws.catalog.Templates[i]
		if item.Problem != "" {
			continue
		}
		start := time.Now()
		ws.selectRuin(item)
		render()
		clicks = append(clicks, time.Since(start))
		if ws.previewError != "" {
			t.Fatal(item.Name, ws.previewError)
		}
		if ws.previewFile != item.File {
			t.Fatalf("%s kept showing %s", item.Name, ws.previewFile)
		}
	}
	sort.Slice(clicks, func(i, j int) bool { return clicks[i] < clicks[j] })
	median, worst := clicks[len(clicks)/2], clicks[len(clicks)-1]
	t.Logf("Previewed %d ruins: median %v, worst %v", len(clicks), median, worst)
	if worst > time.Second {
		t.Fatalf("a ruin took %v to preview", worst)
	}

	setWidth(780)
	capture("ruin-preview-stacked")
	setWidth(1280)

	// Returning from a map tab shows that tab's unsaved edits.
	live, err := ws.loadPreviewRuin()
	if err != nil {
		t.Fatal(err)
	}
	live.Name = "Unsaved ruin edits"
	ws.LiveMap = func(string) *dmmap.Dmm { return live }
	ws.OnFocusChange(true)
	render()
	if scene, _ := ws.SpriteContext(); scene == nil || scene.Name != live.Name {
		t.Fatal("preview did not refresh from the open map")
	}
	ws.LiveMap = nil

	missing := *ws.selected
	missing.File = filepath.Join(t.TempDir(), "missing.dmm")
	ws.selectRuin(missing)
	capture("ruin-preview-unreadable")
	if ws.previewError == "" {
		t.Fatal("unreadable map reported nothing")
	}
	ws.selectRuin(shown)
	render()
	if ws.previewError != "" || ws.previewFile != shown.File {
		t.Fatal("preview did not recover from an unreadable map", ws.previewError)
	}
}
