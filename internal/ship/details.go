package ship

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
)

// ShipDetails keeps handwritten registrations separate from generated projects.
type ShipDetails struct {
	Description string
	Hidden      bool
	Crew        int
}

func (p *Project) ShipDetails() ShipDetails {
	if s := p.Settings; s != nil {
		return ShipDetails{s.Description, s.Hidden, s.Crew}
	}
	return ShipDetails{Description: p.Hull.Description, Hidden: p.Hull.Hidden}
}

func (p *Project) SetShipDetails(details ShipDetails) error {
	details.Description = strings.ReplaceAll(details.Description, "\r", "")
	if p.Settings != nil {
		if details.Crew < 1 || details.Crew > 32 {
			return fmt.Errorf("enter 1 to 32 crew")
		}
		p.Settings.Description, p.Settings.Hidden, p.Settings.Crew = details.Description, details.Hidden, details.Crew
		return nil
	}
	if details.Description == p.Hull.Description && details.Hidden == p.Hull.Hidden {
		return nil
	}
	if err := p.prepareShipDetails(); err != nil {
		return err
	}
	before := p.Hull
	p.Hull.Description, p.Hull.Hidden = details.Description, details.Hidden
	if _, err := p.Changes(); err != nil {
		p.Hull = before
		return err
	}
	return nil
}

func (p *Project) prepareShipDetails() error {
	if err := p.prepareRooms(nil); err != nil {
		return err
	}
	file, err := p.roomTypeFile(p.Hull.Type)
	if err != nil {
		return err
	}
	if err = p.roomSource(file); err != nil {
		return err
	}
	if p.rooms.details.file == "" {
		if obj := p.Dme.Objects[p.Hull.Type]; obj != nil {
			p.rooms.shortName = text(obj.Vars, "short_name")
		}
	}
	p.rooms.details = nameTarget{file, p.Hull.Type}
	portType := "/obj/docking_port/mobile/voidcrew/" + strings.TrimPrefix(p.Hull.Type, HullType+"/")
	if obj := p.Dme.Objects[portType]; obj != nil && obj.Location.File != "" {
		portFile, err := p.roomTypeFile(portType)
		if err != nil {
			return err
		}
		if err := p.roomSource(portFile); err != nil {
			return err
		}
		if p.rooms.portLabel.file == "" {
			p.rooms.portName = text(obj.Vars, "name")
		}
		p.rooms.portLabel = nameTarget{portFile, portType}
	}
	p.rooms.manifest = filepath.Join(p.Catalog.Root, "voidcrew/mapping/ship_projects", p.fileID()+".shipinfo.json")
	if err := p.roomSource(p.rooms.manifest); err != nil {
		return err
	}
	return nil
}

func (p *Project) applyShipDetails(contents map[string][]byte) error {
	target := p.rooms.details
	if target.file == "" {
		return nil
	}
	before, after := p.rooms.base, p.Hull
	data := contents[target.file]
	var err error
	if before.Description != after.Description {
		data, err = rewriteTextField(data, target.typePath, "catalog_desc", before.Description, after.Description)
		if err != nil {
			return err
		}
	}
	if before.Hidden != after.Hidden {
		data, err = rewriteFlag(data, target.typePath, "player_hidden", after.Hidden)
		if err != nil {
			return err
		}
	}
	if before.Name != after.Name {
		data, err = rewriteName(data, target.typePath, before.Name, after.Name)
		if err != nil {
			return err
		}
		if p.rooms.shortName != "" {
			data, err = rewriteTextField(data, target.typePath, "short_name", p.rooms.shortName, after.Name)
			if err != nil {
				return err
			}
		}
	}
	contents[target.file] = data
	if port := p.rooms.portLabel; before.Name != after.Name && port.file != "" {
		contents[port.file], err = rewriteName(contents[port.file], port.typePath, p.rooms.portName, after.Name)
		if err != nil {
			return err
		}
	}
	manifest := p.rooms.manifest
	if p.rooms.sources[manifest].Existed || !reflect.DeepEqual(before, after) {
		id, _ := p.roomID()
		contents[manifest], _ = json.MarshalIndent(Settings{Version: 2, ID: id, FileID: p.fileID(), Hull: after}, "", "  ")
		contents[manifest] = append(contents[manifest], '\n')
	} else {
		contents[manifest] = nil
	}
	return nil
}
