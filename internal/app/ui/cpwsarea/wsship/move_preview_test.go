package wsship

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"sdmm/internal/app/render/bucket/level/chunk/unit"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/ship"
	"sdmm/internal/util"
)

type pixelPreviewProbe struct {
	pane   *pmap.PaneMap
	id     uint64
	bounds util.Bounds
	seen   bool
}

func (p *pixelPreviewProbe) ProcessUnit(u unit.Unit) bool {
	if u.Instance().Id() == p.id {
		p.bounds, p.seen = u.ViewBounds(), true
	}
	return p.pane.ProcessUnit(u)
}

// Exercise temporary offsets through the real module editor, assembled render
// and command history. The Move tool's mouse input is covered in tools tests.
func exerciseMovePreview(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	originalSource := ws.source
	defer func() { ws.source = originalSource; ws.rebuild() }()
	ws.source = 1
	ws.rebuild()
	ws.OnFocusChange(true)
	var instance *dmminstance.Instance
	for _, tile := range ws.pane.Dmm().Tiles {
		for _, i := range tile.Instances() {
			if strings.HasPrefix(i.Prefab().Path(), "/obj/") && !strings.HasPrefix(i.Prefab().Path(), "/obj/modular_map_") {
				instance = i
				break
			}
		}
		if instance != nil {
			break
		}
	}
	if instance == nil {
		t.Fatal("module lacks an object for offset preview")
	}
	editor := ws.pane.Editor()
	editor.FocusCamera(instance)
	before := map[string][]byte{}
	for file, doc := range ws.project.Documents {
		before[file] = ship.RawData(doc.Map).EncodeTGM()
	}
	file := ws.pane.Dmm().Path.Absolute
	original := instance.Prefab()
	count := len(dmmap.PrefabStorage.GetAllByPath(original.Path()))
	started := time.Now()
	var rebuildTime time.Duration
	for n := 1; n <= 20; n++ {
		vars := dmvars.Set(original.Vars(), "pixel_w", strconv.Itoa(40+n))
		vars = dmvars.Set(vars, "pixel_z", strconv.Itoa(-40-n))
		instance.SetPrefab(dmmprefab.New(0, original.Path(), vars))
		updateStart := time.Now()
		editor.UpdateCanvasByCoords([]util.Point{instance.Coord()})
		rebuildTime += time.Since(updateStart)
		render()
		if ws.invalid || len(dmmap.PrefabStorage.GetAllByPath(original.Path())) != count {
			t.Fatal("live module refresh persisted an intermediate offset or broke assembly", ws.message)
		}
		found := false
		for _, tile := range ws.pane.ViewDmm().Tiles {
			for _, i := range tile.Instances() {
				if i.Id() == instance.Id() {
					found = i.Prefab().Id() == instance.Prefab().Id()
				}
			}
		}
		if !found {
			t.Fatal("assembled view did not display the current module offset")
		}
	}
	t.Logf("20 native module offset previews: %s, zero intermediate prefabs", time.Since(started))
	final := instance.Prefab()
	instance.SetPrefab(original)
	editor.UpdateCanvasByCoords([]util.Point{instance.Coord()})
	assembly, view := ws.assembly, ws.pane.ViewDmm()
	probe := &pixelPreviewProbe{pane: ws.pane, id: instance.Id()}
	renderer := ws.pane.Canvas().Render()
	renderer.SetUnitProcessor(probe)
	defer renderer.SetUnitProcessor(ws.pane)
	render()
	if !probe.seen {
		t.Fatal("offset test object is not visible before dragging")
	}
	originalBounds := probe.bounds
	var previewTime time.Duration
	for n := 1; n <= 20; n++ {
		x, y := 40+n-original.Vars().IntV("pixel_w", 0), -40-n-original.Vars().IntV("pixel_z", 0)
		updateStart := time.Now()
		editor.PreviewPixelOffset(instance, x, y)
		previewTime += time.Since(updateStart)
		probe.seen = false
		render()
		if ws.assembly != assembly || ws.pane.ViewDmm() != view || instance.Prefab() != original {
			t.Fatal("pixel preview rebuilt the ship or changed source data before release")
		}
		if !probe.seen || probe.bounds != originalBounds.Plus(float32(x), float32(y)) {
			t.Fatalf("pixel preview bounds = %v, want offset (%d,%d) from %v", probe.bounds, x, y, originalBounds)
		}
	}
	t.Logf("20 module pixel updates: full rebuild %s; visual-only preview %s; zero assembly rebuilds", rebuildTime, previewTime)
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "module-offset-preview.png"), 1400, 960)
	}
	// The dragged sprite must remain visible even when its source chunk has
	// scrolled off screen; hit testing must use the translated bounds too.
	shift := renderer.Camera.ShiftX
	renderer.Camera.ShiftX -= 1200
	editor.PreviewPixelOffset(instance, 1200, 0)
	probe.seen = false
	render()
	renderer.Camera.ShiftX = shift
	if !probe.seen || probe.bounds != originalBounds.Plus(1200, 0) {
		t.Fatal("dragged sprite was culled with its offscreen source chunk")
	}
	editor.ClearPixelOffsetPreview()
	render()
	if probe.bounds != originalBounds {
		t.Fatal("clearing preview did not restore the original sprite position")
	}
	for name, doc := range ws.project.Documents {
		if !bytes.Equal(before[name], ship.RawData(doc.Map).EncodeTGM()) {
			t.Fatal("visual drag changed map data before release")
		}
	}
	_, existed := dmmap.PrefabStorage.GetById(final.Id())
	instance.SetPrefab(dmmap.PrefabStorage.Put(final))
	editor.InstanceSelect(instance)
	editor.CommitContextNow("Move module object")
	after := ship.RawData(ws.pane.Dmm()).EncodeTGM()
	if bytes.Equal(before[file], after) {
		t.Fatal("offset move was not recorded")
	}
	if !existed && len(dmmap.PrefabStorage.GetAllByPath(original.Path())) != count+1 {
		t.Fatal("release did not persist exactly one offset")
	}
	for name, doc := range ws.project.Documents {
		if name != file && !bytes.Equal(before[name], ship.RawData(doc.Map).EncodeTGM()) {
			t.Fatal("offset edit changed another source")
		}
	}
	ws.app.CommandStorage().Undo()
	if !bytes.Equal(before[file], ship.RawData(ws.project.Documents[file].Map).EncodeTGM()) {
		t.Fatal("offset undo did not restore the module")
	}
	ws.app.CommandStorage().Redo()
	if !bytes.Equal(after, ship.RawData(ws.project.Documents[file].Map).EncodeTGM()) {
		t.Fatal("offset redo did not restore the released position")
	}
	ws.app.CommandStorage().Undo()
}
