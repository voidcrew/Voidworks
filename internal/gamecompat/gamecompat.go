// Package gamecompat checks that the loaded Voidcrew code matches this editor.
package gamecompat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/ship"
)

// API is the version of the Voidcrew editor contract this build is made for.
// The game declares its own in File. Raise both together whenever the game
// changes files, variables or formats that Voidworks reads or writes.
const API = 1

// File is the game's declaration, relative to the project root.
const File = "tools/voidworks/compatibility.json"

const voidcrewShip = "/datum/map_template/shuttle/voidcrew"

type Status int

const (
	Compatible Status = iota
	// GameOutdated means the game code is older than this editor expects.
	GameOutdated
	// EditorOutdated means the game code needs a newer editor.
	EditorOutdated
)

type Result struct {
	Status Status
	// GameAPI is the game's declared version, or 0 when it declares none.
	GameAPI int
	// Missing names editor features the loaded game code does not support.
	Missing []string
}

// Check compares the loaded environment with this editor. Projects that are
// not Voidcrew, and current code that predates File, are compatible.
func Check(dme *dmenv.Dme) Result {
	if dme == nil || dme.Objects[voidcrewShip] == nil {
		return Result{}
	}
	result := Result{Missing: missingFeatures(dme)}
	declared, err := declaredAPI(dme.RootDir)
	if err != nil {
		result.Missing = append(result.Missing, err.Error())
	}
	result.GameAPI = declared
	switch {
	case declared > API:
		result.Status = EditorOutdated
	case declared != 0 && declared < API, len(result.Missing) > 0:
		result.Status = GameOutdated
	}
	return result
}

func declaredAPI(root string) (int, error) {
	data, err := os.ReadFile(filepath.Join(root, File))
	if os.IsNotExist(err) {
		return 0, nil // Older checkouts rely on the feature checks alone.
	}
	if err != nil {
		return 0, fmt.Errorf("a readable %s", File)
	}
	var declaration struct {
		API int `json:"voidworks_api"`
	}
	if json.Unmarshal(data, &declaration) != nil || declaration.API < 1 {
		return 0, fmt.Errorf("a valid voidworks_api in %s", File)
	}
	return declaration.API, nil
}

// Features the current editor relies on, newest last. Each is visible in the
// parsed code or checkout, so old code is caught even without File.
func missingFeatures(dme *dmenv.Dme) []string {
	var missing []string
	if !ship.SupportsFootprints(dme) {
		missing = append(missing, "room shapes other than rectangles")
	}
	if !ship.SupportsRoomCrewVariants(dme) {
		missing = append(missing, "separate room crew for each ship variant")
	}
	if !ship.SupportsSiliconCrew(dme) {
		missing = append(missing, "cyborg and AI ship crew")
	}
	previews := filepath.Join(dme.RootDir, "voidcrew/modules/ship_upgrades/previews")
	if _, err := os.Stat(previews); err == nil {
		if info, err := os.Stat(filepath.Join(previews, "hulls")); err != nil || !info.IsDir() {
			missing = append(missing, "purchase preview metadata folders")
		}
	}
	return missing
}
