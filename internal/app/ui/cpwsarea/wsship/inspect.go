package wsship

import "sdmm/internal/dmapi/dmmap/dmminstance"

// The assembled view contains copies in hull coordinates. Switch to the owning
// document before selecting an instance for variable or map edits.
func (ws *WsShip) inspectInstance(instance *dmminstance.Instance) (string, func()) {
	if ws.assembly == nil {
		return "", nil
	}
	for _, atom := range ws.assembly.Cells[instance.Coord()] {
		if atom.Instance == nil || atom.Instance.Id() != instance.Id() {
			continue
		}
		source := ws.assembly.Sources[atom.Source]
		id, coord := atom.Instance.Id(), atom.Local
		return source.Name, func() {
			ws.flush()
			if ws.assembly == nil {
				return
			}
			// Flushing can rebuild the assembly; resolve its current source again.
			for index, current := range ws.assembly.Sources {
				if current.File != source.File || !current.Live.HasTile(coord) {
					continue
				}
				for _, live := range current.Live.GetTile(coord).Instances() {
					if live.Id() == id {
						ws.endShape()
						ws.source = index
						ws.activate(ws.panes[current.File])
						ws.pane.RenderContext()
						ws.pane.Editor().InstanceSelect(live)
						return
					}
				}
			}
		}
	}
	return "", nil
}
