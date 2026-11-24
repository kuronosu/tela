package telatest_test

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// solid returns an image of one color.
func solid(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Rect, image.NewUniform(c), image.Point{}, draw.Src)
	return img
}

// A texture without a producer: the test renders what the application's
// graphics would, at the size the element asks for.
func ExampleApp_SubmitTexture() {
	view := func() tela.Node {
		tex := tela.UseTexture()
		return Div(Class("p-4"),
			Texture(tex, TestID("globe"), Class("h-24 w-40 rounded-xl"), AriaLabel("Globe")))
	}
	app := telatest.Mount(t, tela.C(view), telatest.Size(200, 140), telatest.Scale(2))
	globe := telatest.ByTestID("globe")

	size := app.TextureSize(globe) // device pixels: what a frame should have
	fmt.Printf("%dx%d at x%g\n", size.Width, size.Height, size.Scale)

	app.SubmitTexture(globe, solid(size.Width, size.Height, color.RGBA{79, 70, 229, 255}))
	fmt.Println(app.PixelAt(100, 70)) // the frame is on the screen

	st := app.TextureStats(globe)
	fmt.Println(st.Submitted, st.Shown, st.Dropped)
	// Output:
	// 320x192 at x2
	// {79 70 229 255}
	// 1 1 0
}

// The handle that the element shows takes every call of the application's
// own.
func ExampleApp_Texture() {
	view := func() tela.Node {
		tex := tela.UseTexture()
		return Texture(tex, Class("h-20 w-20"), AriaLabel("Radar"))
	}
	app := telatest.Mount(t, tela.C(view), telatest.Size(100, 100))

	tex := app.Texture(telatest.ByRole("img", "Radar"))
	tex.SubmitImage(solid(80, 80, color.RGBA{255, 0, 0, 255}), nil)
	app.Advance(0) // the window's next frame shows it
	fmt.Println(app.PixelAt(40, 40), tex.Device().GPU)
	// Output: {255 0 0 255} false
}

// A producer that renders at the window's pace never lets the app settle by
// itself: its frames are stepped by hand.
func ExampleApp_StepTexture() {
	view := func() tela.Node {
		tex := tela.UseTexture()
		tela.UseGo(func(ctx context.Context) {
			for n := 0; ; n++ {
				if _, err := tex.NextFrame(ctx); err != nil {
					return
				}
				s := tex.Size()
				tex.SubmitImage(solid(s.Width, s.Height, color.Gray{uint8(60 * (n + 1))}), nil)
			}
		}, tela.Once)
		return Texture(tex, TestID("screen"), Class("h-20 w-20"))
	}
	app := telatest.Mount(t, tela.C(view), telatest.Size(100, 100))
	screen := telatest.ByTestID("screen")

	for range 3 {
		app.StepTexture(screen)
		fmt.Println(app.PixelAt(10, 10))
	}
	for _, p := range app.Pending() {
		if p.Kind == telatest.PendingNextFrame {
			fmt.Println(p) // what keeps the app awake
		}
	}
	// Output:
	// {60 60 60 255}
	// {120 120 120 255}
	// {180 180 180 255}
	// 1 goroutine(s) wait in Texture.NextFrame for the next frame of the window: a texture on screen whose producer renders at the window's pace asks for a frame at every vsync
}

// An application that renders with its own GPU gives Tela dmabufs. The
// fake device stands in for the GPU that draws the windows: the test makes
// the pictures the application's renderer would draw, and signals the
// fences of its frames.
func ExampleNewFakeTextureDevice() {
	dev := telatest.NewFakeTextureDevice(tela.TextureDevice{
		GPU: true, Name: "Fake GPU",
		Formats: []tela.TextureFormat{{Fourcc: 0x34325241}}, // AR24, linear
	})
	// The application's renderer: draws into the buffer it just imported.
	dev.OnImport(func(b *telatest.FakeTextureBuffer) { b.Fill(color.RGBA{16, 185, 129, 255}) })

	var tex *tela.Texture
	view := func() tela.Node {
		tex = tela.UseTexture()
		return Texture(tex, TestID("screen"), Class("h-20 w-20"))
	}
	app := telatest.Mount(t, tela.C(view), telatest.Size(100, 100), telatest.WithTextureDevice(dev))

	// What the application does: import once, submit with a fence.
	fmt.Println(tex.Device().Name)
	buf, _ := tex.Import(tela.Dmabuf{Width: 80, Height: 80, Fourcc: 0x34325241,
		Planes: []tela.DmabufPlane{{FD: 3, Stride: 320}}})
	fence := telatest.NewFakeFence()
	released := make(chan struct{})
	tex.Submit(buf, tela.TextureFrame{Fence: fence, Release: func() { close(released) }})

	app.Advance(0)
	for _, p := range app.Pending() {
		if p.Kind == telatest.PendingFence {
			fmt.Println(p.Kind, app.PixelAt(40, 40)) // waits for the GPU
		}
	}
	fence.Signal()
	app.Advance(0)
	fmt.Println(app.PixelAt(40, 40)) // the frame is shown
	// Output:
	// Fake GPU
	// texture fence {255 255 255 255}
	// {16 185 129 255}
}

// Release is called once per frame, whatever happens to it: the test checks
// how many frames Tela still holds.
func ExampleApp_ExpectReleases() {
	view := func() tela.Node {
		tex := tela.UseTexture()
		return Texture(tex, TestID("screen"), Class("h-20 w-20"))
	}
	app := telatest.Mount(t, tela.C(view), telatest.Size(100, 100))
	tex := app.Texture(telatest.ByTestID("screen"))

	for i := range 3 {
		tex.SubmitImage(solid(80, 80, color.Gray{uint8(80 * i)}), func() {})
		app.Advance(time.Second / 60)
	}
	app.ExpectReleases(1) // the one on the screen; the other two went back
	fmt.Println("held: 1")
	// Output: held: 1
}
