package app

import (
	"fmt"
	"os"

	"github.com/SpaiR/imgui-go"
	"github.com/skratchdot/open-golang/open"
	"sdmm/internal/app/ui/dialog"
	"sdmm/internal/app/ui/workshop"
	"sdmm/internal/app/window"
	w "sdmm/internal/imguiext/widget"
	"sdmm/internal/shippreview"
)

// ShipFilesSaved refreshes previews for the saved maps only.
func (a *app) ShipFilesSaved(paths ...string) {
	dme := a.LoadedEnvironment()
	if dme != nil {
		a.previews.RequestSaved(dme.RootDir, dme.RootFile, paths...)
	}
}

func (a *app) MapFileSaved(path string) {
	if dme := a.LoadedEnvironment(); dme != nil && shippreview.IsShipMap(dme.RootDir, path) {
		a.ShipFilesSaved(path)
	}
}

func (a *app) ShipPreviewStatus() shippreview.Status {
	if dme := a.LoadedEnvironment(); dme != nil {
		return a.previews.Status(dme.RootDir)
	}
	return shippreview.Status{}
}

func (a *app) StopShipPreviews() {
	if dme := a.LoadedEnvironment(); dme != nil {
		a.previews.Stop(dme.RootDir, dme.RootFile)
	}
}

func (a *app) ResumeShipPreviews() {
	if dme := a.LoadedEnvironment(); dme != nil {
		a.previews.Resume(dme.RootDir, dme.RootFile)
	}
}

func (a *app) DoShipPreviews() {
	var plan *shippreview.CleanupPlan
	var selected []bool
	var cleanupError string
	type scanResult struct {
		plan *shippreview.CleanupPlan
		err  error
	}
	var scanning <-chan scanResult
	project := ""
	if dme := a.LoadedEnvironment(); dme != nil {
		project = dme.RootDir
	}
	dialog.Open(dialog.TypeCustom{Title: "Ship purchase previews", CloseButton: true, Layout: w.Layout{
		w.Custom(func() {
			width := min(540*window.PointSize(), max(1, imgui.MainViewport().WorkSize().X-70*window.PointSize()))
			height := min(620*window.PointSize(), max(1, imgui.MainViewport().WorkSize().Y-90*window.PointSize()))
			imgui.BeginChildV("ship-preview-settings", imgui.Vec2{X: width, Y: height}, false, 0)
			defer imgui.EndChild()
			imgui.PushTextWrapPosV(0)
			defer imgui.PopTextWrapPos()
			imgui.TextWrapped("Ship saves refresh the purchase previews of the maps you saved, plus any missing previews, in the background. You can keep editing or close the editor while they finish.")
			workshop.Gap()
			status := a.ShipPreviewStatus()
			workshop.PreviewStatus(status, a.ResumeShipPreviews, a.StopShipPreviews)
			if status.Phase == "" {
				imgui.TextWrapped("No preview generation has run for this project yet.")
			}
			if status.Phase == "" || status.Phase == "complete" || status.Phase == "stopped" {
				imgui.TextWrapped("Saves only refresh the maps you saved. If other ships' maps were changed without new previews, refresh the outdated previews on purpose and commit them separately.")
				if imgui.Button("Refresh outdated previews") {
					if dme := a.LoadedEnvironment(); dme != nil {
						a.previews.RefreshOutdated(dme.RootDir, dme.RootFile)
					}
				}
				imgui.TextWrapped("After changing icons or rendering code, rebuild all previews to refresh unchanged maps too.")
				if imgui.Button("Rebuild all previews") {
					if dme := a.LoadedEnvironment(); dme != nil {
						a.previews.RequestFull(dme.RootDir, dme.RootFile)
					}
				}
			}
			workshop.Gap()
			imgui.Separator()
			imgui.TextWrapped("Unused preview cleanup")
			imgui.TextWrapped("Save your ship changes first. Unchanged previews made obsolete by a save are backed up automatically. Review older leftovers across the project here.")
			select {
			case result := <-scanning:
				scanning = nil
				plan = result.plan
				if result.err != nil {
					cleanupError = result.err.Error()
				} else {
					selected = make([]bool, len(plan.Files))
					for i := range selected {
						selected[i] = true
					}
				}
			default:
			}
			busy := status.Phase == "running" || status.Phase == "starting" || status.Phase == "stopping" || scanning != nil
			imgui.BeginDisabledV(busy || project == "")
			if imgui.Button("Review unused previews") {
				plan, cleanupError = nil, ""
				result := make(chan scanResult, 1)
				scanning = result
				go func() {
					plan, err := a.previews.ScanCleanup(project)
					result <- scanResult{plan, err}
				}()
			}
			imgui.EndDisabled()
			if scanning != nil {
				imgui.TextWrapped("Checking the preview manifest and image files...")
			}
			if plan != nil {
				imgui.TextWrapped(fmt.Sprintf("%d PNG files are not listed in the current preview manifest.", len(plan.Files)))
				if len(plan.Files) > 0 {
					imgui.TextWrapped(plan.Directory)
					imgui.TextWrapped("Uncheck anything you want to keep. Maps, code, subfolders and linked files are excluded. Current previews are checked again before cleanup; changed files are kept.")
					imgui.BeginChildV("unused-preview-files", imgui.Vec2{Y: 150 * window.PointSize()}, true, 0)
					count := 0
					for i, file := range plan.Files {
						imgui.PushIDInt(i)
						imgui.Checkbox("##selected", &selected[i])
						imgui.SameLine()
						imgui.Text(file.Name)
						imgui.PopID()
						if selected[i] {
							count++
						}
					}
					imgui.EndChild()
					imgui.BeginDisabledV(busy || count == 0)
					if imgui.Button(fmt.Sprintf("Move %d selected previews to backup", count)) {
						if dme := a.LoadedEnvironment(); dme != nil && dme.RootDir == project {
							approved := *plan
							approved.Files = nil
							for i, file := range plan.Files {
								if selected[i] {
									approved.Files = append(approved.Files, file)
								}
							}
							if err := a.previews.RequestCleanup(project, dme.RootFile, approved); err != nil {
								cleanupError = err.Error()
							} else {
								plan = nil
							}
							busy = true
						}
					}
					imgui.EndDisabled()
				}
			}
			if cleanupError != "" {
				imgui.TextWrapped(cleanupError)
			}
			imgui.TextWrapped("Cleanup backups are kept outside the game project until you remove them. To restore, copy the PNGs back to the previews folder. This cannot be undone with Ctrl+Z.")
			if backup := a.previews.CleanupBackups(project); project != "" {
				if _, err := os.Stat(backup); err == nil && imgui.Button("Open cleanup backups") {
					_ = open.Run(backup)
				}
			}
			workshop.Gap()
			if shippreview.Bundled() {
				imgui.TextWrapped("Preview tools are included. No additional installation is needed.")
			} else {
				imgui.TextWrapped("This source build requires Python 3.10 or newer, Pillow, and dmm-tools. The Windows release includes these tools.")
			}
		}),
	}})
}
