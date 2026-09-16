package ship

import (
	"fmt"

	"sdmm/internal/dmapi/dmenv"
)

// ReloadCatalog reads saved registrations when opening a new workshop session.
// A successful Save can add modules, variants and includes without reloading
// the editor's environment. Discover alone would still see its old definitions.
// Keep map types intact so ordinary map tabs and draft areas retain their state.
// Existing ship projects must be closed before calling this function.
func ReloadCatalog(dme *dmenv.Dme) (*Catalog, error) {
	if dme == nil {
		return nil, fmt.Errorf("load a Voidcrew environment to use the ship workspace")
	}
	fresh, err := dmenv.New(dme.RootFile)
	if err != nil {
		return nil, fmt.Errorf("refresh ship definitions: %w", err)
	}
	catalog, err := Discover(fresh)
	if err != nil {
		return nil, err
	}
	dme.RefreshTypeTrees(fresh, []string{HullType, "/datum/ship_theme", "/datum/ship_upgrade_module", "/datum/outfit"})
	return catalog, nil
}
