package wsship

func (ws *WsShip) setCrewRole(role string) {
	c := &ws.crew
	j := &c.jobs[c.selected]
	if j.Role == role || (j.Role == "" && role == "crew") {
		return
	}
	j.Role, j.BorgModel = role, ""
	j.Outfit, j.BaseOutfit = "", ""
	j.Equipment, j.Backpack, j.Belt = nil, nil, nil
	if role == "crew" {
		j.Outfit, j.Category = "/datum/outfit/job/assistant", "Assistant"
	} else {
		j.Officer, j.Category = false, "Silicon"
		if role == "cyborg" {
			j.BorgModel = "/obj/item/robot_model/engineering"
		}
	}
	c.dirty = true
	ws.commitCrew()
}
