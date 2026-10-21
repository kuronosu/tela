package telatest_test

import (
	"bytes"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Settings is a panel with a switch, a disclosure, a toggle, a tab, a
// disabled button and a text field, to test the state expectations.
func Settings() tela.Node {
	wifi, setWifi := tela.UseState(false)
	open, setOpen := tela.UseState(false)
	bold, setBold := tela.UseState(false)
	name, setName := tela.UseState("")
	return Div(TestID("panel"), Class("flex flex-col items-start gap-2 p-4"),
		Button(Role("switch"), AriaLabel("Wi-Fi"), AriaChecked(wifi), Class("rounded bg-slate-200 px-2"),
			OnClick(func(tela.MouseEvent) { setWifi.Set(!wifi) }), Text("Wi-Fi")),
		Button(AriaLabel("Details"), AriaExpanded(open), Class("rounded bg-slate-200 px-2"),
			OnClick(func(tela.MouseEvent) { setOpen.Set(!open) }), Text("Details")),
		Button(AriaLabel("Bold"), AriaPressed(bold), Class("rounded bg-slate-200 px-2"),
			OnClick(func(tela.MouseEvent) { setBold.Set(!bold) }), Text("Bold")),
		Div(Role("tab"), AriaLabel("General"), AriaSelected(true), Text("General")),
		Button(Disabled(true), Class("rounded bg-slate-200 px-2"), Text("Save")),
		Input(AriaLabel("Name"), Bind(name, setName)),
	)
}

func TestBoundsIsARect(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(Class("p-4"), Div(TestID("box"), Class("h-10 w-20 bg-black")))
	}, telatest.Size(200, 100))
	if got, want := app.Bounds(telatest.ByTestID("box")), (tela.Rect{X: 16, Y: 16, Width: 80, Height: 40}); got != want {
		t.Errorf("Bounds = %+v, want %+v", got, want)
	}
}

func TestMountFunc(t *testing.T) {
	app := telatest.MountFunc(t, Counter, telatest.Size(400, 300))
	app.Click(telatest.ByRole("button", "Increment"))
	app.ExpectText("Count: 1")
}

func TestByTestID(t *testing.T) {
	app := telatest.MountFunc(t, Settings, telatest.Size(400, 400))
	if r := app.Bounds(telatest.ByTestID("panel")); r.Width != 400 {
		t.Errorf("the panel is %gpx wide, want 400", r.Width)
	}
	app.ExpectCount(telatest.ByTestID("panel"), 1)
	app.ExpectCount(telatest.ByTestID("nope"), 0)
	if s := telatest.ByTestID("panel").String(); s != `getByTestId("panel")` {
		t.Errorf("String() = %s", s)
	}
	// It is not exposed in the aria snapshot.
	if snap := app.AriaSnapshot(); strings.Contains(snap, "panel") {
		t.Errorf("the test id leaked into the aria snapshot:\n%s", snap)
	}
	app.Hover(telatest.ByTestID("panel").Nth(0)) // narrowing works
	// A miss lists the ids on screen and suggests the closest.
	mustFail(t, func(tb testing.TB) {
		telatest.MountFunc(tb, Settings, telatest.Timeout(50*time.Millisecond)).Hover(telatest.ByTestID("pannel"))
	}, `no element matches getByTestId("pannel")`, `did you mean "panel"?`)
	mustFail(t, func(tb testing.TB) {
		telatest.MountFunc(tb, Settings).Hover(telatest.ByTestID(""))
	}, "ByTestID needs the id given to h.TestID")
	// Several elements with the same id make the locator strict.
	mustFail(t, func(tb testing.TB) {
		app := telatest.MountFunc(tb, func() tela.Node {
			return Div(Div(TestID("dup"), Text("a")), Div(TestID("dup"), Text("b")))
		})
		app.Hover(telatest.ByTestID("dup"))
	}, "strict mode violation", `(testid "dup")`)
}

func TestClickAt(t *testing.T) {
	app := telatest.MountFunc(t, Counter, telatest.Size(400, 300))
	r := app.Bounds(telatest.ByRole("button", "Increment"))
	app.ClickAt(r.X+r.Width/2, r.Y+r.Height/2)
	app.ExpectText("Count: 1")
	// A click outside every handler does nothing.
	app.ClickAt(390, 290)
	app.ExpectText("Count: 1")
	mustFail(t, func(tb testing.TB) {
		telatest.MountFunc(tb, Counter, telatest.Size(400, 300)).ClickAt(400, 10)
	}, "ClickAt(400, 10): the point is outside the window", "400x300")
}

