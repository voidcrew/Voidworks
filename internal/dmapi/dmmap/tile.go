package dmmap

import (
	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/util"
)

type Tile struct {
	Coord                    util.Point
	instances                Instances
	DefaultTurf, DefaultArea *dmmprefab.Prefab
}

func (t Tile) Copy() Tile {
	return Tile{Coord: t.Coord, instances: t.instances.DeepCopy(), DefaultTurf: t.DefaultTurf, DefaultArea: t.DefaultArea}
}

func (t *Tile) Set(instances Instances) {
	t.instances = instances
}

func (t Tile) Instances() Instances {
	return t.instances
}

func (t *Tile) InstancesSet(prefabs dmmdata.Prefabs) {
	t.instances = InstancesFromPrefabs(t.Coord, prefabs)
}

func (t *Tile) InstancesAdd(prefab *dmmprefab.Prefab) {
	t.instances = append(t.instances, dmminstance.New(t.Coord, prefab))
}

func (t *Tile) InstancesRemoveByPath(pathToRemove string) {
	instances := make(Instances, 0, len(t.instances))
	for _, instance := range t.instances {
		if !dm.IsPath(instance.Prefab().Path(), pathToRemove) {
			instances = append(instances, instance)
		}
	}
	t.instances = instances
}

func (t *Tile) InstancesRemoveByInstance(i *dmminstance.Instance) {
	for idx, instance := range t.instances {
		if instance.Id() == i.Id() {
			t.instances = append(t.instances[:idx], t.instances[idx+1:]...)
			return
		}
	}
}

// InstancesRegenerate restores the required turf and area only. Mapping helpers
// are ordinary editable instances: restoring them here duplicates moves and cuts.
func (t *Tile) InstancesRegenerate() {
	var hasArea, hasTurf bool
	for _, instance := range t.instances {
		if dm.IsPath(instance.Prefab().Path(), "/area") {
			hasArea = true
		} else if dm.IsPath(instance.Prefab().Path(), "/turf") {
			hasTurf = true
		}
	}
	if !hasArea {
		area := t.DefaultArea
		if area == nil {
			area = BaseArea
		}
		t.InstancesAdd(area)
	}
	if !hasTurf {
		turf := t.DefaultTurf
		if turf == nil {
			turf = BaseTurf
		}
		t.InstancesAdd(turf)
	}
}
