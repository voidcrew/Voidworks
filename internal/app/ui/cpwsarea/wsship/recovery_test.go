package wsship

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/recovery"
	"sdmm/internal/ship"
	"sdmm/internal/util"
)

// Runs in the native workshop test with real panes and the disposable game fixture.
func exerciseRecovery(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	ws.task = taskPaint
	if !ws.Save() {
		t.Fatal(ws.message)
	}
	root := t.TempDir()
	ws.startRecovery(root)
	if ws.recovery.store == nil {
		t.Fatal(ws.recovery.error)
	}
	before, err := os.ReadFile(ws.app.LoadedEnvironment().RootFile)
	if err != nil {
		t.Fatal(err)
	}
	original := ws.project.Hull.Type
	hidden, err := ship.NewProject(ws.catalog, ws.app.LoadedEnvironment(), "recovery_hidden", "Hidden recovery draft", 12, 12)
	if err != nil {
		t.Fatal(err)
	}
	ws.projects[hidden.Hull.Type] = hidden
	ws.beginCrew()
	if len(ws.crew.jobs) == 0 {
		t.Fatal("recovery fixture has no crew")
	}
	ws.crew.jobs[0].Name = "" // Deliberately incomplete: normal Save must reject it.
	ws.crew.dirty = true
	file, err := ws.catalog.HullFile(ws.project.Hull, ws.currentTheme())
	if err != nil {
		t.Fatal(err)
	}
	ws.project.Documents[file].Map.GetTile(util.Point{X: 3, Y: 3, Z: 1}).InstancesAdd(dmmap.PrefabStorage.Initial("/obj/item/crowbar"))
	now := time.Now()
	ws.recovery.next = now
	ws.autosave(now)
	ws.finishAutosave(true)
	if ws.recovery.saved.IsZero() || ws.recovery.error != "" {
		t.Fatal("autosave failed", ws.recovery.error)
	}
	after, _ := os.ReadFile(ws.app.LoadedEnvironment().RootFile)
	if !bytes.Equal(before, after) {
		t.Fatal("autosave changed the game environment")
	}
	hiddenFile, _ := ws.catalog.HullFile(hidden.Hull, hidden.Hull.Themes[0])
	if _, err = os.Stat(hiddenFile); !os.IsNotExist(err) {
		t.Fatal("autosave wrote the hidden draft to the game")
	}
	if recovery.HasPending(root, ws.app.LoadedEnvironment().RootFile) {
		t.Fatal("live workspace offered for recovery")
	}
	// Abandon the workspace like a terminated process: release its OS lock without
	// calling the normal Save/Discard cleanup. Recreate every pane and undo stack.
	ws.recovery.store.Close()
	ws.OnFocusChange(false)
	for _, pane := range ws.panes {
		pane.Dispose()
	}
	ws.app.CommandStorage().DisposeStack(ws.CommandStackId())
	app := ws.app
	*ws = *New(app)
	ws.startRecovery(root)
	if len(ws.recovery.pending) != 1 {
		t.Fatal("no recovery offered on reopening")
	}
	for i := 0; i < 3; i++ {
		render()
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "recovery-prompt.png"), 1400, 960)
	}
	entry := ws.recovery.pending[0]
	if err = ws.restoreRecovery(entry); err != nil {
		t.Fatal(err)
	}
	ws.recovery.pending = nil
	if ws.project.Hull.Type != original || ws.projects[hidden.Hull.Type] == nil || ws.task != taskCrew || !ws.crew.dirty || ws.crew.jobs[0].Name != "" {
		t.Fatal("recovery lost current, hidden, or unfinished roster edits")
	}
	found := false
	for _, inst := range ws.project.Documents[file].Map.GetTile(util.Point{X: 3, Y: 3, Z: 1}).Instances() {
		if inst.Prefab().Path() == "/obj/item/crowbar" {
			found = true
		}
	}
	if !found {
		t.Fatal("map edit was not recovered")
	}
	if !ws.IsModified() {
		t.Fatal("recovery was marked saved")
	}
	for i := 0; i < 3; i++ {
		render()
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "recovery-roster.png"), 1400, 960)
	}
	if ws.Save() {
		t.Fatal("unfinished roster was silently committed")
	}
	// A failed save retains the recovery; correcting the draft allows a normal
	// Save of every recovered ship, then retires only this session's copies.
	ws.crew.jobs[0].Name = "Recovered captain"
	if !ws.Save() {
		t.Fatal(ws.message)
	}
	if _, err = os.Stat(hiddenFile); err != nil {
		t.Fatal("hidden recovered ship was not saved", err)
	}
	if ws.IsModified() {
		t.Fatal("saved recovery remains dirty")
	}
	store := ws.recovery.store
	ws.recovery.store = nil
	store.Close()
	if recovery.HasPending(root, app.LoadedEnvironment().RootFile) {
		t.Fatal("successful Save left stale recovery")
	}
	ws.recovery = recoveryState{}
}

func exerciseJeanShorts(t *testing.T, ws *WsShip, render func()) {
	t.Helper()
	const path = "/obj/item/clothing/under/shorts/jeanshorts"
	job := ship.CrewJob{Equipment: map[string]string{"uniform": path}}
	for _, dir := range []int{1, 2, 4, 8} {
		v := buildCrewVisual(ws.project, job, dir)
		if len(v.warnings) > 0 {
			t.Fatalf("jean shorts still fail in direction %d: %v", dir, v.warnings)
		}
		colors := map[uint32]bool{}
		for _, l := range v.layers {
			if l.layer == 28 {
				colors[l.color] = true
			}
		}
		for _, color := range []string{"#787878", "#723E0E", "#4D7EAC"} {
			if !colors[crewColor(color)] {
				t.Fatalf("missing jean-shorts color %s in direction %d", color, dir)
			}
		}
	}
	ws.beginCrew()
	if len(ws.crew.jobs) == 0 {
		ws.crew.jobs = []ship.CrewJob{{ID: "preview", Name: "Jean shorts preview", Category: "Assistant", Slots: 1}}
	}
	ws.crew.selected = 0
	ws.crew.jobs[0].Equipment = job.Equipment
	for i := 0; i < 3; i++ {
		render()
	}
	if dst := os.Getenv("SHIP_RENDER_TEST_OUTPUT"); dst != "" {
		captureFrame(t, filepath.Join(dst, "jean-shorts.png"), 1400, 960)
	}
	ws.loadCrewScope("ship")
	ws.task = taskPaint
}

func TestClosingWaitsForAutosaveThenDiscardsOnlyItsCopies(t *testing.T) {
	root := t.TempDir()
	first, err := recovery.Open(root, "fixture.dme")
	if err != nil {
		t.Fatal(err)
	}
	other, err := recovery.Open(root, "fixture.dme")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err = other.Save([]byte("another editor"), nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	app := &previewApp{}
	ws := &WsShip{app: app, recovery: recoveryState{store: first, writing: make(chan error, 1)}}
	result := ws.recovery.writing
	go func() {
		time.Sleep(20 * time.Millisecond)
		result <- first.Save([]byte("closing draft"), nil, time.Now())
	}()
	ws.Dispose()
	other.Close()
	entries, err := recovery.Pending(root, "fixture.dme")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		defer entry.Store.Close()
	}
	if len(entries) != 1 || string(entries[0].Snapshot.Data) != "another editor" {
		t.Fatal("closing raced with autosave or removed another session")
	}
}
