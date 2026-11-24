package h_test

import (
	"fmt"
	"image"
	"testing/fstest"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// check is a Lucide-style icon: stroked with currentColor.
var check = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none"
  stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
  <path d="M20 6 9 17l-5-5"/></svg>`}

func ExampleIcon() {
	t := &exampleT{}
	defer t.close()
	ui := Div(Class("flex gap-2 text-2xl"),
		// Decorative: hidden from accessibility; the button is named by
		// its text. Its color is the text color.
		Button(Class("inline-flex items-center gap-2 text-red-600"),
			Icon(check, Class("size-4 stroke-[1.5]")), Text("Save")),
		// Named: an image. Without a size it is 1em: 24 px here.
		Button(Icon(check, AriaLabel("Done"))),
	)
	app := telatest.Mount(t, ui)
	fmt.Println(app.AriaSnapshot())
	r := app.Bounds(telatest.ByRole("img", "Done"))
	fmt.Println(r.Width, r.Height)
	// Output:
	// - button "Save"
	// - button "Done":
	//   - img "Done"
	// 24 24
}

func ExampleSrcPath() {
	t := &exampleT{}
	defer t.close()
	// Usually an embed.FS registered from an init function.
	tela.Assets(fstest.MapFS{"img/logo.svg": {Data: []byte(
		`<svg xmlns="http://www.w3.org/2000/svg" width="120" height="40"><rect width="120" height="40" fill="#4f46e5"/></svg>`)}})

	// With only a height, the width follows the image's aspect ratio.
	app := telatest.Mount(t, Img(SrcPath("img/logo.svg"), Alt("Logo"), Class("h-8")))
	r := app.Bounds(telatest.ByRole("img", "Logo"))
	fmt.Println(r.Width, r.Height)
	// Output: 96 32
}

func ExampleTexture() {
	t := &exampleT{}
	defer t.close()
	var tex *tela.Texture
	player := func() tela.Node {
		tex = tela.UseTexture()
		return Div(Class("relative w-80 overflow-hidden rounded-xl"),
			Texture(tex, Class("w-full aspect-video object-contain bg-black"), AriaLabel("Video")),
			P(Class("absolute bottom-2 left-2 text-white"), Text("Live")))
	}
	app := telatest.Mount(t, tela.C(player))
	// A frame of the video, from any goroutine: 4:3 in a 16:9 element.
	frame := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for i := range frame.Pix {
		frame.Pix[i] = 255
	}
	if err := tex.SubmitImage(frame, nil); err != nil {
		fmt.Println(err)
	}
	fmt.Println(app.AriaSnapshot())
	fmt.Println(app.PixelAt(10, 90), app.PixelAt(160, 90))
	// Output:
	// - img "Video"
	// - paragraph: Live
	// {0 0 0 255} {255 255 255 255}
}

func ExampleTint() {
	t := &exampleT{}
	defer t.close()
	// A monochrome icon: only its alpha matters.
	glyph := `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16" fill="#000"/></svg>`
	app := telatest.Mount(t, Img(SrcBytes([]byte(glyph)), Tint(tela.Hex("#D97757")), Class("size-8")))
	fmt.Println(app.PixelAt(16, 16))
	// Output: {217 119 87 255}
}
