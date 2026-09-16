package wsruin

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/app/ui/cpwsarea/workspace"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/app/ui/cpwsarea/wspreview"
	"sdmm/internal/app/ui/dialog"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/imguiext/style"
	"sdmm/internal/planet"
	"sdmm/internal/ruin"
)

type App interface {
	LoadedEnvironment() *dmenv.Dme
	DoLoadResource(string)
	DoSelectPrefabByPath(string)
	SyncPrefabs()
}

type form struct {
	Name, Description, Cost, Mineral, Weight string
	Duplicates, Always, Manual               bool
}

func makeForm(p ruin.Properties) form {
	return form{p.Name, p.Description, strconv.FormatFloat(p.Cost, 'f', -1, 64), strconv.FormatFloat(p.MineralCost, 'f', -1, 64), strconv.FormatFloat(p.Weight, 'f', -1, 64), p.AllowDuplicates, p.AlwaysPlace, p.Unpickable}
}
func (f form) properties() (ruin.Properties, error) {
	p := ruin.Properties{Name: strings.TrimSpace(f.Name), Description: f.Description, AllowDuplicates: f.Duplicates, AlwaysPlace: f.Always, Unpickable: f.Manual}
	for _, v := range []struct {
		label, value string
		target       *float64
	}{{"Ruin budget", f.Cost, &p.Cost}, {"Mineral budget", f.Mineral, &p.MineralCost}, {"Spawn weight", f.Weight, &p.Weight}} {
		n, err := strconv.ParseFloat(strings.TrimSpace(v.value), 64)
		if err != nil {
			return p, fmt.Errorf("%s needs a number", v.label)
		}
		*v.target = n
	}
	return p, p.Validate()
}

type WsRuin struct {
	workspace.Content
	app                                       App
	catalog                                   *ruin.Catalog
	project                                   *ruin.Project
	selected                                  *ruin.Template
	form, initial                             form
	creating                                  bool
	step, destination, location, ground, area int
	width, height                             int32
	filter, locationFilter, id, message       string
	customID                                  bool
	SourceBusy                                func(string) bool
	removalBackup                             string
	LiveMap                                   func(string) *dmmap.Dmm
	PlanetDrafts                              func() []planet.State
	planetCatalog                             *planet.Catalog
	planetPreview                             *wspreview.Preview
	viewOnPlanet, planetRefresh, planetCaves  bool
	planetPath                                string
	planetSeeds                               *planet.Seeds
	planetLevel                               int
	planetLevels                              int
	planetHosts                               []planet.State
	planetTraits                              map[string]bool
}

func New(app App) *WsRuin {
	ws := &WsRuin{app: app, width: 24, height: 24}
	ws.refresh()
	return ws
}

func (ws *WsRuin) SpriteContext() (*dmmap.Dmm, *dmenv.Dme) {
	if ws.planetPreview != nil {
		return ws.planetPreview.SpriteContext()
	}
	return nil, ws.app.LoadedEnvironment()
}
func (ws *WsRuin) refresh() {
	var err error
	ws.catalog, err = ruin.Discover(ws.app.LoadedEnvironment())
	if err != nil {
		ws.message = err.Error()
	}
	ws.planetTraits = map[string]bool{}
	ws.planetCatalog, _ = planet.Discover(ws.app.LoadedEnvironment())
	if ws.catalog != nil {
		for path, obj := range ws.catalog.Dme.Objects {
			if strings.HasPrefix(path, "/datum/overmap/planet/") && obj.Vars.ValueV("planet_template", "null") != "null" {
				ws.planetTraits[obj.Vars.ValueV("ruin_type", "null")] = true
			}
		}
	}
}
func (ws *WsRuin) Name() string {
	prefix := ""
	if ws.IsModified() {
		prefix = "* "
	}
	return prefix + "Ruin Workshop###" + ws.Id()
}
func (ws *WsRuin) Title() string    { return "Ruin Workshop" }
func (ws *WsRuin) IsModified() bool { return ws.form != ws.initial || ws.creating && ws.step > 0 }
func (ws *WsRuin) OnFocusChange(focused bool) {
	if focused {
		tools.SetEnabled(false)
		ws.planetRefresh = true
	}
}
func (ws *WsRuin) Dispose() {
	if ws.planetPreview != nil {
		ws.planetPreview.Dispose()
	}
}

