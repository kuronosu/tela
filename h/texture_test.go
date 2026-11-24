package h_test

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// scenePixels is a frame of the goldens: a sky, a sun, a checkered ground
// and a border one pixel wide that shows how the frame is fitted. Integers
// only: the same on every machine.
func scenePixels(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			var c color.RGBA
			switch {
			case x == 0 || y == 0 || x == w-1 || y == h-1:
				c = color.RGBA{220, 38, 38, 255}
			case (4*x-3*w)*(4*x-3*w)+(4*y-h)*(4*y-h) < h*h/3:
				c = color.RGBA{253, 224, 71, 255}
			case y < h*3/5:
				c = color.RGBA{uint8(60 + 100*y/h), uint8(130 + 100*y/h), 230, 255}
			case (x*10/w+y*8/h)%2 == 0:
				c = color.RGBA{22, 101, 52, 255}
			default:
				c = color.RGBA{134, 239, 172, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// glassDisc is a frame with transparent pixels: a disc, premultiplied.
func glassDisc(n int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := range n {
		for x := range n {
			if d := (2*x-n+1)*(2*x-n+1) + (2*y-n+1)*(2*y-n+1); d < n*n {
				a := 255 - 200*d/(n*n)
				img.SetRGBA(x, y, color.RGBA{uint8(79 * a / 255), uint8(70 * a / 255), uint8(229 * a / 255), uint8(a)})
			}
		}
	}
	return img
}

// textureGallery shows textures with what composes them.
func textureGallery() tela.Node {
	photo, disc, empty := tela.UseTexture(), tela.UseTexture(), tela.UseTexture()
	tela.UseLayoutEffect(func() func() {
		if err := photo.SubmitImage(scenePixels(160, 100), nil); err != nil {
			panic(err)
		}
		if err := disc.SubmitImage(glassDisc(64), nil); err != nil {
			panic(err)
		}
		return nil
	}, tela.Once)
	cell := func(label string, children ...tela.Child) tela.Node {
		return Div(Class("flex w-40 flex-col gap-1"),
			Div(append([]tela.Child{Class("relative flex h-28 items-center justify-center rounded-lg bg-white ring-1 ring-slate-200")}, children...)...),
			P(Class("text-center text-xs text-slate-500"), Text(label)))
	}
	return Div(Class("flex min-h-screen flex-wrap content-start gap-4 bg-slate-100 p-4"),
		cell("card, object-cover", Div(Class("overflow-hidden rounded-2xl border-2 border-slate-800 shadow-lg"), Texture(photo, Class("size-24 object-cover")))),
		cell("rounded, padded, border", Texture(photo, Class("h-24 w-36 rounded-3xl border-4 border-indigo-500 bg-amber-100 p-2"))),
		cell("object-contain", Texture(photo, Class("h-24 w-36 rounded-lg bg-slate-800 object-contain object-right"))),
		cell("object-none", Texture(photo, Class("h-20 w-24 object-none"))),
		cell("opacity", Texture(photo, Class("h-24 w-36 rounded-xl opacity-40"))),
		cell("rotated", Texture(photo, Class("h-16 w-28 rotate-12 rounded-xl shadow-xl"))),
		cell("blurred", Texture(photo, Class("h-20 w-32 blur-xs"))),
		cell("blend mode", Div(Class("absolute inset-2 rounded bg-linear-to-r from-rose-300 to-sky-300")), Texture(photo, Class("h-20 w-32 mix-blend-multiply"))),
		cell("backdrop-blur over it", Texture(photo, Class("h-24 w-36 rounded-xl")),
			Div(Class("absolute inset-x-6 bottom-4 rounded-lg border border-white/50 bg-white/30 p-1 text-center text-sm backdrop-blur-sm"), Text("Glass"))),
		cell("mask", Texture(photo, Class("h-24 w-36 mask-radial-from-40% mask-radial-to-70%"))),
		cell("transparent frame", Div(Class("absolute inset-0 rounded-lg bg-conic from-amber-200 via-emerald-200 to-amber-200")), Texture(disc, Class("relative size-24"))),
		cell("no frame yet", Texture(empty, Class("h-20 w-32 rounded-xl bg-slate-800"))),
	)
}

func TestTextureGolden(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			_, img := core.Render(core.Component(textureGallery), core.RenderOptions{
				Config: core.Config{Dev: true, Log: func(m string) { t.Error(m) }},
				Width:  720, Height: 460, Scale: scale,
			})
			golden.Image(t, fmt.Sprintf("texture_x%g", scale), img)
		})
	}
}

func TestTextureNilPanics(t *testing.T) {
	defer func() {
		if r := fmt.Sprint(recover()); !strings.Contains(r, "tex := tela.UseTexture()") {
			t.Errorf("recovered %q, want the fix", r)
		}
	}()
	Texture(nil, Class("w-full"))
}
