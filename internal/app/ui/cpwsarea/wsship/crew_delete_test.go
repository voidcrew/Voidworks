package wsship

import (
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/ship"
)

func unsupportedCrewWorkspace(t *testing.T, count int) *WsShip {
	t.Helper()
	vars := dmvars.MutableVariables{}
	vars.Put("jobtype", "/datum/job/assistant")
	env := &dmenv.Dme{Objects: map[string]*dmenv.Object{
		"/datum/job/assistant":        {Path: "/datum/job/assistant", Vars: vars.ToImmutable()},
		"/datum/outfit/job/assistant": {Path: "/datum/outfit/job/assistant", Vars: vars.ToImmutable()},
	}}
	hull := ship.Hull{Type: ship.HullType + "/fixture", Themes: []ship.Theme{{ID: "standard"}}, Modules: []ship.Module{{ID: "cargo", Slot: "cargo"}}, Slots: []string{"cargo"}}
	catalog := &ship.Catalog{Root: t.TempDir(), Hulls: []ship.Hull{hull}}
	jobs := []ship.CrewJob{{Name: "Engineer", Slots: 1, Category: "Engineering", Outfit: "/datum/outfit/job/assistant"}}
	if count == 2 {
		jobs = append(jobs, ship.CrewJob{Name: "DJ", Slots: 1, Category: "Service", Outfit: "/datum/outfit/job/assistant"})
	}
	project := &ship.Project{Dme: env, Catalog: catalog, Hull: hull, Crew: &ship.CrewConfig{Rosters: map[string][]ship.CrewJob{"module/cargo": jobs}}}
	ws := &WsShip{project: project, catalog: catalog, app: &previewApp{dme: env}, task: taskCrew}
	ws.loadCrewScope("module/cargo/theme/standard")
	return ws
}

func TestFailedCrewDeletionRestoresForm(t *testing.T) {
	for _, count := range []int{1, 2} {
		for _, dirty := range []bool{false, true} {
			ws := unsupportedCrewWorkspace(t, count)
			ws.crew.dirty = dirty
			if dirty {
				ws.crew.jobs[0].Name = "Unfinished rename"
			}
			before := ship.CloneCrewJobs(ws.crew.jobs)
			projectBefore := ws.project.Capture()
			if ws.deleteCrewJob() {
				t.Fatal("unsupported game accepted deletion")
			}
			if !strings.Contains(ws.crew.error, "per-theme module crew support") {
				t.Fatalf("missing compatibility error: %s", ws.crew.error)
			}
			if !reflect.DeepEqual(ws.crew.jobs, before) || ws.crew.selected != 0 || ws.crew.dirty != dirty {
				t.Fatalf("rejected deletion changed form: %+v", ws.crew)
			}
			if !reflect.DeepEqual(ws.project.Capture(), projectBefore) {
				t.Fatal("rejected deletion changed project")
			}
			if !dirty && !ws.commitCrew() {
				t.Fatal("rejected deletion blocks navigation")
			}
		}
	}
}

func TestEmptyCrewDraftCanBeDiscarded(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx := imgui.CreateContext(nil)
	defer ctx.Destroy()
	io := imgui.CurrentIO()
	io.SetIniFilename("")
	io.SetDisplaySize(imgui.Vec2{X: 600, Y: 500})
	io.SetDeltaTime(1.0 / 60)
	io.Fonts().TextureDataRGBA32()
	ws := unsupportedCrewWorkspace(t, 1)
	// Recover an empty pending deletion made by an older editor version.
	ws.crew.jobs, ws.crew.selected, ws.crew.dirty = nil, -1, true
	var discard imgui.Vec2
	frame := func() {
		imgui.NewFrame()
		imgui.SetNextWindowPos(imgui.Vec2{})
		imgui.SetNextWindowSize(imgui.Vec2{X: 600, Y: 500})
		imgui.BeginV("Empty crew recovery", nil, imgui.WindowFlagsNoSavedSettings)
		if ws.crew.selected < 0 {
			ws.crewContent()
			lo, hi := imgui.ItemRectMin(), imgui.ItemRectMax()
			discard = imgui.Vec2{X: (lo.X + hi.X) / 2, Y: (lo.Y + hi.Y) / 2}
		}
		imgui.End()
		imgui.Render()
	}
	for range 3 {
		frame()
	}
	io.SetMousePosition(discard)
	frame()
	io.SetMouseButtonDown(0, true)
	frame()
	io.SetMouseButtonDown(0, false)
	frame()
	if ws.crew.dirty || len(ws.crew.jobs) != 1 || ws.crew.selected != 0 {
		t.Fatal("empty roster has no usable discard control")
	}
	if !ws.commitCrew() {
		t.Fatal("discard did not unblock navigation")
	}
}

func TestCrewDeleteConfirmationFitsWindow(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, test := range []struct {
		name      string
		size      imgui.Vec2
		job       string
		maxHeight float32
	}{
		{"ordinary", imgui.Vec2{X: 900, Y: 700}, "DJ", 180},
		{"long name", imgui.Vec2{X: 900, Y: 700}, strings.Repeat("DJ 100% ", 40), 668},
		{"small window", imgui.Vec2{X: 320, Y: 260}, "DJ", 228},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := imgui.CreateContext(nil)
			defer ctx.Destroy()
			io := imgui.CurrentIO()
			io.SetIniFilename("")
			io.SetDisplaySize(test.size)
			io.SetDeltaTime(1.0 / 60)
			io.Fonts().TextureDataRGBA32()
			ws := &WsShip{crew: crewEditor{jobs: []ship.CrewJob{{Name: test.job}}, selected: 0}}
			for i := 0; i < 4; i++ {
				imgui.NewFrame()
				imgui.SetNextWindowPos(imgui.Vec2{})
				imgui.SetNextWindowSize(test.size)
				imgui.BeginV("Delete confirmation sizing", nil, imgui.WindowFlagsNoSavedSettings)
				if i == 0 {
					imgui.OpenPopup("Delete crew job")
				}
				ws.crewJobActions()
				if !imgui.IsPopupOpen("Delete crew job") {
					t.Fatal("confirmation is not open")
				}
				imgui.End()
				imgui.Render()
			}
			lists := imgui.RenderedDrawData().CommandLists()
			if len(lists) < 2 {
				t.Fatal("confirmation did not render")
			}
			commands := lists[len(lists)-1].Commands()
			rect := commands[len(commands)-1].ClipRect()
			width, height := rect.Z-rect.X, rect.W-rect.Y
			if width < min(float32(400), test.size.X-50) || width > 460 || width > test.size.X-32 || height > test.maxHeight || height < 40 {
				t.Fatalf("confirmation is not compact: %+v", rect)
			}
			if rect.X < 0 || rect.Y < 0 || rect.Z > test.size.X || rect.W > test.size.Y {
				t.Fatalf("confirmation leaves the viewport: %+v", rect)
			}
		})
	}
}