func TestDragFromTo(t *testing.T) {
	var rec recorder
	var last tela.DragEvent
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(Class("p-4"), Div(Class("size-12 bg-indigo-500"),
			OnDragStart(func(tela.DragEvent) { rec.add("start") }),
			OnDrag(func(e tela.DragEvent) { last = e }),
			OnDragEnd(func(tela.DragEvent) { rec.add("end") })))
	}, telatest.Size(300, 200))
	app.DragFromTo(tela.Point{X: 30, Y: 30}, tela.Point{X: 110, Y: 70}, telatest.Steps(4))
	if got := rec.take(); got != "start,end" {
		t.Errorf("drag sent %q", got)
	}
	if last.OffsetX != 80 || last.OffsetY != 40 {
		t.Errorf("the last drag offset is (%v, %v), want (80, 40)", last.OffsetX, last.OffsetY)
	}
	mustFail(t, func(tb testing.TB) {
		telatest.MountFunc(tb, Counter, telatest.Size(300, 200)).DragFromTo(tela.Point{X: 10, Y: 10}, tela.Point{X: 10, Y: 500})
	}, "DragFromTo(10, 500): the point is outside the window")
}

func TestPixelAtMapsLogicalToDevicePixels(t *testing.T) {
	// A red 20x20 box at logical (10, 10).
	red := color.RGBA{R: 255, A: 255}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	for _, scale := range []float64{1, 2} {
		app := telatest.MountFunc(t, func() tela.Node {
			return Div(Class("p-[10px]"), Div(Class("size-5 bg-[#ff0000]")))
		}, telatest.Size(60, 60), telatest.Scale(scale))
		if got := app.PixelAt(10, 10); got != red {
			t.Errorf("scale %g: PixelAt(10, 10) = %v, want red", scale, got)
		}
		if got := app.PixelAt(29.9, 29.9); got != red {
			t.Errorf("scale %g: PixelAt(29.9, 29.9) = %v, want red", scale, got)
		}
		if got := app.PixelAt(9.9, 15); got != white {
			t.Errorf("scale %g: PixelAt(9.9, 15) = %v, want white", scale, got)
		}
		if got := app.PixelAt(30, 15); got != white {
			t.Errorf("scale %g: PixelAt(30, 15) = %v, want white", scale, got)
		}
		img := app.Screenshot()
		if got, want := app.PixelAt(10.5, 12.25), img.RGBAAt(int(10.5*scale), int(12.25*scale)); got != want {
			t.Errorf("scale %g: PixelAt = %v, the frame has %v at the device pixel", scale, got, want)
		}
	}
	mustFail(t, func(tb testing.TB) {
		telatest.MountFunc(tb, Counter, telatest.Size(60, 60), telatest.Scale(2)).PixelAt(60, 0)
	}, "PixelAt(60, 0): the point is outside the window", "60x60 logical pixels (120x120 device pixels at scale 2)")
}

func TestMeasureCountsRebuilds(t *testing.T) {
	app := telatest.MountFunc(t, Counter, telatest.Size(400, 300))
	idle := app.Measure(func() {})
	if idle != (telatest.Stats{}) {
		t.Errorf("a no-op measured %+v, want nothing", idle)
	}
	st := app.Measure(func() { app.Click(telatest.ByRole("button", "Increment")) })
	if st.Builds < 1 || st.Frames < 1 || st.Layouts < 1 || st.Paints < 1 {
		t.Errorf("a click that rebuilds measured %+v, want builds, frames, layouts and paints", st)
	}
	// The delta equals the difference of the readings of Stats.
	before := app.Stats()
	st = app.Measure(func() { app.Click(telatest.ByRole("button", "Increment")) })
	after := app.Stats()
	if st.Builds != after.Builds-before.Builds || st.Frames != after.Frames-before.Frames || st.Paints != after.Paints-before.Paints {
		t.Errorf("Measure = %+v, Stats difference = builds %d, frames %d, paints %d",
			st, after.Builds-before.Builds, after.Frames-before.Frames, after.Paints-before.Paints)
	}
	// A narrow update: a TextFn click builds nothing.
	app = telatest.MountFunc(t, Clock, telatest.Size(300, 200))
	if st := app.Measure(func() { app.Click(telatest.ByRole("button", "Tick")) }); st.Builds != 0 {
		t.Errorf("the TextFn update built %d components, want none", st.Builds)
	}
}

