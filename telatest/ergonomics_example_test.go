package telatest_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func ExampleMountFunc() {
	app := telatest.MountFunc(t, Counter, telatest.Size(400, 300))

	app.Click(telatest.ByRole("button", "Increment"))
	app.ExpectText("Count: 1")
	fmt.Println(app.Count(telatest.ByRole("button", "Increment")))
	// Output: 1
}

func ExampleByTestID() {
	card := func() tela.Node {
		return Div(TestID("card"), Class("size-16 bg-slate-200"))
	}
	app := telatest.MountFunc(t, card, telatest.Size(100, 100))

	fmt.Println(telatest.ByTestID("card"))
	fmt.Println(app.Bounds(telatest.ByTestID("card")))
	fmt.Println(app.AriaSnapshot() == "") // the id is not in the snapshot
	// Output:
	// getByTestId("card")
	// {0 0 64 64}
	// true
}

func ExampleApp_Expect() {
	app := telatest.MountFunc(t, Settings, telatest.Size(400, 400))

	wifi := telatest.ByRole("switch", "Wi-Fi")
	app.Expect(wifi, telatest.Checked(false))
	app.Click(wifi)
	app.Expect(wifi, telatest.Checked(true), telatest.Disabled(false))
	app.Expect(telatest.ByRole("button", "Save"), telatest.Disabled(true))
	fmt.Println("ok")
	// Output: ok
}

func ExampleApp_Measure() {
	app := telatest.MountFunc(t, Counter, telatest.Size(400, 300))

	st := app.Measure(func() {
		app.Click(telatest.ByRole("button", "Increment"))
	})
	fmt.Println("builds:", st.Builds, "frames:", st.Frames > 0)
	// Output: builds: 1 frames: true
}

func ExampleApp_PixelAt() {
	swatch := func() tela.Node {
		return Div(Class("size-4 bg-[#ff0000]"))
	}
	// At scale 2 the logical point (8, 8) is the device pixel (16, 16).
	app := telatest.MountFunc(t, swatch, telatest.Size(32, 32), telatest.Scale(2))

	fmt.Println(app.PixelAt(8, 8), app.PixelAt(20, 20))
	// Output: {255 0 0 255} {255 255 255 255}
}

func ExampleApp_ClickAt() {
	app := telatest.MountFunc(t, Counter, telatest.Size(400, 300))

	r := app.Bounds(telatest.ByRole("button", "Increment"))
	app.ClickAt(r.X+r.Width/2, r.Y+r.Height/2)
	app.ExpectText("Count: 1")
	fmt.Println("clicked")
	// Output: clicked
}

func ExampleApp_DragFromTo() {
	moved := 0
	area := func() tela.Node {
		return Div(Class("size-32 bg-slate-200"),
			OnDrag(func(e tela.DragEvent) { moved = int(e.OffsetX) }))
	}
	app := telatest.MountFunc(t, area, telatest.Size(200, 200))

	app.DragFromTo(tela.Point{X: 10, Y: 10}, tela.Point{X: 90, Y: 10})
	fmt.Println(moved)
	// Output: 80
}

// GoldenScales has no output to check: it compares files in testdata.
func ExampleApp_GoldenScales() {
	app := telatest.MountFunc(t, Counter, telatest.Size(400, 300))

	// testdata/counter_x1.png and testdata/counter_x2.png; TELA_UPDATE=1
	// writes them.
	app.GoldenScales("counter", 1, 2)
}
