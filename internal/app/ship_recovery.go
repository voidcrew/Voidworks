package app

import (
	"path/filepath"
	"sdmm/internal/recovery"
)

func (a *app) ShipRecoveryDirectory() string {
	return filepath.Join(a.internalDir, "recovery", "ships")
}
func (a *app) offerShipRecovery() {
	if a.loadedEnvironment != nil && recovery.HasPending(a.ShipRecoveryDirectory(), a.loadedEnvironment.RootFile) {
		a.layout.WsArea.OpenShip()
	}
}
