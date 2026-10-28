package telatest_test

import (
	"image/color"
	"reflect"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// shell is the root of the apps below: a swatch of a theme token through an
// alias, a box with a dark: variant, and a counter with state.
func shell() tela.Node {
	n, setN := tela.UseState(1)
	return Div(Class("flex items-start gap-2"),
		Div(TestID("token"), Class("swatch")),
		Div(TestID("scheme"), Class("size-8 bg-rose-500 dark:bg-emerald-500")),
		Button(Class("px-2"), OnClick(func(tela.MouseEvent) { setN.Set(n + 1) }), Textf("n=%d", n)),
	)
}

func shellApp() tela.App {
	return tela.App{
		Title:       "Shell",
		Width:       200,
		Height:      100,
		RootClass:   "bg-slate-800 p-4 text-white",
		ColorScheme: tela.Dark,
		Theme: &tela.Theme{Colors: map[string]tela.SchemeColor{
			"brand": {Light: tela.Hex("#ff0000"), Dark: tela.Hex("#0000ff")},
		}},
		ClassAliases: map[string]string{"swatch": "size-8 bg-brand"},
		Root:         tela.C(shell),
	}
}

func center(app *telatest.App, id string) color.RGBA {
	r := app.Bounds(telatest.ByTestID(id))
	return app.PixelAt(r.X+r.Width/2, r.Y+r.Height/2)
}

// MountApp mounts the root of the App in the App's window: its size, root
// class, color scheme, theme and aliases.
func TestMountApp(t *testing.T) {
	app := telatest.MountApp(t, shellApp())
	if b := app.Screenshot().Bounds(); b.Dx() != 200 || b.Dy() != 100 {
		t.Errorf("the window is %v, want the App's 200x100", b)
	}
	if got := app.PixelAt(198, 98); got != (color.RGBA{0x1d, 0x29, 0x3d, 0xff}) {
		t.Errorf("RootClass: corner %v, want slate-800", got)
	}
	if got := center(app, "token"); got != (color.RGBA{0, 0, 0xff, 0xff}) {
		t.Errorf("Theme and ClassAliases: swatch %v, want the dark value of brand", got)
	}
	if got := center(app, "scheme"); got != (color.RGBA{0x00, 0xbc, 0x7d, 0xff}) {
		t.Errorf("ColorScheme: box %v, want emerald-500 (the dark: variant)", got)
	}
	// Root is a component: its hooks work.
	app.Click(telatest.ByRole("button", "n=1"))
	app.ExpectText("n=2")
}

// The options of MountApp override the App's, and the ones before FromApp
// give way to it.
func TestMountAppOptionsOverride(t *testing.T) {
	app := telatest.MountApp(t, shellApp(), telatest.Size(120, 80), telatest.ColorScheme(tela.Light))
	if b := app.Screenshot().Bounds(); b.Dx() != 120 || b.Dy() != 80 {
		t.Errorf("the window is %v, want telatest.Size's 120x80", b)
	}
	if got := center(app, "scheme"); got.R < 0xf0 || got.G > 0x40 {
		t.Errorf("box %v, want rose-500 (ColorScheme(Light) after the App's Dark)", got)
	}
	if got := center(app, "token"); got != (color.RGBA{0xff, 0, 0, 0xff}) {
		t.Errorf("swatch %v, want the light value of brand", got)
	}

	// FromApp sets only what the App sets: an earlier RootClass and size stay.
	bare := telatest.Mount(t, Div(Text("x")), telatest.Size(90, 60), telatest.RootClass("bg-rose-500"), telatest.FromApp(tela.App{Title: "x"}))
	if b := bare.Screenshot().Bounds(); b.Dx() != 90 || b.Dy() != 60 {
		t.Errorf("an App without a size changed the window to %v", b)
	}
	if got := bare.PixelAt(88, 58); got.R < 0xf0 || got.G > 0x40 {
		t.Errorf("an App without a RootClass undid the earlier one: corner %v", got)
	}
	// Only a full size counts.
	half := telatest.Mount(t, Div(Text("x")), telatest.FromApp(tela.App{Width: 300}))
	if b := half.Screenshot().Bounds(); b.Dx() != 800 || b.Dy() != 600 {
		t.Errorf("an App with only a Width gave a %v window, want the default 800x600", b)
	}
}

// App.Dev = false mounts as telatest.NoDev does: a class typo is not a test
// failure.
func TestMountAppDev(t *testing.T) {
	typo := func() tela.Node { return Div(Class("flex-colum p-2"), Text("hi")) }
	mustFail(t, func(tb testing.TB) { telatest.MountApp(tb, tela.App{Root: tela.C(typo)}) }, "flex-colum")
	off := false
	mustPass(t, func(tb testing.TB) { telatest.MountApp(tb, tela.App{Root: tela.C(typo), Dev: &off}) })
}

func TestMountAppNeedsRoot(t *testing.T) {
	mustFail(t, func(tb testing.TB) { telatest.MountApp(tb, tela.App{Title: "x"}) }, "MountApp needs app.Root", "tela.App{Root: tela.C(Counter)}", "Root: Counter{}")
}

// What `tela snapshot` renders and a window shows must not drift: every
// field of tela.App is applied by FromApp (how the content looks), applied
// by MountApp alone (the OS window: tools that render the screen want it
// visible and quit-able), or knowingly ignored. Adding a field to App makes
// this test fail until it is classified.
func TestAppFieldsAreClassified(t *testing.T) {
	applied := map[string]bool{
		"Width": true, "Height": true, "RootClass": true, "ColorScheme": true, "Theme": true,
		"ClassAliases": true, "ClassicScrollbars": true, "InstantScroll": true, "Dev": true,
	}
	// The option that sets each OS-window field by hand.
	mountApp := map[string]string{
		"Title": "Title", "AppID": "AppID",
		"MinWidth": "MinSize", "MinHeight": "MinSize",
		"MaxWidth": "MaxSize", "MaxHeight": "MaxSize",
		"FixedSize": "FixedSize", "Frameless": "Frameless",
		"AlwaysOnTop": "AlwaysOnTop", "Hidden": "StartHidden",
		"Icon": "Icon", "Placement": "Placement", "OnClose": "OnClose",
		"TitleBar": "TitleBar", "Backdrop": "Backdrop",
	}
	ignored := map[string]string{
		"Root":     "mounted by MountApp, not a setting",
		"DevState": "state outside the tree that only tela dev keeps across reloads",
	}
	typ := reflect.TypeFor[tela.App]()
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		_, isMount := mountApp[name]
		_, isIgnored := ignored[name]
		n := 0
		for _, b := range []bool{applied[name], isMount, isIgnored} {
			if b {
				n++
			}
		}
		switch {
		case n == 0:
			t.Errorf("tela.App.%s is not classified: apply it in telatest.FromApp (how the content looks) or in MountApp (the OS window, with an option of its own), or list it as ignored in this test", name)
		case n > 1:
			t.Errorf("tela.App.%s is classified more than once", name)
		}
	}
}

// A struct component is the Root as is; a function component is tela.C(fn).
type badge struct{ Text string }

func (b badge) Build() tela.Node { return Span(Text(b.Text)) }

func TestMountAppStructRoot(t *testing.T) {
	app := telatest.MountApp(t, tela.App{Root: badge{Text: "new"}})
	app.ExpectText("new")
}

func TestMountAppFuncRoot(t *testing.T) {
	app := telatest.MountApp(t, tela.App{Root: tela.C(func() tela.Node {
		n, set := tela.UseState(0)
		return Button(OnClick(func() { set.Set(n + 1) }), Textf("n=%d", n))
	})})
	app.Click(telatest.ByRole("button", "n=0"))
	app.ExpectText("n=1")
}
