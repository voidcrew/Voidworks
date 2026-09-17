package ship

import (
	"fmt"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/util"
)

// A hull may be transparent below its rooms. Check every available option,
// including ones not selected in the preview, before cropping that space.
func (p *Project) checkResizeRooms(hull *dmmap.Dmm, theme Theme, w, h int, offset util.Point) error {
	for _, module := range p.Hull.Modules {
		if !module.Available(theme.ID) || !Contains(p.Hull.SlotsFor(theme), module.Slot) {
			continue
		}
		marker, _, instance := slotMarker(hull, module.Slot)
		if instance == nil {
			continue
		}
		file, err := p.moduleFile(module, theme.ID)
		if err != nil {
			return err
		}
		doc, err := p.document(file)
		if err != nil {
			return err
		}
		shift := marker.Minus(connectorAt(doc.Map)).Plus(offset)
		for _, tile := range doc.Map.Tiles {
			next := tile.Coord.Plus(shift)
			if next.X >= 1 && next.Y >= 1 && next.X <= w && next.Y <= h {
				continue
			}
			for _, instance := range tile.Instances() {
				if path := instance.Prefab().Path(); !mappingMarker(path) && !isNoop(path) {
					return fmt.Errorf("resize would remove content from %s", module.Name)
				}
			}
		}
	}
	return nil
}
