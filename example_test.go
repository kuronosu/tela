package tela_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing/fstest"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/style"
)

func ExamplePreviews() {
	login := func() tela.Node { return core.Text("Login") }
	settings := func() tela.Node { return core.Text("Settings") }

	previews := tela.Previews{
		"Settings": {Root: tela.C(settings)},
		"Login":    {Root: tela.C(login)},
	}
	// `tela snapshot` renders the first name unless --preview says otherwise.
	fmt.Println(previews.Names())
	// Output: [Login Settings]
}

func ExampleHex() {
	c := tela.Hex("#1e1e2e")
	fmt.Println(c.R == tela.RGB(30, 30, 46).R, c)
	// Output: true #1e1e2e
}

// solidPNG returns a 2x2 PNG of one color.
func solidPNG(c color.Color) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for i := range 4 {
		img.Set(i%2, i/2, c)
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func ExampleAssets() {
	// An app embeds its images and registers them from an init function:
	//
	//	//go:embed img
	//	var images embed.FS
	//
	//	func init() { tela.Assets(images) }
	//
	// Here an in-memory file system holds one red PNG.
	tela.Assets(fstest.MapFS{"img/red.png": {Data: solidPNG(color.RGBA{R: 255, A: 255})}})

	// With package h: Div(Class("size-8 bg-[url(img/red.png)] bg-cover")).
	card := core.Primitive(style.KindDiv, core.Class("size-8 bg-[url(img/red.png)] bg-cover"))
	_, img := core.Render(card, core.RenderOptions{Width: 32, Height: 32})
	fmt.Println(img.RGBAAt(16, 16))
	// Output: {255 0 0 255}
}

func ExampleBgImage() {
	// A decoded image, such as one from image.Decode or generated in
	// code. Pass the same value on every build: it is converted once.
	photo := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := range 16 {
		photo.Set(i%4, i/4, color.RGBA{B: 255, A: 255})
	}

	// With package h: Div(Class("size-8 bg-cover bg-center"), Style(tela.BgImage(photo))).
	card := core.Primitive(style.KindDiv, core.Class("size-8 bg-cover bg-center"), core.Style(tela.BgImage(photo)))
	_, img := core.Render(card, core.RenderOptions{Width: 32, Height: 32})
	fmt.Println(img.RGBAAt(16, 16))
	// Output: {0 0 255 255}
}
