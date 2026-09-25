package app

import (
	"context"
	"os"
	"sync"

	"sdmm/internal/app/selfupdate"
	"sdmm/internal/app/ui/cpwsarea/wsmap"
	"sdmm/internal/app/ui/cpwsarea/wsplanet"
	"sdmm/internal/app/ui/cpwsarea/wsruin"
	"sdmm/internal/app/ui/cpwsarea/wsship"
	"sdmm/internal/app/window"
	"sdmm/internal/env"
	"sdmm/internal/startup"
	"sdmm/internal/util/slice"

	"github.com/rs/zerolog/log"
)

type updateState struct {
	ctx              context.Context
	cancel           context.CancelFunc
	workers          sync.WaitGroup
	busy             bool
	checking         bool
	manualCheck      bool
	ready            bool
	restartRequested bool
	restarting       bool
	release          selfupdate.Release
	// Written by the download worker; read on the UI thread only after its
	// completion callback or after workers.Wait during shutdown.
	downloaded *selfupdate.Staged
	channel    selfupdate.Channel
}

func (a *app) checkForUpdatesV(manual bool) {
	a.checkForUpdatesOnChannel(manual, a.selectedUpdateChannel())
}

func (a *app) selectedUpdateChannel() selfupdate.Channel {
	if a.updates.channel.Valid() {
		return a.updates.channel
	}
	return selfupdate.CurrentChannel(env.Version)
}

func (a *app) DoSelectUpdateChannel(channel selfupdate.Channel) {
	if !channel.Valid() || a.updates.busy || a.updates.restartRequested {
		return
	}
	if a.updates.ready {
		executable, err := os.Executable()
		if err == nil {
			err = a.updates.downloaded.Discard(executable)
		}
		if err != nil {
			a.menu.SetRestartError(err.Error())
			return
		}
		a.updates.downloaded = nil
		a.updates.ready = false
	}
	a.checkForUpdatesOnChannel(true, channel)
}

func (a *app) checkForUpdatesOnChannel(manual bool, channel selfupdate.Channel) {
	if a.updates.busy || a.updates.ready {
		if manual {
			if a.updates.checking {
				a.updates.manualCheck = true
				a.menu.SetChecking()
			}
			a.menu.ShowUpdatePopup()
		}
		return
	}
	a.updates.channel = channel
	a.updates.release = selfupdate.Release{}
	a.menu.SetUpdateChannel(channel)
	a.updates.busy = true
	a.updates.checking = true
	a.updates.manualCheck = manual
	if manual {
		a.menu.SetChecking()
		a.menu.ShowUpdatePopup()
	}
	a.updates.workers.Add(1)
	go func() {
		defer a.updates.workers.Done()
		release, err := selfupdate.CheckChannel(a.updates.ctx, env.Version, channel)
		window.RunLater(func() {
			a.updates.busy = false
			a.updates.checking = false
			manual := a.updates.manualCheck
			if a.closed {
				return
			}
			if err != nil {
				log.Printf("update check failed: %v", err)
				if manual {
					a.menu.SetUpdateError(err.Error())
				}
				return
			}
			a.updates.release = release
			if release.Version == "" {
				if manual {
					a.menu.SetUpToDate(env.Version)
					a.menu.SetUpdateNotice(release.Notice)
				}
				return
			}
			if !manual && slice.StrContains(a.config().UpdateIgnore, release.Version) {
				return
			}
			a.menu.SetUpdateAvailable(release.Version, release.Description)
			a.menu.SetUpdateNotice(release.Notice)
			if a.Prefs().Application.AutoUpdate && release.SwitchFrom == "" {
				a.selfUpdate()
			}
		})
	}()
}

func (a *app) selfUpdate() {
	if a.updates.busy || a.updates.ready || a.updates.release.Version == "" {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		a.menu.SetUpdateError(err.Error())
		return
	}
	a.updates.busy = true
	a.menu.SetUpdating()
	release := a.updates.release
	a.updates.workers.Add(1)
	go func() {
		defer a.updates.workers.Done()
		staged, err := selfupdate.Stage(a.updates.ctx, release, executable)
		a.updates.downloaded = staged
		window.RunLater(func() {
			a.updates.busy = false
			if a.closed {
				return
			}
			if err != nil {
				log.Printf("update download failed: %v", err)
				a.menu.SetUpdateError(err.Error())
				return
			}
			a.updates.ready = true
			a.menu.SetUpdated()
		})
	}()
}

func (a *app) restartForUpdate() {
	if !a.updates.ready || a.closing {
		return
	}
	if !startup.CanRestartForUpdate() {
		a.menu.SetRestartError("Restart Voidworks.exe normally before applying updates.")
		return
	}
	a.updates.restartRequested = true
	a.tmpShouldClose = true
}

func (a *app) updateRestartArgs() []string {
	var args []string
	if a.loadedEnvironment != nil {
		args = append(args, a.loadedEnvironment.RootFile)
	}
	for _, ws := range a.layout.WsArea.MapWorkspaces() {
		switch content := ws.Content().(type) {
		case *wsplanet.Workspace:
			args = append(args, "--planet-workspace")
		case *wsship.WsShip:
			args = append(args, "--ship-workspace")
		case *wsruin.WsRuin:
			args = append(args, "--ruin-workspace")
		case *wsmap.WsMap:
			path := content.Map().Dmm().Path.Absolute
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				args = append(args, path)
			}
		}
	}
	return args
}

func (a *app) disposeUpdates() {
	if a.updates.cancel == nil {
		return
	}
	a.updates.cancel()
	a.updates.workers.Wait()
	if staged := a.updates.downloaded; staged != nil && !a.updates.restarting {
		if executable, err := os.Executable(); err == nil {
			if err := staged.Discard(executable); err != nil {
				log.Printf("remove unused update: %v", err)
			}
		}
	}
}
