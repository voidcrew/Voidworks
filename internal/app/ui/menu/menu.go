package menu

import (
	"sdmm/internal/app/command"
	"sdmm/internal/app/selfupdate"
	"sdmm/internal/app/ui/shortcut"
	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmclip"
	"sdmm/internal/env"
	"sdmm/internal/imguiext/icon"
	"sdmm/internal/imguiext/style"
	w "sdmm/internal/imguiext/widget"
	"sdmm/internal/platform"
	"sdmm/internal/rsc"
	"sdmm/internal/shippreview"

	"github.com/SpaiR/imgui-go"
)

//goland:noinspection GoCommentStart
type app interface {
	// File
	DoNewWorkspace()
	DoNewMap()
	DoOpenShipWorkspace()
	DoNewShip()
	DoOpenRuinWorkspace()
	DoOpenPlanetWorkspace()
	HasPlanetProject() bool
	DoNewRuin()
	DoOpenProject()
	DoOpen()
	DoLoadResource(path string)
	DoClearRecentMaps()
	DoCloseEnvironment()
	DoClose()
	DoCloseAll()
	DoSave()
	DoSaveAll()
	DoOpenPreferences()
	DoExit()

	// Edit
	DoUndo()
	DoRedo()
	DoCopy()
	DoPaste()
	DoCut()
	DoDelete()
	DoSearch()
	DoDeselect()
	DoOpenJumpWindow()

	// View
	DoAreaBorders()
	DoMultiZRendering()
	DoMirrorCanvasCamera()

	// Window
	DoResetLayout()

	// Help
	DoOpenChangelog()
	DoOpenAbout()
	DoOpenLogs()
	DoShipPreviews()
	ShipPreviewStatus() shippreview.Status
	DoOpenSourceCode()
	DoCheckForUpdates()
	DoSelectUpdateChannel(selfupdate.Channel)
	DoOpenUpdateDownload()
	DoOpenSupport()

	// Other
	DoSelfUpdate()
	DoRestart()
	DoIgnoreUpdate()

	// Helpers

	RecentMapsByLoadedEnvironment() []string
	RecentMaps() []string

	LoadedEnvironment() *dmenv.Dme
	HasLoadedEnvironment() bool
	HasVoidcrewProject() bool
	HasRuinProject() bool

	HasActiveMap() bool
	HasSaveableWorkspace() bool
	CanPaste() bool

	PathsFilter() *dm.PathsFilter
	CommandStorage() *command.Storage
	Clipboard() *dmmclip.Clipboard

	AreaBordersRendering() bool
	MultiZRendering() bool
	MirrorCanvasCamera() bool
}

type upStatus int

const (
	upStatusNone upStatus = iota
	upStatusAvailable
	upStatusUpdating
	upStatusUpdated
	upStatusError
	upStatusChecking
	upStatusCurrent
)

type Menu struct {
	app app

	shortcuts shortcut.Shortcuts

	updateStatus      upStatus
	updateVersion     string
	updateDescription string
	updateError       string
	updateOpen        bool
	updateChannel     selfupdate.Channel
}

func New(app app) *Menu {
	m := &Menu{app: app, updateChannel: selfupdate.CurrentChannel(env.Version)}
	m.addShortcuts()
	return m
}

