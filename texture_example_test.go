package tela_test

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// fill returns an image of one color.
func fill(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Rect, image.NewUniform(c), image.Point{}, draw.Src)
	return img
}

func ExampleUseTexture() {
	t := &exampleT{}
	defer t.close()
	view := func() tela.Node {
		tex := tela.UseTexture()
		// A frame for every size the element gets, drawn before it is
		// painted: no frame is ever stretched.
		tela.UseLayoutEffect(func() func() {
			tex.OnResize(func(s tela.TextureSize) {
				tex.SubmitImage(fill(s.Width, s.Height, color.RGBA{79, 70, 229, 255}), nil)
			})
			return nil
		}, tela.Once)
		return Texture(tex, Class("w-40 h-20 rounded-xl"), AriaLabel("Globe"))
	}
	app := telatest.Mount(t, tela.C(view))
	fmt.Println(app.AriaSnapshot())
	fmt.Println(app.PixelAt(80, 40))
	// Output:
	// - img "Globe"
	// {79 70 229 255}
}

func ExampleTexture_SubmitImage() {
	t := &exampleT{}
	defer t.close()
	var tex *tela.Texture
	view := func() tela.Node {
		tex = tela.UseTexture()
		return Texture(tex, Class("w-40 h-20"))
	}
	app := telatest.Mount(t, tela.C(view))

	// Two images drawn in turns: each one is free again when its Release is
	// called.
	free := make(chan *image.RGBA, 2)
	free <- image.NewRGBA(image.Rect(0, 0, 160, 80))
	free <- image.NewRGBA(image.Rect(0, 0, 160, 80))
	for _, c := range []color.RGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}} {
		img := <-free // waits until Tela does not read it
		draw.Draw(img, img.Rect, image.NewUniform(c), image.Point{}, draw.Src)
		if err := tex.SubmitImage(img, func() { free <- img }); err != nil {
			fmt.Println(err)
		}
		app.Advance(0) // the window's next frame shows it
		fmt.Println(app.PixelAt(80, 40))
	}
	st := tex.Stats()
	fmt.Println(st.Submitted, st.Shown, st.Dropped)
	// Output:
	// {255 0 0 255}
	// {0 255 0 255}
	// {0 0 255 255}
	// 3 3 0
}

func ExampleTexture_OnResize() {
	t := &exampleT{}
	defer t.close()
	view := func() tela.Node {
		tex := tela.UseTexture()
		wide, setWide := tela.UseState(false)
		tela.UseMount(func() func() {
			tex.OnResize(func(s tela.TextureSize) { fmt.Printf("%dx%d at scale %g\n", s.Width, s.Height, s.Scale) })
			return nil
		})
		return Div(
			Texture(tex, Class("w-40 h-20 border-2"), If(wide, Class("w-80"))),
			Button(OnClick(func() { setWide.Set(true) }), Text("Widen")))
	}
	app := telatest.Mount(t, tela.C(view), telatest.Scale(2))
	app.Click(telatest.ByRole("button", "Widen"))
	// Output:
	// 312x152 at scale 2
	// 632x152 at scale 2
}

func ExampleTexture_NextFrame() {
	t := &exampleT{}
	defer t.close()
	frames := make(chan int, 3)
	view := func() tela.Node {
		tex := tela.UseTexture()
		tela.UseGo(func(ctx context.Context) {
			for n := range 3 { // usually: for
				ft, err := tex.NextFrame(ctx)
				if err != nil {
					return // ctx is done, or the component unmounted
				}
				_ = ft.Time // what to advance the scene to
				size := tex.Size()
				tex.SubmitImage(fill(size.Width, size.Height, color.Gray{uint8(100 * n)}), nil)
				frames <- n
			}
		}, tela.Once)
		return Texture(tex, Class("w-40 h-20"))
	}
	app := telatest.Mount(t, tela.C(view))
	app.WaitFor("three frames", func() bool { return len(frames) == 3 })
	fmt.Println(<-frames, <-frames, <-frames)
	// Output: 0 1 2
}

// Frames drawn by the GPU: each buffer is imported once, and submitted with
// a fence that tells when its drawing ended.
func ExampleTexture_Submit() {
	const w, h, stride = 1280, 720, 1280 * 4
	// What the application does with its own graphics API: allocate the
	// memory of a buffer and export it as a dma-buf, and draw into it,
	// exporting a sync file.
	allocate := func(i int) (fd int) { return -1 }
	render := func(i int) (syncFD int) { return -1 }

	start := func(ctx context.Context, tex *tela.Texture) error {
		free := make(chan int, 5) // five buffers are enough with fences
		var bufs [5]*tela.TextureBuffer
		for i := range bufs {
			buf, err := tex.Import(tela.Dmabuf{
				Width: w, Height: h,
				Fourcc: 0x34325258, // XR24: DRM_FORMAT_XRGB8888
				Planes: []tela.DmabufPlane{{FD: allocate(i), Stride: stride}},
			})
			if errors.Is(err, tela.ErrTextureUnsupported) {
				return err // tex.Device() lists what is
			} else if err != nil {
				return err
			}
			bufs[i] = buf
			free <- i
		}
		for {
			if _, err := tex.NextFrame(ctx); err != nil {
				return err
			}
			i := <-free
			err := tex.Submit(bufs[i], tela.TextureFrame{
				Fence:   tela.SyncFile(render(i)),
				Release: func() { free <- i },
			})
			if err != nil {
				return err
			}
		}
	}
	_ = start
}
