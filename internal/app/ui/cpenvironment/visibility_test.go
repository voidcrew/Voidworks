package cpenvironment

import (
	"github.com/SpaiR/imgui-go"
	"runtime"
	"sdmm/internal/app/config"
	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmenv"
	"testing"
)

type visibilityApp struct {
	App
	filter *dm.PathsFilter
}

func (a *visibilityApp) PathsFilter() *dm.PathsFilter { return a.filter }
func (*visibilityApp) ConfigFind(string) config.Config {
	return &cpenvironmentConfig{Version: 1, NodeScale: 100}
}

func TestCheckingMixedVisibilityShowsWholeSubtree(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx := imgui.CreateContext(nil)
	defer ctx.Destroy()
	io := imgui.CurrentIO()
	io.SetIniFilename("")
	io.SetDisplaySize(imgui.Vec2{X: 400, Y: 300})
	io.SetDeltaTime(1.0 / 60)
	io.Fonts().TextureDataRGBA32()
	f := dm.NewPathsFilterEmpty()
	f.SetVisible("/obj/item/wrench", false)
	e := &Environment{app: &visibilityApp{filter: f}}
	node := &treeNode{orig: &dmenv.Object{Path: "/obj/item"}, name: "item"}
	var pos imgui.Vec2
	frame := func() {
		imgui.NewFrame()
		imgui.SetNextWindowPos(imgui.Vec2{})
		imgui.SetNextWindowSize(imgui.Vec2{X: 400, Y: 300})
		imgui.BeginV("Filter", nil, imgui.WindowFlagsNoSavedSettings)
		e.showVisibilityCheckbox(node)
		lo, hi := imgui.ItemRectMin(), imgui.ItemRectMax()
		pos = imgui.Vec2{X: (lo.X + hi.X) / 2, Y: (lo.Y + hi.Y) / 2}
		imgui.End()
		imgui.Render()
	}
	for i := 0; i < 3; i++ {
		frame()
	}
	io.SetMousePosition(pos)
	frame()
	io.SetMouseButtonDown(0, true)
	frame()
	io.SetMouseButtonDown(0, false)
	frame()
	if !f.IsVisiblePath("/obj/item") || !f.IsVisiblePath("/obj/item/wrench") || f.HasHiddenChildPath("/obj/item") {
		t.Fatal("checking a mixed subtree hid it instead of showing it")
	}
}
