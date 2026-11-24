// Command texture shows content that the application renders itself: an
// animated scene drawn by its own Vulkan code (or, without a GPU, by its own
// Go code), pushed frame by frame to a tela.Texture and composed by Tela like
// any image, here inside a rounded card that clips it, under a label and over
// controls.
//
//   - view.go is the window, main.go what opens it.
//   - stream.go is the loop that draws the frames: at the window's pace
//     (Texture.NextFrame), of the size the texture is shown at, asleep while
//     paused or out of sight.
//   - gpu_linux.go submits dmabufs drawn by package vulkan, the application's
//     own Vulkan renderer, on the GPU that Texture.Device names; software.go
//     submits pixels.
//   - scene.go moves the scene; shaders/ and shade.go draw it.
//
// Built with -tags telagpu the frames never leave the GPU:
//
//	CGO_ENABLED=0 go run -tags telagpu .
//
// Without the tag, or with TELA_RENDERER=cpu, the same program draws the same
// scene in software. `tela snapshot .` renders a still of it without a window.
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
	Width:       960,
	Height:      760,
	Theme:       &theme,
	ColorScheme: tela.Dark,
	RootClass:   "bg-background text-foreground",
	Root:        Demo{},
}

// Preview is the screen of `tela snapshot` and `tela inspect`: a still of the
// scene.
func Preview() tela.Component { return Demo{Still: true, At: 4 * time.Second} }

func main() { tela.Main(App) }