func (ws *WsRuin) navigate(action func()) {
	if !ws.IsModified() {
		action()
		return
	}
	dialog.Open(dialog.TypeConfirmation{Title: "Save ruin properties?", Question: "Save your ruin setup before continuing?", ActionYes: func() {
		if ws.Save() {
			action()
		}
	}, ActionNo: action})
}
func (ws *WsRuin) BeginNewRuin() {
	ws.navigate(func() {
		ws.refresh()
		if ws.catalog == nil || len(ws.catalog.Locations) == 0 {
			ws.message = "This project has no ruin locations."
			return
		}
		ws.selected = nil
		ws.viewOnPlanet = false
		ws.project = nil
		ws.creating = true
		ws.step = 0
		ws.customID = false
		ws.width, ws.height = 24, 24
		ws.ground, ws.area = 0, 0
		ws.message = ""
		ws.form = makeForm(ruin.Properties{Weight: 1})
		ws.chooseDestination(destinationPlanet)
		ws.form.Name = ""
		ws.form.Description = ""
		ws.initial = ws.form
	})
}
func (ws *WsRuin) chooseLocation(index int) {
	ws.location = index
	ws.area = 0
	loc := ws.catalog.Locations[index]
	p, err := ruin.ReadProperties(ws.catalog.Dme.Objects[loc.Type])
	if err == nil {
		p.Name = ws.form.Name
		p.Description = ws.form.Description
		ws.form = makeForm(p)
	}
	ws.suggestID()
}

const (
	destinationSpace = iota
	destinationPlanet
	destinationAnywhere
)

var destinations = []string{"Space", "A planet type", "Anywhere"}

func (ws *WsRuin) chooseDestination(destination int) {
	ws.destination = destination
	for i, loc := range ws.catalog.Locations {
		if (loc.Type == ruin.Type+"/space") == (destination == destinationSpace) {
			ws.chooseLocation(i)
			return
		}
	}
	if destination != destinationAnywhere {
		ws.destination = destinationPlanet
		if ws.catalog.Locations[0].Type == ruin.Type+"/space" {
			ws.destination = destinationSpace
		}
	}
	ws.chooseLocation(0)
}

func (ws *WsRuin) suggestID() {
	if !ws.customID {
		if ws.destination == destinationAnywhere {
			ws.id = ws.catalog.SuggestAnywhereID(ws.form.Name)
		} else {
			ws.id = ws.catalog.SuggestID(ws.form.Name, ws.catalog.Locations[ws.location])
		}
	}
}

func (ws *WsRuin) areas() []ruin.AreaChoice {
	return ws.catalog.Areas(ws.catalog.Locations[ws.location], ws.destination == destinationAnywhere)
}

func (ws *WsRuin) destinationName() string {
	if ws.destination == destinationAnywhere {
		return "Anywhere"
	}
	return ws.catalog.Locations[ws.location].Name
}
func (ws *WsRuin) selectRuin(t ruin.Template) {
	ws.navigate(func() {
		ws.creating = false
		ws.selected = &t
		ws.viewOnPlanet = false
		ws.message = ""
		ws.form = form{}
		ws.initial = ws.form
		var err error
		ws.project, err = ruin.Open(ws.catalog, t)
		if err != nil {
			ws.message = err.Error()
			return
		}
		ws.form = makeForm(ws.project.Properties)
		ws.initial = ws.form
	})
}

type groundOption struct{ name, path string }

func (ws *WsRuin) grounds() []groundOption {
	var result []groundOption
	for _, g := range []groundOption{{"Keep the surrounding terrain", "/turf/template_noop"}, {"Plating", "/turf/open/floor/plating"}, {"Space", "/turf/open/space"}} {
		if ws.catalog.Dme.Objects[g.path] != nil {
			result = append(result, g)
		}
	}
	return result
}
func (ws *WsRuin) setup() (ruin.Setup, error) {
	p, err := ws.form.properties()
	if err != nil {
		return ruin.Setup{}, err
	}
	if ws.catalog == nil || ws.location >= len(ws.catalog.Locations) {
		return ruin.Setup{}, fmt.Errorf("choose a ruin location")
	}
	grounds := ws.grounds()
	if ws.ground >= len(grounds) {
		return ruin.Setup{}, fmt.Errorf("this project has no supported starting turf")
	}
	areas := ws.areas()
	if ws.area >= len(areas) {
		return ruin.Setup{}, fmt.Errorf("this project has no supported ruin areas")
	}
	s := ruin.Setup{Location: ws.catalog.Locations[ws.location], ID: ws.id, Properties: p, Width: int(ws.width), Height: int(ws.height), Turf: grounds[ws.ground].path, Area: areas[ws.area].Path, Anywhere: ws.destination == destinationAnywhere}
	return s, ws.catalog.ValidateSetup(s)
}

