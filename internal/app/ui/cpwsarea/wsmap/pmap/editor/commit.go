package editor

import (
	"sdmm/internal/app/command"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/util"
)

func (e *Editor) CommitMapSizeChange(oldMaxX, oldMaxY, oldMaxZ int) {
	before := e.pMap.Snapshot().Initial().Copy()
	after := e.dmm.Copy()
	restore := func(saved dmmap.Dmm) {
		copy := saved.Copy()
		*e.dmm = copy
		e.onMapSizeChange(copy.MaxZ)
	}

	e.onMapSizeChange(e.dmm.MaxZ)

	e.app.CommandStorage().PushV(e.pMap.CommandStackId(), command.Make("Set Map Size", func() {
		restore(before)
	}, func() {
		restore(after)
	}))
}

func (e *Editor) onMapSizeChange(maxZ int) {
	tools.Selected().OnDeselect()
	// Ensure we are on the visible level.
	if e.pMap.ActiveLevel() > maxZ {
		e.pMap.SetActiveLevel(maxZ)
	}
	e.pMap.Snapshot().Sync() // Do a full snapshots sync.
	e.pMap.OnMapSizeChange()
	e.updateAreasZones()
	e.app.SyncVarEditor()
}

// CommitChanges triggers a snapshot to commit changes and create a patch between two map states.
func (e *Editor) CommitChanges(commitMsg string) {
	if e.pMap.InContext() {
		window.RunLater(func() { e.CommitContextNow(commitMsg) })
		return
	}
	go e.commitChanges(commitMsg)
}

// CommitContextNow is called on the UI thread before switching sources or saving.
// Copies make this history independent of structural multi-file snapshot resets.
func (e *Editor) CommitContextNow(name string) {
	if !e.pMap.InContext() {
		return
	}
	before := e.pMap.Snapshot().Initial().Copy()
	_, changed := e.pMap.Snapshot().Commit()
	if len(changed) == 0 {
		return
	}
	after := e.dmm.Copy()
	restore := func(m dmmap.Dmm) {
		e.pMap.BeforeHistory()
		copy := m.Copy()
		*e.dmm = copy
		e.pMap.Snapshot().Sync()
		e.pMap.CanvasState().SetMaxX(e.dmm.MaxX)
		e.pMap.CanvasState().SetMaxY(e.dmm.MaxY)
		e.updateAreasZones()
		e.pMap.Refresh(nil)
		e.app.SyncPrefabs()
		e.app.SyncVarEditor()
	}
	e.app.CommandStorage().PushV(e.pMap.CommandStackId(), command.Make(name+" - "+e.dmm.Name, func() { restore(before) }, func() { restore(after) }))
	e.updateAreasZones()
	e.pMap.Refresh(changed)
}

// Used as a wrapper to do a stuff inside the goroutine.
func (e *Editor) commitChanges(commitMsg string) {
	if e.pMap.InContext() {
		e.pMap.Refresh(nil)
	}
	stateId, tilesToUpdate := e.pMap.Snapshot().Commit()

	// Do not push command if there is no tiles to update.
	if len(tilesToUpdate) == 0 {
		return
	}

	// Copy the value to pass it to the lambda.
	activeLevel := e.pMap.ActiveLevel()

	// Ensure that the user has updated visuals.
	e.updateAreasZones()
	e.updateBucket(activeLevel, tilesToUpdate)

	window.RunLater(func() {
		e.app.CommandStorage().PushV(e.pMap.CommandStackId(), command.Make(commitMsg+" - "+e.dmm.Name, func() {
			e.pMap.BeforeHistory()
			e.pMap.Snapshot().GoTo(stateId - 1)
			e.updateAreasZones()
			e.updateBucket(activeLevel, tilesToUpdate)
			e.dmm.PersistPrefabs()
			e.app.SyncPrefabs()
			e.app.SyncVarEditor()
		}, func() {
			e.pMap.BeforeHistory()
			e.pMap.Snapshot().GoTo(stateId)
			e.updateAreasZones()
			e.updateBucket(activeLevel, tilesToUpdate)
			e.dmm.PersistPrefabs()
			e.app.SyncPrefabs()
			e.app.SyncVarEditor()
		}))
	})
}

// We need to update bucket in the main thread, since it can have OpenGL operations.
// RunLater do that by running the job in th end of the frame.
func (e *Editor) updateBucket(activeLevel int, tilesToUpdate []util.Point) {
	window.RunLater(func() {
		e.pMap.Refresh(tilesToUpdate)
	})
}
