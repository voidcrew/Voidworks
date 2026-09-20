package app

import "sdmm/internal/ship"

type crewWorkspace interface {
	EditingCrew() bool
	CopyCrewJob() (ship.CrewJob, bool)
	PasteCrewJob(ship.CrewJob) bool
}

func (a *app) activeCrewWorkspace() crewWorkspace {
	if ws := a.layout.WsArea.ActiveWorkspace(); ws != nil {
		if crew, ok := ws.Content().(crewWorkspace); ok && crew.EditingCrew() {
			return crew
		}
	}
	return nil
}

func (a *app) CanPaste() bool {
	if a.activeSprite() != nil {
		return true
	}
	if a.activeCrewWorkspace() != nil {
		return a.crewClipboard != nil
	}
	return a.clipboard.HasData()
}