func (ws *WsRuin) Save() bool {
	if !ws.IsModified() {
		return true
	}
	var p *ruin.Project
	var err error
	if ws.creating {
		var s ruin.Setup
		s, err = ws.setup()
		if err == nil {
			p, err = ruin.New(ws.catalog, s)
		}
	} else {
		p = ws.project
		if p == nil {
			return false
		}
		p.Properties, err = ws.form.properties()
	}
	if err == nil {
		err = p.Save()
	}
	if err != nil {
		ws.message = err.Error()
		return false
	}
	ws.project = p
	t := p.Template
	ws.selected = &t
	ws.creating = false
	ws.initial = ws.form
	ws.refresh()
	ws.app.SyncPrefabs()
	ws.message = "Ruin properties saved."
	return true
}

func (ws *WsRuin) Process() {
	scale := window.PointSize()
	workshop.PushStyle()
	defer workshop.PopStyle()
	workshop.Banner("Ruin Workshop", "Locations, maps & encounters", style.Amber)
	if ws.catalog == nil {
		imgui.TextWrapped(ws.message)
		return
	}
	width := 300 * scale
	if avail := imgui.ContentRegionAvail().X; width > avail*.4 {
		width = avail * .4
	}
	workshop.Panel("ruin-library", imgui.Vec2{X: width}, false)
	ws.library()
	imgui.EndChild()
	imgui.SameLine()
	workshop.Panel("ruin-details", imgui.Vec2{}, false)
	if ws.creating {
		ws.wizard()
	} else if ws.viewOnPlanet && ws.selected != nil {
		ws.planetView()
	} else if ws.selected != nil {
		ws.details()
	} else {
		workshop.Title("Build a place to discover")
		hint("Choose a ruin from the library, or create a new encounter.")
		workshop.Gap()
		if button("Create a ruin") {
			ws.BeginNewRuin()
		}
	}
	if ws.message != "" {
		imgui.Spacing()
		imgui.TextWrapped(ws.message)
	}
	ws.removalRecovery()
	imgui.EndChild()
}
func heading(s string) { workshop.Section(s, style.Amber) }

func field(label string, value *string) {
	imgui.Text(label)
	imgui.SetNextItemWidth(-1)
	imgui.InputText("##"+label, value)
}
func button(label string) bool { return workshop.Button(label, false) }

func combo(label, preview string) bool {
	imgui.Text(label)
	imgui.SetNextItemWidth(-1)
	return imgui.BeginCombo("##"+label, preview)
}
func hint(s string) { workshop.Muted(s) }

func (ws *WsRuin) library() {
	workshop.Title("Ruin library")
	if workshop.Button("+ Create a ruin", true) {
		ws.BeginNewRuin()
	}
	field("Search ruins", &ws.filter)
	preview := ws.locationFilter
	if preview == "" {
		preview = "All locations"
	}
	if combo("Location", preview) {
		if imgui.Selectable("All locations") {
			ws.locationFilter = ""
		}
		if imgui.Selectable("Anywhere") {
			ws.locationFilter = "Anywhere"
		}
		for _, loc := range ws.catalog.Locations {
			if imgui.Selectable(loc.Name) {
				ws.locationFilter = loc.Name
			}
		}
		imgui.EndCombo()
	}
	imgui.Separator()
	imgui.BeginChild("ruin-results")
	count := 0
	for _, t := range ws.catalog.Templates {
		if ws.locationFilter != "" && !t.PlacedIn(ws.locationFilter) {
			continue
		}
		if !strings.Contains(strings.ToLower(t.Name+" "+t.ID+" "+t.Location), strings.ToLower(ws.filter)) {
			continue
		}
		count++
		selected := ws.selected != nil && ws.selected.Type == t.Type && !ws.creating
		imgui.PushID(t.Type)
		if workshop.Row("ruin", t.Name, t.Location, ">", selected, style.Amber, 0) {
			ws.selectRuin(t)
		}
		imgui.PopID()
	}
	if count == 0 {
		hint("No ruins match this search.")
	}
	imgui.EndChild()
}

