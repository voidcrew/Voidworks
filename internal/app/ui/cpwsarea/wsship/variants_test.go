package wsship

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/ship"
)

// Repainting the sidebar must not repeat path resolution, for either an
// existing shared room or a room that only has variant-specific copies.
func TestSharedRoomStatusHasNoPerFrameAllocations(t *testing.T) {
	for _, exists := range []bool{false, true} {
		name := "missing"
		if exists {
			name = "shared"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			m := ship.Module{ID: "cargo", File: "cargo.dmm"}
			if exists {
				if err := os.WriteFile(filepath.Join(root, m.File), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			ws := &WsShip{project: &ship.Project{Catalog: &ship.Catalog{Root: root}}}
			if ws.sharedRoom(m) != exists {
				t.Fatal("incorrect initial shared-room status")
			}
			if allocations := testing.AllocsPerRun(100, func() {
				if ws.sharedRoom(m) != exists {
					t.Fatal("shared-room status changed between frames")
				}
			}); allocations != 0 {
				t.Fatalf("repainting a room status allocated %g times", allocations)
			}
		})
	}
}

func TestThemeDetail(t *testing.T) {
	for _, c := range []struct {
		name string
		info ship.ThemeInfo
		want string
	}{
		{"default", ship.ThemeInfo{Default: true, Slots: []string{"a", "b", "c"}, Price: "Free"}, "default  ·  3 rooms  ·  Free"},
		{"priced", ship.ThemeInfo{Slots: []string{"a", "b", "c", "d"}, Price: "12 science parts", Crew: 6}, "4 rooms  ·  12 science parts  ·  6 crew"},
		{"single", ship.ThemeInfo{Slots: []string{"a"}, Price: "Free", Crew: 1}, "1 room  ·  Free  ·  1 crew"},
		{"bare", ship.ThemeInfo{}, "no rooms"},
	} {
		if got := themeDetail(c.info); got != c.want {
			t.Fatalf("%s: %q", c.name, got)
		}
	}
}

func TestOptionStatusText(t *testing.T) {
	for _, c := range []struct {
		option ship.OptionStatus
		shared bool
		want   string
	}{
		{ship.OptionStatus{}, true, "Not offered here"},
		{ship.OptionStatus{Available: true}, true, "Shared"},
		{ship.OptionStatus{Available: true, Forked: true}, true, "Own copy"},
		{ship.OptionStatus{Available: true, Forked: true, Differs: true}, true, "Own copy  ·  differs"},
		// An option with no base room is a copy in every variant.
		{ship.OptionStatus{Available: true, Forked: true, Differs: true}, false, "Own copy"},
	} {
		if got := optionStatusText(c.option, c.shared); got != c.want {
			t.Fatalf("%+v shared=%v: %q", c.option, c.shared, got)
		}
	}
}

// Switching variants keeps the edited room when the other variant has it, and
// keeps its option only when that variant offers the same one.
func TestKeptRoomAcrossVariantSwitch(t *testing.T) {
	h := ship.Hull{
		Slots: []string{"cargo", "bridge"},
		Themes: []ship.Theme{
			{ID: "standard", Default: true},
			{ID: "salvage", Slots: []string{"cargo"}},
		},
		Modules: []ship.Module{
			{ID: "cargo_basic", Slot: "cargo", Themes: []string{"standard", "salvage"}},
			{ID: "cargo_medical", Slot: "cargo", Themes: []string{"standard"}},
			{ID: "bridge_basic", Slot: "bridge", Themes: []string{"standard"}},
		},
	}
	salvage, standard := h.Themes[1], h.Themes[0]
	if option, keep := keptRoom(h, salvage, "cargo", "cargo_basic"); !keep || option != "cargo_basic" {
		t.Fatalf("shared option: %q %v", option, keep)
	}
	if option, keep := keptRoom(h, salvage, "cargo", "cargo_medical"); !keep || option != "" {
		t.Fatalf("option missing from the variant: %q %v", option, keep)
	}
	if _, keep := keptRoom(h, salvage, "bridge", "bridge_basic"); keep {
		t.Fatal("room missing from the variant was kept")
	}
	if _, keep := keptRoom(h, standard, "", ""); keep {
		t.Fatal("the hull is not a room")
	}
	if option, keep := keptRoom(h, standard, "bridge", "bridge_basic"); !keep || option != "bridge_basic" {
		t.Fatalf("switch back: %q %v", option, keep)
	}
}

// Native: the Variants panel. A shared variant is added, one of its rooms is
// forked and put back, a room is disabled, the default moves, the variant is
// deleted, and every step undoes.
func exerciseVariantsPanel(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	capture := func(name string) {
		for i := 0; i < 3; i++ {
			render()
		}
		if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
			captureFrame(t, filepath.Join(dst, name+".png"), 1400, 960)
		}
	}
	ws.setStage(stepBuild)
	ws.finishTask()
	project := ws.project
	ws.theme = ws.defaultThemeIndex()
	ws.defaults()
	ws.rebuild()
	ws.OnFocusChange(true)
	if ws.message != "" || ws.invalid {
		t.Fatalf("variants fixture: %s", ws.message)
	}
	before := append([]ship.Theme{}, project.Hull.Themes...)
	if len(before) < 2 {
		t.Fatalf("fixture has %d variants", len(before))
	}
	// Workshop ships keep one room map per variant. Give the cargo option a
	// shared base as fleet ships have, so a new variant can share it.
	basic, ok := ws.module("cargo_basic")
	if !ok {
		t.Fatal("fixture lost its cargo option")
	}
	themed, err := ship.Inside(ws.catalog.Root, filepath.Join(ws.catalog.ModuleDir, strings.TrimSuffix(basic.File, ".dmm")+"_"+before[0].ID+".dmm"))
	if err != nil {
		t.Fatal(err)
	}
	shared, err := ship.Inside(ws.catalog.Root, filepath.Join(ws.catalog.ModuleDir, basic.File))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(themed)
	if err != nil {
		t.Fatal(err)
	}
	if ws.sharedRoom(basic) {
		t.Fatal("fixture unexpectedly already has a shared room")
	}
	if err = os.WriteFile(shared, data, 0600); err != nil {
		t.Fatal(err)
	}
	ws.editRoom("cargo")
	if ws.editingSlot() != "cargo" {
		t.Fatal("fixture cannot edit its room")
	}
	if !ws.sharedRoom(basic) {
		t.Fatal("rebuilding after a room change kept stale shared-room status")
	}
	capture("variants-panel")
	// Add a variant that shares the ship's rooms.
	ws.beginTask(taskTheme)
	if ws.copyRooms {
		t.Fatal("the variant form does not default to shared rooms")
	}
	ws.itemID, ws.itemName = "salvager", "Salvager"
	capture("variants-new")
	ws.createVariant()
	if ws.message != "" || ws.invalid {
		t.Fatalf("create variant: %s", ws.message)
	}
	ws.finishTask()
	added := ws.currentTheme()
	if len(project.Hull.Themes) != len(before)+1 || added.ID != "salvager" || ws.theme != len(before) {
		t.Fatalf("new variant is not the one being edited: %+v", added)
	}
	// The new hull map is part of the project's unsaved changes.
	ws.setStage(stepReview)
	capture("variants-review")
	newHull := ""
	for _, item := range ws.reviewed {
		for _, file := range item.files {
			if !file.existed && strings.HasSuffix(file.path, added.Suffix+".dmm") {
				newHull = file.path
			}
		}
	}
	if newHull == "" {
		t.Fatalf("review does not list the new variant's hull: %+v", ws.reviewed)
	}
	ws.setStage(stepBuild)
	info, summaryErr := ws.variant(added, true)
	if summaryErr != "" {
		t.Fatal(summaryErr)
	}
	if detail := themeDetail(info); !strings.Contains(detail, "room") {
		t.Fatalf("variant detail: %q", detail)
	}
	status := func(id string) ship.OptionStatus {
		ws.variantInfos = nil
		fresh, err := ws.variant(ws.currentTheme(), true)
		if err != "" {
			t.Fatal(err)
		}
		for _, o := range fresh.Options {
			if o.ModuleID == id {
				return o
			}
		}
		t.Fatalf("variant has no %s option", id)
		return ship.OptionStatus{}
	}
	statusText := func(id string) string {
		m, ok := ws.module(id)
		if !ok {
			t.Fatalf("variant has no %s option", id)
		}
		return optionStatusText(status(id), ws.sharedRoom(m))
	}
	if got := statusText("cargo_basic"); got != "Shared" {
		t.Fatalf("shared room after a shared variant: %q", got)
	}
	// The room with no base map was copied even though rooms are shared.
	if got := statusText("medical"); got != "Own copy" {
		t.Fatalf("room without a shared base: %q", got)
	}
	capture("variants-expanded")
	section := roomSection{hull: project.Hull.Type, theme: added.ID, slot: "cargo", variants: true}
	ws.setRoomCollapsed(section, true)
	checkCollapsed := func() {
		t.Helper()
		render()
		if !ws.collapsedSections[section] {
			t.Fatal("a workshop transition reset the collapsed variant room")
		}
	}
	ws.rebuild()
	checkCollapsed()
	// The edited room follows a variant switch when both variants have it.
	ws.editRoom("cargo")
	checkCollapsed()
	ws.selectRoomOption("cargo", "medical")
	checkCollapsed()
	ws.selectRoomOption("cargo", "cargo_basic")
	ws.beginTask(taskSettings)
	render()
	ws.finishTask()
	checkCollapsed()
	ws.switchTheme(0)
	checkCollapsed()
	if ws.theme != 0 || ws.editingSlot() != "cargo" {
		t.Fatalf("switching variants dropped the edited room: %d %q", ws.theme, ws.editingSlot())
	}
	ws.switchTheme(len(before))
	checkCollapsed()
	if ws.theme != len(before) || ws.editingSlot() != "cargo" {
		t.Fatal("switching back dropped the edited room")
	}
	// Fork the shared room, then put it back.
	ws.forkOption("cargo_basic", added.ID)
	checkCollapsed()
	if ws.message != "" || ws.invalid {
		t.Fatalf("fork: %s", ws.message)
	}
	if got := statusText("cargo_basic"); got != "Own copy" {
		t.Fatalf("forked room status: %q", got)
	}
	if suffix, _ := ws.editingShare(); suffix != "(this variant)" {
		t.Fatalf("canvas header after fork: %q", suffix)
	}
	ws.unforkOption("cargo_basic", added.ID)
	if ws.message != "" || ws.invalid {
		t.Fatalf("unfork: %s", ws.message)
	}
	if got := statusText("cargo_basic"); got != "Shared" {
		t.Fatalf("unforked room status: %q", got)
	}
	ws.share = editShare{}
	if suffix, about := ws.editingShare(); suffix != "(shared)" || about == "" {
		t.Fatalf("canvas header after unfork: %q", suffix)
	}
	// Leave the room out of this variant, then put it back.
	ws.setVariantRoom(added.ID, "cargo", false)
	checkCollapsed()
	if ws.message != "" || ws.invalid {
		t.Fatalf("disable room: %s", ws.message)
	}
	if ship.Contains(project.Hull.SlotsFor(ws.currentTheme()), "cargo") || ws.source != 0 {
		t.Fatalf("disabled room still loads: source %d", ws.source)
	}
	capture("variants-room-off")
	ws.setVariantRoom(added.ID, "cargo", true)
	checkCollapsed()
	if ws.message != "" || !ship.Contains(project.Hull.SlotsFor(ws.currentTheme()), "cargo") {
		t.Fatalf("enable room: %s", ws.message)
	}
	// The default variant is the one players get for free.
	ws.change("Set default variant", func() error { return project.SetDefaultTheme(added.ID) })
	if ws.message != "" || !project.Hull.Themes[ws.theme].Default {
		t.Fatalf("make default: %s", ws.message)
	}
	ws.deleteVariant(added.ID)
	if ws.message == "" {
		t.Fatal("deleting the default variant was allowed")
	}
	ws.message = ""
	ws.change("Set default variant", func() error { return project.SetDefaultTheme(before[0].ID) })
	ws.deleteVariant(added.ID)
	if ws.message != "" || ws.invalid {
		t.Fatalf("delete variant: %s", ws.message)
	}
	if len(project.Hull.Themes) != len(before) || ws.theme < 0 || ws.theme >= len(before) {
		t.Fatalf("after deletion: %d variants, editing %d", len(project.Hull.Themes), ws.theme)
	}
	if ws.currentTheme().ID != before[0].ID {
		t.Fatalf("deletion did not fall back to the remaining variant: %q", ws.currentTheme().ID)
	}
	// Undo every step, ending on the fixture the other walkthroughs expect.
	for i := 0; i < 8; i++ {
		ws.app.CommandStorage().Undo()
		if ws.invalid || ws.message != "" {
			t.Fatalf("undo %d: invalid=%v %s", i, ws.invalid, ws.message)
		}
		if ws.theme >= len(project.Hull.Themes) {
			t.Fatalf("undo %d left variant %d of %d", i, ws.theme, len(project.Hull.Themes))
		}
	}
	if len(project.Hull.Themes) != len(before) {
		t.Fatalf("undo left %d variants", len(project.Hull.Themes))
	}
	for i, theme := range project.Hull.Themes {
		if theme.ID != before[i].ID || theme.Default != before[i].Default {
			t.Fatalf("undo changed variant %d: %+v", i, theme)
		}
	}
	ws.theme = ws.defaultThemeIndex()
	ws.defaults()
	ws.rebuild()
	ws.editRoom("cargo")
	if ws.invalid || ws.editingSlot() != "cargo" {
		t.Fatalf("fixture is not editable after the walkthrough: %s", ws.message)
	}
}