func (m *Menu) Process() {
	w.MainMenuBar(w.Layout{
		w.Menu("File", w.Layout{
			w.Custom(func() {
				if app, ok := m.app.(interface{ DoNewDMI() }); ok {
					w.MenuItem("New DMI", app.DoNewDMI).IconEmpty().Build()
				}
				if app, ok := m.app.(interface{ DoRecoverDMI() }); ok {
					w.MenuItem("Recover DMI drafts...", app.DoRecoverDMI).IconEmpty().Build()
				}
			}),
			w.MenuItem("New Workspace", m.app.DoNewWorkspace).
				Icon(icon.File).
				Shortcut(platform.KeyModName(), "N"),
			w.MenuItem("New Map", m.app.DoNewMap).
				IconEmpty().
				Enabled(m.app.HasLoadedEnvironment()),
			w.Separator(),
			w.MenuItem("Open...", m.app.DoOpen).
				Icon(icon.FolderOpen).
				Shortcut(platform.KeyModName(), "O"),
			w.Menu("Recent Maps", w.Layout{
				w.Custom(func() {
					for _, recentMap := range m.app.RecentMaps() {
						w.MenuItem(recentMap, func() {
							m.app.DoLoadResource(recentMap)
						}).IconEmpty().Build()
					}
					w.Layout{
						w.Separator(),
						w.MenuItem("Clear Recent Maps", m.app.DoClearRecentMaps).
							Icon(icon.Delete),
					}.Build()
				}),
			}).Icon(icon.AccessTime).Enabled(len(m.app.RecentMaps()) != 0),
			w.MenuItem("Close Environment", m.app.DoCloseEnvironment).
				IconEmpty().
				Enabled(m.app.HasLoadedEnvironment()),
			w.Separator(),
			w.MenuItem("Close", m.app.DoClose).
				IconEmpty().
				Shortcut(platform.KeyModName(), "W"),
			w.MenuItem("Close All", m.app.DoCloseAll).
				IconEmpty().
				Shortcut(platform.KeyModName(), "Shift", "W"),
			w.Separator(),
			w.MenuItem("Save", m.app.DoSave).
				Icon(icon.Save).
				Enabled(m.app.HasSaveableWorkspace()).
				Shortcut(platform.KeyModName(), "S"),
			w.MenuItem("Save All", m.app.DoSaveAll).
				Icon(icon.Save).
				Enabled(m.app.HasSaveableWorkspace()).
				Shortcut(platform.KeyModName(), "Shift", "S"),
			w.Separator(),
			w.MenuItem("Preferences", m.app.DoOpenPreferences).
				Icon(icon.Wrench),
			w.Separator(),
			w.MenuItem("Exit", m.app.DoExit).
				IconEmpty().
				Shortcut(shortcut.Combine(platform.KeyModName(), "Q")),
		}),

		w.Menu("Voidcrew", w.Layout{
			w.MenuItem("Open Project...", m.app.DoOpenProject).Icon(icon.FolderOpen),
			w.Separator(),
			w.MenuItem("Ship Workshop", m.app.DoOpenShipWorkspace).IconEmpty().Enabled(m.app.HasVoidcrewProject()),
			w.MenuItem("Planet Workshop", m.app.DoOpenPlanetWorkspace).IconEmpty().Enabled(m.app.HasPlanetProject()),
			w.MenuItem("New Ship...", m.app.DoNewShip).IconEmpty().Enabled(m.app.HasVoidcrewProject()),
			w.Separator(),
			w.MenuItem("Ruin Workshop", m.app.DoOpenRuinWorkspace).IconEmpty().Enabled(m.app.HasRuinProject()),
			w.MenuItem("New Ruin...", m.app.DoNewRuin).IconEmpty().Enabled(m.app.HasRuinProject()),
		}),

		w.Menu("Edit", w.Layout{
			w.MenuItem("Undo", m.app.DoUndo).
				Icon(icon.Undo).
				Enabled(m.app.CommandStorage().HasUndo()).
				Shortcut(platform.KeyModName(), "Z"),
			w.MenuItem("Redo", m.app.DoRedo).
				Icon(icon.Redo).
				Enabled(m.app.CommandStorage().HasRedo()).
				Shortcut(platform.KeyModName(), "Shift", "Z"),
			w.Separator(),
			w.MenuItem("Copy", m.app.DoCopy).
				Icon(icon.ContentCopy).
				Shortcut(platform.KeyModName(), "C"),
			w.MenuItem("Paste", m.app.DoPaste).
				Icon(icon.ContentPaste).
				Enabled(m.app.CanPaste()).
				Shortcut(platform.KeyModName(), "V"),
			w.MenuItem("Cut", m.app.DoCut).
				Icon(icon.ContentCut).
				Shortcut(platform.KeyModName(), "X"),
			w.MenuItem("Delete", m.app.DoDelete).
				Icon(icon.Eraser).
				Shortcut("Delete"),
			w.MenuItem("Deselect", m.app.DoDeselect).
				IconEmpty().
				Shortcut(platform.KeyModName(), "D"),
			w.Separator(),
			w.MenuItem("Search", m.app.DoSearch).
				Icon(icon.Search).
				Enabled(m.app.HasActiveMap()).
				Shortcut(platform.KeyModName(), "F"),
			w.MenuItem("Go to Coords", m.app.DoOpenJumpWindow).
				Icon(icon.Shrink).
				Enabled(m.app.HasActiveMap()).
				Shortcut(platform.KeyModName(), "G"),
		}),

		w.Menu("View", w.Layout{
			w.MenuItem("Show Area", m.doToggleArea).
				IconEmpty().
				Enabled(m.app.HasLoadedEnvironment()).
				Selected(m.isAreaToggled()).
				Shortcut(platform.KeyModName(), "1"),
			w.MenuItem("Show Turf", m.doToggleTurf).
				IconEmpty().
				Enabled(m.app.HasLoadedEnvironment()).
				Selected(m.isTurfToggled()).
				Shortcut(platform.KeyModName(), "2"),
			w.MenuItem("Show Object", m.doToggleObject).
				IconEmpty().
				Enabled(m.app.HasLoadedEnvironment()).
				Selected(m.isObjectToggled()).
				Shortcut(platform.KeyModName(), "3"),
			w.MenuItem("Show Mob", m.doToggleMob).
				IconEmpty().
				Enabled(m.app.HasLoadedEnvironment()).
				Selected(m.isMobToggled()).
				Shortcut(platform.KeyModName(), "4"),
			w.MenuItem("Show All", m.doShowAll).
				IconEmpty().
				Enabled(m.app.HasLoadedEnvironment()),
			w.Separator(),
			w.MenuItem("Area Borders", m.app.DoAreaBorders).
				IconEmpty().
				Selected(m.app.AreaBordersRendering()),
			w.MenuItem("Multi-Z Rendering", m.app.DoMultiZRendering).
				IconEmpty().
				Selected(m.app.MultiZRendering()).
				Shortcut(platform.KeyModName(), "0"),
			w.MenuItem("Mirror Canvas Camera", m.app.DoMirrorCanvasCamera).
				IconEmpty().
				Selected(m.app.MirrorCanvasCamera()),
		}),

		w.Menu("Window", w.Layout{
			w.MenuItem("Reset Layout", m.app.DoResetLayout).Shortcut("F5").
				Icon(icon.WindowRestore),
		}),

		w.Menu("Help", w.Layout{
			w.MenuItem("Changelog", m.app.DoOpenChangelog).
				Icon(icon.ClipboardMultiple),
			w.MenuItem("Source Code", m.app.DoOpenSourceCode).
				Icon(icon.GitHub),
			w.MenuItem("Updates & Release Channel", m.app.DoCheckForUpdates).
				Enabled(selfupdate.Supported()).
				Icon(icon.SystemUpdate),
			w.Separator(),
			w.MenuItem("Open Logs Folder", m.app.DoOpenLogs).
				IconEmpty(),
			w.MenuItem("Ship Purchase Previews", m.app.DoShipPreviews).
				Enabled(m.app.HasVoidcrewProject()).IconEmpty(),
			w.MenuItem("About", m.app.DoOpenAbout).
				IconEmpty(),
			w.Separator(),
			w.Button("Support Upstream", m.app.DoOpenSupport).
				Size(imgui.Vec2{X: -1}).
				Style(style.ButtonFireCoral{}).
				Tooltip(rsc.SupportTxt).
				Icon(icon.KoFi),
		}),

		w.Custom(func() {
			status := m.app.ShipPreviewStatus()
			if status.Phase != "" && status.Phase != "complete" {
				label := "Generating previews..."
				if status.Phase == "failed" || status.Phase == "error" {
					label = "Previews need attention"
				} else if status.Phase == "stopped" {
					label = "Previews paused"
				} else if status.Phase == "stopping" {
					label = "Stopping previews..."
				}
				if imgui.Button(label) {
					m.app.DoShipPreviews()
				}
			}
			if m.updateStatus != upStatusNone {
				m.showUpdateMenu()
			}
		}),
	}).Build()
}

