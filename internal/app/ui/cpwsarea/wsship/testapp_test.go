package wsship

import (
	"sdmm/internal/app/command"
	"sdmm/internal/app/config"
	"sdmm/internal/app/prefs"
	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/dmapi/dmmclip"
	"sdmm/internal/shippreview"
)

type previewApp struct {
	dme             *dmenv.Dme
	commands        *command.Storage
	filter          *dm.PathsFilter
	clipboard       dmmclip.Clipboard
	prefab          *dmmprefab.Prefab
	instance        *dmminstance.Instance
	previewRequests int
	previewStatus   shippreview.Status
}

func (a *previewApp) ShipFilesSaved() { a.previewRequests++ }

func (a *previewApp) StopShipPreviews() {
	a.previewStatus = shippreview.Status{Phase: "stopped", Message: "Preview generation is paused. Your ship saves are safe. Resume when ready; completed renders will be reused."}
}

func (a *previewApp) ResumeShipPreviews() { a.previewRequests++ }

func (a *previewApp) DoPreviewMap(*dmmap.Dmm) {}

func (a *previewApp) ShipPreviewStatus() shippreview.Status { return a.previewStatus }

func (a *previewApp) LoadedEnvironment() *dmenv.Dme { return a.dme }
func (a *previewApp) CommandStorage() *command.Storage {
	if a.commands == nil {
		a.commands = command.NewStorage()
	}
	return a.commands
}
func (a *previewApp) PathsFilter() *dm.PathsFilter {
	if a.filter == nil {
		a.filter = dm.NewPathsFilterEmpty()
	}
	return a.filter
}
func (a *previewApp) Clipboard() *dmmclip.Clipboard             { return &a.clipboard }
func (a *previewApp) Prefs() prefs.Prefs                        { return prefs.Prefs{} }
func (a *previewApp) SelectedPrefab() (*dmmprefab.Prefab, bool) { return a.prefab, a.prefab != nil }
func (a *previewApp) SelectedInstance() (*dmminstance.Instance, bool) {
	return a.instance, a.instance != nil
}
func (a *previewApp) HasSelectedPrefab() bool                     { return a.prefab != nil }
func (a *previewApp) HasSelectedInstance() bool                   { return a.instance != nil }
func (a *previewApp) DoSelectPrefab(p *dmmprefab.Prefab)          { a.prefab = p }
func (a *previewApp) DoEditInstance(i *dmminstance.Instance)      { a.instance = i }
func (a *previewApp) AddMouseChangeCallback(func(uint, uint)) int { return 0 }
func (a *previewApp) RemoveMouseChangeCallback(int)               {}
func (a *previewApp) OnWorkspaceSwitched()                        {}
func (a *previewApp) ConfigRegister(config.Config)                {}
func (a *previewApp) ShowLayout(string, bool)                     {}
func (a *previewApp) SyncPrefabs()                                {}
func (a *previewApp) SyncVarEditor()                              {}
func (a *previewApp) DoSearchPrefab(uint64)                       {}
func (a *previewApp) DoSearchPrefabByPath(string)                 {}
func (a *previewApp) DoUndo()                                     { a.CommandStorage().Undo() }
func (a *previewApp) DoRedo()                                     { a.CommandStorage().Redo() }
func (a *previewApp) DoCopy()                                     {}
func (a *previewApp) DoCut()                                      {}
func (a *previewApp) DoPaste()                                    {}
func (a *previewApp) DoDelete()                                   {}