func (ws *WsRuin) identity() {
	field("Ruin name", &ws.form.Name)
	imgui.Text("Description")
	imgui.SetNextItemWidth(-1)
	imgui.InputTextMultilineV("##description", &ws.form.Description, imgui.Vec2{X: -1, Y: 95 * window.PointSize()}, 0, nil)
}
func (ws *WsRuin) spawning() {
	mode := "Random placement"
	if ws.form.Always {
		mode = "Always place"
	}
	if ws.form.Manual {
		mode = "Manual or linked placement only"
	}
	if combo("Ruin placement", mode) {
		for i, label := range []string{"Random placement", "Always place", "Manual or linked placement only"} {
			if imgui.Selectable(label) {
				ws.form.Always = i == 1
				ws.form.Manual = i == 2
			}
		}
		imgui.EndCombo()
	}
	hint("Controls when the whole ruin appears in a generated planet or space encounter.")
	imgui.Checkbox("Allow more than one per map", &ws.form.Duplicates)
	field("Ruin budget cost", &ws.form.Cost)
	field("Spawn weight", &ws.form.Weight)
	hint("Higher weight makes a random ruin more likely. Its cost comes from the location's ruin budget.")
	if imgui.CollapsingHeader("More spawning options") {
		field("Mineral budget cost", &ws.form.Mineral)
	}
}

func (ws *WsRuin) wizard() {
	heading("New ruin")
	steps := []string{"1. Name and destination", "2. Map canvas", "3. Ruin placement", "4. Create"}
	imgui.Text(steps[ws.step])
	imgui.Separator()
	switch ws.step {
	case 0:
		ws.identity()
		if combo("Where can this ruin appear?", destinations[ws.destination]) {
			for i, name := range destinations {
				available := i == destinationAnywhere
				for _, loc := range ws.catalog.Locations {
					if (loc.Type == ruin.Type+"/space") == (i == destinationSpace) {
						available = true
					}
				}
				if available && imgui.Selectable(name) {
					ws.chooseDestination(i)
				}
			}
			imgui.EndCombo()
		}
		if ws.destination == destinationPlanet && combo("Planet type", ws.catalog.Locations[ws.location].Name) {
			for i, loc := range ws.catalog.Locations {
				if loc.Type != ruin.Type+"/space" && imgui.Selectable(loc.Name) {
					ws.chooseLocation(i)
				}
			}
			imgui.EndCombo()
		}
		if ws.destination == destinationAnywhere {
			hint("Uses one shared map for space encounters and every supported planet type.")
		}
		ws.suggestID()
		if imgui.CollapsingHeader("File identifier") {
			imgui.Checkbox("Choose identifier manually", &ws.customID)
			imgui.BeginDisabledV(!ws.customID)
			field("Identifier", &ws.id)
			imgui.EndDisabled()
		}
	case 1:
		heading("Map size")
		if combo("Canvas preset", fmt.Sprintf("%d x %d tiles", ws.width, ws.height)) {
			for _, size := range []int32{16, 24, 32, 48, 64} {
				if imgui.Selectable(fmt.Sprintf("%d x %d", size, size)) {
					ws.width = size
					ws.height = size
				}
			}
			imgui.EndCombo()
		}
		imgui.Text("Width in tiles")
		imgui.SetNextItemWidth(-1)
		imgui.InputInt("##width", &ws.width)
		imgui.Text("Height in tiles")
		imgui.SetNextItemWidth(-1)
		imgui.InputInt("##height", &ws.height)
		grounds := ws.grounds()
		if len(grounds) > 0 && combo("Starting terrain", grounds[ws.ground].name) {
			for i, g := range grounds {
				if imgui.Selectable(g.name) {
					ws.ground = i
				}
			}
			imgui.EndCombo()
		}
		hint("Use the normal mapping tools to build and furnish the ruin.")
		areas := ws.areas()
		if len(areas) > 0 && combo("Starting area", areas[ws.area].Name) {
			for i, area := range areas {
				if imgui.Selectable(area.Name) {
					ws.area = i
				}
			}
			imgui.EndCombo()
		}
		hint("Fills the canvas with an existing area. Paint interiors and outdoors with the normal area tools as you build.")
		if ws.destination == destinationAnywhere {
			hint("Outdoors keeps the host planet or space area's settings.")
		}
	case 2:
		ws.spawning()
	case 3:
		imgui.TextWrapped(ws.form.Name)
		imgui.Text(fmt.Sprintf("%s | %d x %d tiles", ws.destinationName(), ws.width, ws.height))
		if ws.form.Description != "" {
			hint(ws.form.Description)
		}
		imgui.Spacing()
		if grounds := ws.grounds(); ws.ground < len(grounds) {
			imgui.Text("Terrain: " + grounds[ws.ground].name)
		}
		if areas := ws.areas(); ws.area < len(areas) {
			imgui.Text("Area: " + areas[ws.area].Name)
		}
		spawning := "Random placement"
		if ws.form.Always {
			spawning = "Always place"
		}
		if ws.form.Manual {
			spawning = "Manual or linked placement only"
		}
		imgui.Text("Ruin placement: " + spawning)
		imgui.Text("Budget: " + ws.form.Cost + " | Weight: " + ws.form.Weight)
		imgui.Spacing()
		imgui.TextWrapped("Create and register the map, then open it for editing.")
		if imgui.CollapsingHeader("Files") {
			if s, err := ws.setup(); err == nil {
				if p, err := ruin.New(ws.catalog, s); err == nil {
					ws.filePreview(p)
				} else {
					hint(err.Error())
				}
			}
		}
	}
	imgui.Spacing()
	if ws.step > 0 && button("Back") {
		ws.step--
		ws.message = ""
	}
	_, err := ws.setup()
	if err != nil {
		hint(err.Error())
	}
	imgui.BeginDisabledV(err != nil)
	if ws.step < 3 {
		if button("Continue") {
			ws.step++
			ws.message = ""
		}
	} else if button("Create ruin and open map") {
		if ws.Save() {
			ws.app.DoLoadResource(ws.selected.File)
		}
	}
	imgui.EndDisabled()
	if button("Cancel") {
		ws.navigate(func() {
			ws.creating = false
			ws.project = nil
			ws.selected = nil
			ws.form = form{}
			ws.initial = ws.form
			ws.message = ""
		})
	}
}

