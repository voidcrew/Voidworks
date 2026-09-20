package editor

import (
	"fmt"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmap/dmminstance"
)

// SpriteTarget resolves native instances after a context switch or history
// restore. Projected workshop copies must never receive map edits.
func (e *Editor) SpriteTarget(id uint64) *dmminstance.Instance {
	for _, tile := range e.dmm.Tiles {
		for _, instance := range tile.Instances() {
			if instance.Id() == id {
				return instance
			}
		}
	}
	return nil
}

func (e *Editor) ReplaceSprite(id uint64, original, replacement *dmmprefab.Prefab) error {
	instance := e.SpriteTarget(id)
	if instance == nil || instance.Prefab().Id() != original.Id() {
		return fmt.Errorf("This object has changed. Close the picker and select it again.")
	}
	if replacement.Path() != original.Path() {
		return fmt.Errorf("Sprite replacement must keep the same object type.")
	}
	if original.Vars().ValueV("icon", "") == replacement.Vars().ValueV("icon", "") && original.Vars().ValueV("icon_state", "") == replacement.Vars().ValueV("icon_state", "") {
		return nil
	}
	instance.SetPrefab(dmmap.PrefabStorage.Put(replacement))
	e.app.DoSelectPrefab(instance.Prefab())
	e.app.DoEditInstance(instance)
	e.CommitChanges("Replace sprite")
	return nil
}
