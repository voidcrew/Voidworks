// shipcheck checks saved ship combinations using the same assembly code as the UI.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/ship"
)

type result struct {
	Hull, Theme, Module string
	Sources             int
	Issues              []ship.Issue `json:",omitempty"`
	Error               string       `json:",omitempty"`
}

func main() {
	dmePath := flag.String("dme", "", "path to the Voidcrew .dme")
	hullFilter := flag.String("hull", "", "optional hull type suffix")
	all := flag.Bool("all", false, "also check each module against the other default modules")
	flag.Parse()
	if *dmePath == "" {
		flag.Usage()
		os.Exit(2)
	}
	abs, err := filepath.Abs(*dmePath)
	if err != nil {
		fail(err)
	}
	dme, err := dmenv.New(abs)
	if err != nil {
		fail(err)
	}
	catalog, err := ship.Discover(dme)
	if err != nil {
		fail(err)
	}
	var results []result
	failed := false
	for _, h := range catalog.Hulls {
		if *hullFilter != "" && !strings.HasSuffix(h.Type, "/"+*hullFilter) {
			continue
		}
		project, err := ship.OpenProject(catalog, dme, h)
		if err == nil {
			err = project.ValidateSiliconCrew("", nil)
		}
		if err != nil {
			results = append(results, result{Hull: h.Name, Error: err.Error()})
			failed = true
			continue
		}
		themes := h.Themes
		if len(themes) == 0 {
			themes = []ship.Theme{{}}
		}
		for _, theme := range themes {
			selected := map[string]string{}
			for _, slot := range h.SlotsFor(theme) {
				for _, m := range h.Modules {
					if m.Slot == slot && m.Default && m.Available(theme.ID) {
						selected[slot] = m.ID
						break
					}
				}
			}
			check := func(module string) {
				r := result{Hull: h.Name, Theme: theme.ID, Module: module}
				a, err := catalog.Load(h, theme, selected)
				if err == nil {
					for _, atoms := range a.Cells {
						for _, atom := range atoms {
							if dme.Objects[atom.Prefab.Path()] == nil {
								err = fmt.Errorf("unknown map type %s", atom.Prefab.Path())
								break
							}
						}
						if err != nil {
							break
						}
					}
				}
				if err != nil {
					r.Error = err.Error()
					failed = true
				} else {
					a.CheckAccess(dme)
					r.Sources = len(a.Sources)
					r.Issues = a.Issues
				}
				results = append(results, r)
			}
			check("defaults")
			if *all {
				for _, m := range h.Modules {
					if !m.Available(theme.ID) || !ship.Contains(h.SlotsFor(theme), m.Slot) || selected[m.Slot] == m.ID {
						continue
					}
					previous := selected[m.Slot]
					selected[m.Slot] = m.ID
					check(m.ID)
					selected[m.Slot] = previous
				}
			}
		}
	}
	if len(results) == 0 {
		fail(fmt.Errorf("no matching hulls"))
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(results); err != nil {
		fail(err)
	}
	if failed {
		os.Exit(1)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
