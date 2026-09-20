package app

import (
	"os"
	"path/filepath"
	"strings"
)

type startupArgs struct {
	project         string
	maps            []string
	sprites         []string
	shipWorkspace   bool
	ruinWorkspace   bool
	planetWorkspace bool
}

func parseStartupArgs(args []string) startupArgs {
	var parsed startupArgs
	for _, arg := range args {
		if arg == "--planet-workspace" {
			parsed.planetWorkspace = true
			continue
		}
		if arg == "--ruin-workspace" {
			parsed.ruinWorkspace = true
			continue
		}
		if arg == "--ship-workspace" {
			parsed.shipWorkspace = true
			continue
		}
		path := projectArgument(arg)
		switch strings.ToLower(filepath.Ext(path)) {
		case ".dme":
			parsed.project = path
		case ".dmm":
			parsed.maps = append(parsed.maps, path)
		case ".dmi", ".png":
			parsed.sprites = append(parsed.sprites, path)
		}
	}
	return parsed
}

func (a *app) checkProgramArgs() {
	args := parseStartupArgs(os.Args[1:])
	if args.project == "" && len(args.maps) > 0 {
		args.project, _ = findEnvironmentFileFromBase(args.maps[0])
	}
	if args.project != "" {
		a.loadEnvironmentV(args.project, func() {
			if args.planetWorkspace {
				a.DoOpenPlanetWorkspace()
			}
			if args.shipWorkspace {
				a.DoOpenShipWorkspace()
			}
			if args.ruinWorkspace {
				a.DoOpenRuinWorkspace()
			}
			for _, path := range args.maps {
				a.loadMap(path, nil)
			}
			for _, path := range args.sprites {
				a.openDMI(path, "", 2)
			}
		})
		return
	}
	for _, path := range args.maps {
		a.loadResource(path)
	}
	for _, path := range args.sprites {
		a.openDMI(path, "", 2)
	}
}
