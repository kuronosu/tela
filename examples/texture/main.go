// Command texture shows content that the application renders itself: one of
// three animated scenes drawn by its own Vulkan code (or, without a GPU, by
// its own Go code), pushed frame by frame to a tela.Texture and composed by
// Tela like any image, here inside a rounded card that clips it, under a label
// and over controls.
//
//   - view.go is the window, main.go what opens it.
//   - stream.go is the loop that draws the frames: at the window's pace
//     (Texture.NextFrame), of a share of the size the texture is shown at,
//     asleep while paused or out of sight.
//   - gpu_linux.go submits dmabufs drawn by package vulkan, the application's
//     own Vulkan renderer, on the GPU that Texture.Device names; software.go
//     submits pixels.
//   - scene.go moves the scenes; shaders/ draws them with a GPU, and shade.go,
//     planet.go, mountains.go and satin.go draw the same with the CPU.
//
// Built with -tags telagpu the frames never leave the GPU:
//
//	CGO_ENABLED=0 go run -tags telagpu .
//
// Without the tag, or with TELA_RENDERER=cpu, the same program draws the same
// scenes in software. `tela snapshot --preview Planet .` renders a still of one
// without a window.
package main

import (
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
)

var theme = kit.Theme()

// App is the window: kit's theme, always dark.
var App = tela.App{
	Title:       "Tela — texture",
	AppID:       "dev.telaui.texture",
	Width:       1180,
	Height:      660,
	MinWidth:    760,
	MinHeight:   600,
	Theme:       &theme,
	ColorScheme: tela.Dark,
	RootClass:   "bg-background text-foreground",
	Root:        Demo{},
}

// Previews are the screens of `tela snapshot` and `tela inspect`: a still of
// each scene.
var Previews = tela.Previews{
	"Planet":    {Root: Demo{Still: true, Scene: Planet, At: 4 * time.Second}},
	"Mountains": {Root: Demo{Still: true, Scene: Mountains, At: 4 * time.Second}},
	"Satin":     {Root: Demo{Still: true, Scene: Satin, At: 4 * time.Second}},
}

func main() { tela.Main(App) }
