package wsship

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/ship"
	"sdmm/internal/util"
)

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
	for n := 1; n <= 20; n++ {
		vars := dmvars.Set(original.Vars(), "pixel_w", strconv.Itoa(40+n))
		vars = dmvars.Set(vars, "pixel_z", strconv.Itoa(-40-n))
		instance.SetPrefab(dmmprefab.New(0, original.Path(), vars))
		editor.UpdateCanvasByCoords([]util.Point{instance.Coord()})
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
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "module-offset-preview.png"), 1400, 960)
	}
	final := instance.Prefab()
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