func TestGoldenScales(t *testing.T) {
	t.Chdir(t.TempDir())
	mount := func(tb testing.TB, opts ...telatest.Option) *telatest.App {
		return telatest.MountFunc(tb, Counter, append([]telatest.Option{telatest.Size(200, 120)}, opts...)...)
	}
	t.Setenv("TELA_UPDATE", "1")
	mustPass(t, func(tb testing.TB) { mount(tb).GoldenScales("counter", 1, 2, 1.5) })
	t.Setenv("TELA_UPDATE", "")
	for _, f := range []string{"counter_x1.png", "counter_x2.png", "counter_x1.5.png"} {
		if _, err := os.Stat(filepath.Join("testdata", f)); err != nil {
			t.Errorf("GoldenScales did not write %s: %v", f, err)
		}
	}
	// They compare, from an app at any scale and with or without the extension.
	mustPass(t, func(tb testing.TB) { mount(tb, telatest.Scale(2)).GoldenScales("counter.png", 1, 2, 1.5) })
	// Each is what an app mounted at that scale shows, and the app keeps
	// its state and its scale.
	t.Setenv("TELA_UPDATE", "1")
	mustPass(t, func(tb testing.TB) {
		app := mount(tb)
		app.Click(telatest.ByRole("button", "Increment"))
		app.GoldenScales("counter_one", 2)
		if got := app.Screenshot().Bounds().Dx(); got != 200 {
			tb.Errorf("the app is %d device pixels wide after GoldenScales, want 200 (its scale restored)", got)
		}
		app.ExpectText("Count: 1")
	})
	mustPass(t, func(tb testing.TB) {
		fresh := mount(tb, telatest.Scale(2))
		fresh.Click(telatest.ByRole("button", "Increment"))
		fresh.Golden("fresh_x2.png")
	})
	t.Setenv("TELA_UPDATE", "")
	got := readFile(t, filepath.Join("testdata", "counter_one_x2.png"))
	if want := readFile(t, filepath.Join("testdata", "fresh_x2.png")); !bytes.Equal(got, want) {
		t.Error("the golden at scale 2 differs from the picture of an app mounted at scale 2")
	}
	// Wrong arguments and a changed picture fail.
	mustFail(t, func(tb testing.TB) { mount(tb).GoldenScales("counter") }, "needs at least one scale")
	mustFail(t, func(tb testing.TB) { mount(tb).GoldenScales("counter", 0) }, "must be positive")
	mustFail(t, func(tb testing.TB) {
		app := mount(tb)
		app.Click(telatest.ByRole("button", "Increment"))
		app.GoldenScales("counter", 1)
	}, "golden: counter_x1", "pixel(s) differ")
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestExpectStates(t *testing.T) {
	app := telatest.MountFunc(t, Settings, telatest.Size(400, 400))
	wifi := telatest.ByRole("switch", "Wi-Fi")
	app.Expect(wifi, telatest.Checked(false), telatest.Disabled(false), telatest.Focused(false), telatest.HasName("Wi-Fi"))
	app.Click(wifi)
	app.Expect(wifi, telatest.Checked(true), telatest.Focused(true))
	details := telatest.ByRole("button", "Details")
	app.Expect(details, telatest.Expanded(false))
	app.Click(details)
	app.Expect(details, telatest.Expanded(true))
	bold := telatest.ByRole("button", "Bold")
	app.Expect(bold, telatest.Pressed(false))
	app.Click(bold)
	app.Expect(bold, telatest.Pressed(true))
	app.Expect(telatest.ByRole("tab", "General"), telatest.Selected(true))
	app.Expect(telatest.ByRole("button", "Save"), telatest.Disabled(true))
	app.Fill(telatest.ByRole("textbox", "Name"), "Ada")
	app.Expect(telatest.ByRole("textbox", "Name"), telatest.HasValue("Ada"))
}

func TestExpectStateFailures(t *testing.T) {
	mount := func(tb testing.TB) *telatest.App {
		return telatest.MountFunc(tb, Settings, telatest.Size(400, 400), telatest.Timeout(100*time.Millisecond))
	}
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(telatest.ByRole("switch", "Wi-Fi"), telatest.Checked(true), telatest.Disabled(false))
	}, `expected switch "Wi-Fi" to have checked=true, but it has checked=false`,
		"its states: checked=false, disabled=false, focused=false",
		`its aria line: - switch "Wi-Fi"`, "aria snapshot:")
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(telatest.ByRole("button", "Save"), telatest.Disabled(false), telatest.HasName("Store"))
	}, `to have disabled=false, name="Store", but it has disabled=true, name="Save"`, "[disabled]")
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(telatest.ByRole("button", "Save"), telatest.Checked(true))
	}, "checked is not set on it (h.AriaChecked sets it)")
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(telatest.ByRole("textbox", "Name"), telatest.HasValue("Bob"))
	}, `value="Bob", but it has value=""`)
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(telatest.ByRole("switch", "Wifi"), telatest.Checked(true))
	}, `no element matches getByRole("switch", name="Wifi")`, `did you mean switch "Wi-Fi"?`)
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(telatest.ByRole("switch", "Wi-Fi"))
	}, "without states checks nothing")
	mustFail(t, func(tb testing.TB) {
		mount(tb).Expect(telatest.ByRole("button", ""), telatest.Disabled(false))
	}, "strict mode violation")
}

func TestExpectWaits(t *testing.T) {
	var set func(bool)
	app := telatest.MountFunc(t, func() tela.Node {
		on, setOn := tela.UseState(false)
		set = func(v bool) { setOn.Set(v) }
		return Button(Role("switch"), AriaLabel("Sync"), AriaChecked(on), Text("Sync"))
	})
	go func() {
		time.Sleep(20 * time.Millisecond)
		set(true)
	}()
	app.Expect(telatest.ByRole("switch", "Sync"), telatest.Checked(true))
}
