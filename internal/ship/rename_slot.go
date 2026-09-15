package ship

import (
	"fmt"
	"strings"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
)

func roomName(name string) string {
	return SlotDisplayName(strings.Join(strings.Fields(strings.ReplaceAll(name, "_", " ")), " "))
}

func renamedSlotKey(slot, name string) string {
	name = roomName(name)
	if name == roomName(SlotDisplayName(slot)) {
		return slot
	}
	// Slot keys are quoted strings, not DM identifiers. Keep the entered case
	// and punctuation so the editor and purchase screen show the same name.
	return strings.ReplaceAll(name, " ", "_")
}

func (p *Project) RenameSlotNameError(slot, name string) error {
	if !p.slotIDUsed(slot) {
		return fmt.Errorf("room no longer exists")
	}
	existing := []string{"Hull"}
	for other := range p.Hull.RoomSlots() {
		if other != slot {
			existing = append(existing, SlotDisplayName(other))
		}
	}
	return uniqueName(roomName(name), existing)
}

// RoomSlots includes rooms disabled in some variants and rooms used only by
// an option, so renames cannot collide with a currently hidden room.
func (h Hull) RoomSlots() map[string]bool {
	slots := map[string]bool{}
	for _, s := range h.Slots {
		slots[s] = true
	}
	for _, t := range h.Themes {
		for _, s := range t.Slots {
			slots[s] = true
		}
	}
	for _, m := range h.Modules {
		slots[m.Slot] = true
	}
	return slots
}

// PrepareSlotRename loads every affected hull before the caller captures an
// undo state. Source baselines are retained for Save and crash recovery.
func (p *Project) PrepareSlotRename(slot, name string) error {
	if err := p.RenameSlotNameError(slot, name); err != nil {
		return err
	}
	next := renamedSlotKey(slot, name)
	if next == slot {
		return nil
	}
	hulls, err := p.hullMaps()
	if err != nil {
		return err
	}
	for _, h := range hulls {
		for _, tile := range h.doc.Map.Tiles {
			for _, i := range tile.Instances() {
				path := i.Prefab().Path()
				if path != SlotMarker && !strings.HasPrefix(path, SlotMarker+"/") {
					continue
				}
				key := text(i.Prefab().Vars(), "key")
				if key != slot && nameKey(roomName(key)) == nameKey(roomName(next)) {
					return fmt.Errorf("the name %q is already used by a hull room", roomName(name))
				}
			}
		}
	}
	if p.Settings != nil {
		return nil
	}
	if err = p.prepareRooms(nil); err != nil {
		return err
	}
	if Contains(p.Hull.Slots, slot) {
		if err = p.prepareRooms(&Theme{}); err != nil {
			return err
		}
	}
	for _, t := range p.Hull.Themes {
		if Contains(t.Slots, slot) && p.inBaseThemes(t.ID) {
			if err = p.prepareRooms(&t); err != nil {
				return err
			}
		}
	}
	if p.rooms.moduleSlots == nil {
		p.rooms.moduleSlots = map[string]nameTarget{}
	}
	for _, original := range p.rooms.base.Modules {
		i := p.moduleIndex(original.ID)
		if i < 0 || p.Hull.Modules[i].Slot != slot {
			continue
		}
		if _, ok := p.rooms.moduleSlots[original.ID]; ok {
			continue
		}
		typePath, err := p.componentType("/datum/ship_upgrade_module/", original.ID)
		if err != nil {
			return err
		}
		file, err := p.roomTypeFile(typePath)
		if err != nil {
			return err
		}
		if err = p.roomSource(file); err != nil {
			return err
		}
		if _, err = rewriteTextField(p.rooms.sources[file].Before, typePath, "slot", original.Slot, next); err != nil {
			return err
		}
		p.rooms.moduleSlots[original.ID] = nameTarget{file, typePath}
	}
	return nil
}

// RenameSlot changes a room in every variant, including hidden options and
// hull markers. Option IDs, names, maps, prices and crew remain attached.
func (p *Project) RenameSlot(slot, name string) (string, error) {
	if err := p.PrepareSlotRename(slot, name); err != nil {
		return "", err
	}
	next := renamedSlotKey(slot, name)
	if next == slot {
		return slot, nil
	}
	hulls, err := p.hullMaps()
	if err != nil {
		return "", err
	}
	for _, h := range hulls {
		for _, tile := range h.doc.Map.Tiles {
			for _, i := range tile.Instances() {
				f := i.Prefab()
				if (f.Path() == SlotMarker || strings.HasPrefix(f.Path(), SlotMarker+"/")) && text(f.Vars(), "key") == slot {
					i.SetPrefab(dmmap.PrefabStorage.Put(dmmprefab.New(0, f.Path(), dmvars.Set(f.Vars(), "key", dmQuote(next)))))
				}
			}
		}
		p.reserveAnchors(h.doc.Map)
		p.protect(h.doc.Map)
	}
	replace := func(values []string) {
		for i, value := range values {
			if value == slot {
				values[i] = next
			}
		}
	}
	replace(p.Hull.Slots)
	for i := range p.Hull.Themes {
		replace(p.Hull.Themes[i].Slots)
	}
	for i := range p.Hull.Modules {
		if p.Hull.Modules[i].Slot == slot {
			p.Hull.Modules[i].Slot = next
		}
	}
	return next, nil
}