func (ws *WsRuin) filePreview(p *ruin.Project) {
	changes, err := p.Changes()
	if err != nil {
		hint(err.Error())
		return
	}
	for _, c := range changes {
		if string(c.Before) == string(c.After) {
			continue
		}
		rel, _ := filepath.Rel(ws.catalog.Dme.RootDir, c.Path)
		action := "Create "
		if c.Existed {
			action = "Update "
		}
		hint(action + filepath.ToSlash(rel))
	}
}

func (ws *WsRuin) details() {
	heading(ws.selected.Name)
	hint(ws.selected.Location)
	defer func() {
		workshop.Gap()
		if workshop.DangerButton("Remove ruin...") {
			ws.requestRemoval()
		}
	}()
	imgui.BeginDisabledV(ws.selected.Problem != "")
	if button("Open map for editing") {
		ws.app.DoLoadResource(ws.selected.File)
	}
	if ws.hasPlanetDestination() && button("View on planet") {
		ws.beginPlanetView()
	}
	imgui.EndDisabled()
	if ws.selected.Problem == "" && imgui.CollapsingHeader("Areas for this ruin") {
		hint("Select an existing area, then paint it onto the map with the area tools.")
		for _, area := range ws.catalog.TemplateAreas(*ws.selected) {
			if button("Select " + area.Name) {
				ws.app.DoLoadResource(ws.selected.File)
				ws.app.DoSelectPrefabByPath(area.Path)
			}
		}
	}
	if ws.selected.Problem != "" {
		imgui.TextWrapped(ws.selected.Problem)
	}
	if ws.project == nil {
		return
	}
	imgui.Separator()
	ws.identity()
	ws.spawning()
	_, err := ws.form.properties()
	if err == nil && ws.form.Name != ws.initial.Name {
		err = ws.catalog.NameError(ws.form.Name, ws.selected.Type)
	}
	if err != nil {
		hint(err.Error())
	}
	imgui.BeginDisabledV(err != nil || !ws.IsModified())
	if button("Save ruin properties") {
		ws.Save()
	}
	imgui.EndDisabled()
	if ws.IsModified() && button("Revert property changes") {
		ws.form = ws.initial
		ws.message = ""
	}
	if imgui.CollapsingHeader("Source details") {
		hint(ws.selected.Type)
		rel, _ := filepath.Rel(ws.catalog.Dme.RootDir, ws.selected.File)
		hint(filepath.ToSlash(rel))
		for _, key := range []string{"always_spawn_with", "never_spawn_with"} {
			value := ws.catalog.Dme.Objects[ws.selected.Type].Vars.ValueV(key, "null")
			if value != "null" {
				hint(key + " = " + value)
			}
		}
	}
}
