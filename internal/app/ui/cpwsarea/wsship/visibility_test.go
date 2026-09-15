package wsship

import (
	"bytes"
	"sdmm/internal/app/render/bucket/level/chunk/unit"
	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmmap"
	"strings"
	"testing"
)

func TestWorkshopAllowsUnhideUnderHiddenCategory(t *testing.T) {
	const table = "/obj/structure/table"
	filter := dm.NewPathsFilter(func(path string) []string {
		switch path {
		case "/obj":
			return []string{"/obj/structure"}
		case "/obj/structure":
			return []string{table, "/obj/structure/chair"}
		}
		return nil
	})
	ws := &WsShip{app: &previewApp{filter: filter}}
	filter.TogglePath("/obj")
	filter.TogglePath(table)
	if filter.IsHiddenPath(table) {
		t.Fatal("unhide did not update the filter")
	}
	if !ws.visible(table) {
		t.Fatal("workshop overrides an individually unhidden type")
	}
	if !filter.IsHiddenPath("/obj/structure/chair") {
		t.Fatal("unhide also revealed unrelated objects")
	}
	if !filter.IsHiddenPath("/obj") {
		t.Fatal("unhide changed the whole category")
	}
}

func TestWorkshopTemplatePlaceholdersStayInvisible(t *testing.T) {
	ws := &WsShip{app: &previewApp{}}
	for _, path := range []string{"/turf/template_noop", "/area/template_noop"} {
		if ws.visible(path) {
			t.Fatalf("template placeholder %s became visible", path)
		}
	}
}

// Native: the canvas must redraw an individually unhidden type without a
// rebuild, while other objects remain hidden by the category filter.
func exerciseVisibilityExceptions(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	app := ws.app.(*previewApp)
	previous := app.filter
	defer func() { app.filter = previous; render() }()
	app.filter = dm.NewPathsFilter(func(path string) []string {
		if object := app.dme.Objects[path]; object != nil {
			return object.DirectChildren
		}
		return nil
	})
	app.filter.SetVisible("/area", false)
	var table, other unit.Unit
	for _, tile := range ws.pane.ViewDmm().Tiles {
		for _, instance := range tile.Instances() {
			path := instance.Prefab().Path()
			if strings.HasPrefix(path, "/obj/structure/table") {
				table = unit.Make(tile.Coord.X, tile.Coord.Y, instance, dmmap.WorldIconSize)
			} else if strings.HasPrefix(path, "/obj/") {
				other = unit.Make(tile.Coord.X, tile.Coord.Y, instance, dmmap.WorldIconSize)
			}
		}
	}
	if table.Instance() == nil || other.Instance() == nil {
		t.Fatal("visibility fixture needs a table and another object")
	}
	app.filter.SetVisible("/obj", false)
	for i := 0; i < 3; i++ {
		render()
	}
	hidden := ws.pane.Canvas().ReadPixels()
	if ws.pane.ProcessUnit(table) || ws.pane.ProcessUnit(other) {
		t.Fatal("bulk-hidden objects reached the renderer")
	}
	app.filter.TogglePath(table.Instance().Prefab().Path())
	if !ws.pane.ProcessUnit(table) {
		t.Fatal("individually unhidden table is still rejected by the renderer")
	}
	if ws.pane.ProcessUnit(other) {
		t.Fatal("unhiding a table also revealed unrelated objects")
	}
	for i := 0; i < 3; i++ {
		render()
	}
	if bytes.Equal(hidden, ws.pane.Canvas().ReadPixels()) {
		t.Fatal("unhiding the table did not change the canvas pixels")
	}
	app.filter.SetVisible("/obj", false)
	if ws.pane.ProcessUnit(table) {
		t.Fatal("bulk hiding again kept the table visible")
	}
	app.filter.SetVisible("/obj", true)
	if !ws.pane.ProcessUnit(table) || !ws.pane.ProcessUnit(other) {
		t.Fatal("bulk showing failed after a visibility exception")
	}
}
