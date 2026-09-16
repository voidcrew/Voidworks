package cpenvironment

import (
	"strings"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/util"

	"github.com/SpaiR/imgui-go"
)

type treeNode struct {
	name         string
	orig         *dmenv.Object
	sprite       *dmicon.Sprite
	color        imgui.Vec4
	dir          int
	iconRevision uint64
}

func (e *Environment) newTreeNode(object *dmenv.Object) (*treeNode, bool) {
	if node, ok := e.treeNodes[object.Path]; ok {
		if node.iconRevision != dmicon.LayoutRevision {
			icon, _ := object.Vars.Text("icon")
			state, _ := object.Vars.Text("icon_state")
			node.sprite = dmicon.Cache.GetSpriteOrPlaceholderV(icon, state, node.dir)
			node.iconRevision = dmicon.LayoutRevision
		}
		return node, true
	}

	if e.tmpNewTreeNodesCount >= newTreeNodesLimit {
		return nil, false
	}

	e.tmpNewTreeNodesCount += 1

	icon, _ := object.Vars.Text("icon")
	iconState, _ := object.Vars.Text("icon_state")
	color := imgui.Vec4{X: 1, Y: 1, Z: 1, W: 1}
	dir, _ := object.Vars.Int("dir")

	if col, _ := object.Vars.Text("color"); col != "" {
		r, g, b, _ := util.ParseColor(col).RGBA()
		color = imgui.Vec4{X: r, Y: g, Z: b, W: 1}
	}

	node := &treeNode{
		name:         object.Path[strings.LastIndex(object.Path, "/")+1:],
		orig:         object,
		sprite:       dmicon.Cache.GetSpriteOrPlaceholderV(icon, iconState, dir),
		color:        color,
		dir:          dir,
		iconRevision: dmicon.LayoutRevision,
	}

	e.treeNodes[object.Path] = node
	return node, true
}