func (m *Menu) SetUpdateAvailable(version, description string) {
	m.updateStatus = upStatusAvailable
	m.updateVersion = version
	m.updateDescription = description
	m.updateError = ""
}

func (m *Menu) SetUpdating() {
	m.updateStatus = upStatusUpdating
	m.updateError = ""
}

func (m *Menu) SetUpdated() {
	m.updateStatus = upStatusUpdated
}

func (m *Menu) SetUpdateError(message string) {
	m.updateStatus = upStatusError
	m.updateError = message
}

func (m *Menu) doToggleArea() {
	m.app.PathsFilter().TogglePath("/area")
}

func (m *Menu) doToggleTurf() {
	m.app.PathsFilter().TogglePath("/turf")
}

func (m *Menu) doToggleObject() {
	m.app.PathsFilter().TogglePath("/obj")
}

func (m *Menu) doToggleMob() {
	m.app.PathsFilter().TogglePath("/mob")
}

func (m *Menu) doShowAll() {
	m.app.PathsFilter().Clear()
}

func (m *Menu) isAreaToggled() bool {
	return m.app.PathsFilter().IsVisiblePath("/area")
}

func (m *Menu) isTurfToggled() bool {
	return m.app.PathsFilter().IsVisiblePath("/turf")
}

func (m *Menu) isObjectToggled() bool {
	return m.app.PathsFilter().IsVisiblePath("/obj")
}

func (m *Menu) isMobToggled() bool {
	return m.app.PathsFilter().IsVisiblePath("/mob")
}