// Disclosure clicks must hide the option controls without changing what the
// variant loads. Each ship and variant remembers its own open room sections.
func TestVariantRoomCollapsePreservesConfiguration(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx := imgui.CreateContext(nil)
	defer ctx.Destroy()
	io := imgui.CurrentIO()
	io.SetIniFilename("")
	io.SetDisplaySize(imgui.Vec2{X: 400, Y: 400})
	io.SetDeltaTime(1.0 / 60)
	io.Fonts().TextureDataRGBA32()
	ws := &WsShip{
		project: &ship.Project{Hull: ship.Hull{Type: "/datum/map_template/shuttle/test", Slots: []string{"cargo"},
			Modules: []ship.Module{{ID: "cargo_basic", Slot: "cargo", Name: "Cargo", File: "cargo.dmm"}}}},
		selected:    map[string]string{"cargo": "cargo_basic"},
		sharedRooms: map[string]bool{"cargo.dmm": true},
	}
	info := ship.ThemeInfo{ID: "standard", Slots: []string{"cargo"},
		Options: []ship.OptionStatus{{ModuleID: "cargo_basic", Slot: "cargo", Available: true}}}
	before, err := json.Marshal(ws.project.Hull)
	if err != nil {
		t.Fatal(err)
	}
	var arrow imgui.Vec2
	host := "Variant room disclosure"
	frame := func() float32 {
		imgui.NewFrame()
		imgui.SetNextWindowPos(imgui.Vec2{})
		imgui.SetNextWindowSize(imgui.Vec2{X: 400, Y: 400})
		imgui.BeginV(host, nil, imgui.WindowFlagsNoSavedSettings)
		imgui.BeginChild("ship-controls")
		pos := imgui.CursorScreenPos()
		arrow = pos.Plus(imgui.Vec2{X: 22, Y: imgui.FrameHeight() / 2})
		ws.variantRooms(info)
		height := imgui.CursorScreenPos().Y - pos.Y
		imgui.EndChild()
		imgui.End()
		imgui.Render()
		return height
	}
	click := func() float32 {
		io.SetMousePosition(arrow)
		frame()
		io.SetMouseButtonDown(0, true)
		frame()
		io.SetMouseButtonDown(0, false)
		frame()
		io.SetMousePosition(imgui.Vec2{X: -1000, Y: -1000})
		return frame()
	}
	frame()
	expanded := frame()
	collapsed := click()
	if collapsed >= expanded {
		t.Fatalf("collapse did not hide the option rows: %g >= %g", collapsed, expanded)
	}
	info.ID = "salvage"
	if frame() != expanded {
		t.Fatal("collapsing one variant also collapsed another")
	}
	info.ID = "standard"
	originalType := ws.project.Hull.Type
	ws.project.Hull.Type += "_other"
	if frame() != expanded {
		t.Fatal("collapsing one ship also collapsed another")
	}
	ws.project.Hull.Type = originalType
	if frame() != collapsed {
		t.Fatal("switching away and back lost the collapsed state")
	}
	// The disclosure belongs to the workshop even if its containing window
	// changes. ImGui's tree storage alone is scoped to the old child window.
	host = "Reopened variant room disclosure"
	frame()
	if frame() != collapsed {
		t.Fatal("recreating the sidebar lost the collapsed state")
	}
	info.Slots = nil
	frame()
	info.Slots = []string{"cargo"}
	if frame() != collapsed {
		t.Fatal("disabling and enabling a room lost the collapsed state")
	}
	if click() != expanded {
		t.Fatal("expanding the room did not restore its option rows")
	}
	after, err := json.Marshal(ws.project.Hull)
	if err != nil || string(before) != string(after) || ws.selected["cargo"] != "cargo_basic" {
		t.Fatalf("disclosure changed the ship configuration: %s (%v)", after, err)
	}
}
