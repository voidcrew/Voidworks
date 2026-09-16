package platform

import (
	"os"
	"runtime"
	"testing"

	"github.com/SpaiR/imgui-go"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// Run with VOIDWORKS_GL_TEST=1 and -gcflags=sdmm/internal/platform=-d=checkptr=2.
// A nonzero buffer offset converted to unsafe.Pointer fails that runtime check.
func TestRenderWithStackRelocation(t *testing.T) {
	if os.Getenv("VOIDWORKS_GL_TEST") == "" {
		t.Skip("requires a desktop OpenGL context")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := glfw.Init(); err != nil {
		t.Fatal(err)
	}
	defer glfw.Terminate()
	glfw.WindowHint(glfw.Visible, glfw.False)
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	w, err := glfw.CreateWindow(320, 240, "Rendering regression", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Destroy()
	w.MakeContextCurrent()
	if err := gl.Init(); err != nil {
		t.Fatal(err)
	}
	ctx := imgui.CreateContext(nil)
	defer ctx.Destroy()
	io := imgui.CurrentIO()
	io.SetIniFilename("")
	io.SetDisplaySize(imgui.Vec2{X: 320, Y: 240})
	io.SetDisplayFrameBufferScale(imgui.Vec2{X: 1, Y: 1})
	io.SetDeltaTime(1.0 / 60)
	InitImGuiGL()
	defer DisposeImGuiGL()
	var grow func(int) int
	grow = func(n int) int {
		var stack [1024]byte
		stack[n] = byte(n)
		if n > 0 {
			n += grow(n - 1)
		}
		runtime.KeepAlive(&stack)
		return n
	}
	for frame := 0; frame < 200; frame++ {
		grow(64)
		if frame%5 == 0 {
			runtime.GC()
		}
		gl.Clear(gl.COLOR_BUFFER_BIT)
		imgui.NewFrame()
		imgui.SetNextWindowPos(imgui.Vec2{})
		imgui.SetNextWindowSize(imgui.Vec2{X: 320, Y: 240})
		imgui.Begin("Render regression")
		imgui.Text("Render after stack growth and garbage collection")
		// Different clipping rectangles split the list into indexed draw commands.
		imgui.BeginChildV("clipped", imgui.Vec2{X: 180, Y: 80}, true, 0)
		imgui.Button("A second draw command")
		imgui.EndChild()
		if frame == 0 {
			draw := imgui.WindowDrawList()
			for range 20000 {
				draw.AddRectFilled(imgui.Vec2{X: 20, Y: 130}, imgui.Vec2{X: 22, Y: 132}, imgui.PackedColor(0xffffffff))
			}
			draw.AddRectFilled(imgui.Vec2{X: 200, Y: 150}, imgui.Vec2{X: 220, Y: 170}, imgui.PackedColorFromVec4(imgui.Vec4{Y: 1, W: 1}))
		}
		imgui.End()
		imgui.Render()
		Render(imgui.RenderedDrawData())
		if frame == 0 {
			var pixel [4]byte
			gl.ReadPixels(210, 240-160, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(&pixel[0]))
			if pixel[0] != 0 || pixel[1] != 255 || pixel[2] != 0 {
				t.Fatalf("geometry beyond 65536 vertices was not rendered: %v", pixel)
			}
		}
		if code := gl.GetError(); code != gl.NO_ERROR {
			t.Fatalf("frame %d: GL error %#x", frame, code)
		}
	}
}
