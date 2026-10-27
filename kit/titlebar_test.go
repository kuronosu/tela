package kit_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// TitleBar and WindowControls (ADR-182) are the kit's own, not shadcn/ui's,
// so they have no Chromium sheet: these tests check the roles and names,
// the sides, the states and the window actions.

var (
	stActive    = &tela.WindowState{Focused: true}
	stInactive  = &tela.WindowState{}
	stMaximized = &tela.WindowState{Focused: true, Maximized: true}
)

var (
	btnMinimize = telatest.ByRole("button", "Minimize")
	btnMaximize = telatest.ByRole("button", "Maximize")
	btnRestore  = telatest.ByRole("button", "Restore")
	btnClose    = telatest.ByRole("button", "Close")
)

var titleBarLooks = []struct {
	name string
	v    kit.Variant
}{{"windows", kit.Windows}, {"gnome", kit.GNOME}, {"macos", kit.MacOS}}

func mountTitleBar(t *testing.T, root tela.Component, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("w-full bg-background text-foreground"), root)
	}, append([]telatest.Option{telatest.Size(600, 120), telatest.Theme(kit.Theme())}, opts...)...)
}

// Every look has the three buttons by name, in a group, with the maximize
// button turning into Restore while the window is maximized.
func TestWindowControlsNames(t *testing.T) {
	for _, l := range titleBarLooks {
		t.Run(l.name, func(t *testing.T) {
			a := mountTitleBar(t, kit.WindowControls{Variant: l.v, State: stActive})
			a.ExpectRole("group", "Window controls")
			a.ExpectCount(btnMinimize, 1)
			a.ExpectCount(btnMaximize, 1)
			a.ExpectCount(btnClose, 1)
			a.ExpectCount(btnRestore, 0)

			m := mountTitleBar(t, kit.WindowControls{Variant: l.v, State: stMaximized})
			m.ExpectCount(btnMaximize, 0)
			m.ExpectCount(btnRestore, 1)
		})
	}
}

// HideMinimize and HideMaximize leave a button out; Close stays. The names
// can be translated.
func TestWindowControlsOptions(t *testing.T) {
	a := mountTitleBar(t, kit.WindowControls{Variant: kit.Windows, State: stActive, HideMinimize: true, HideMaximize: true})
	a.ExpectCount(btnMinimize, 0)
	a.ExpectCount(btnMaximize, 0)
	a.ExpectCount(btnClose, 1)

	b := mountTitleBar(t, kit.WindowControls{Variant: kit.GNOME, State: stMaximized, AriaLabel: "Controles de la ventana",
		MinimizeLabel: "Minimizar", MaximizeLabel: "Maximizar", RestoreLabel: "Restaurar", CloseLabel: "Cerrar"})
	b.ExpectRole("group", "Controles de la ventana")
	for _, name := range []string{"Minimizar", "Restaurar", "Cerrar"} {
		b.ExpectRole("button", name)
	}
	b.ExpectCount(telatest.ByRole("button", "Maximizar"), 0)
}

// Aqua orders the dots close, minimize, zoom; the others minimize,
// maximize, close.
func TestWindowControlsOrder(t *testing.T) {
	order := func(v kit.Variant) []string {
		snap := mountTitleBar(t, kit.WindowControls{Variant: v, State: stActive}).AriaSnapshot()
		type at struct {
			name string
			i    int
		}
		var found []at
		for _, n := range []string{"Minimize", "Maximize", "Close"} {
			found = append(found, at{n, strings.Index(snap, `button "`+n+`"`)})
		}
		for i := range found {
			for j := i + 1; j < len(found); j++ {
				if found[j].i < found[i].i {
					found[i], found[j] = found[j], found[i]
				}
			}
		}
		names := make([]string, len(found))
		for i, f := range found {
			names[i] = f.name
		}
		return names
	}
	if got := strings.Join(order(kit.Windows), ","); got != "Minimize,Maximize,Close" {
		t.Errorf("Windows order %s", got)
	}
	if got := strings.Join(order(kit.GNOME), ","); got != "Minimize,Maximize,Close" {
		t.Errorf("GNOME order %s", got)
	}
	if got := strings.Join(order(kit.MacOS), ","); got != "Close,Minimize,Maximize" {
		t.Errorf("MacOS order %s", got)
	}
}

// An unknown look panics in dev mode with the ones there are.
func TestWindowControlsUnknownVariant(t *testing.T) {
	defer func() {
		msg := fmt.Sprint(recover())
		if !strings.Contains(msg, `"windows"`) || !strings.Contains(msg, "WindowControls.Variant") {
			t.Errorf("panic %q does not name the looks", msg)
		}
	}()
	telatest.Mount(t, kit.WindowControls{Variant: "glass"})
}

// The slots are all there, and the window controls are on the side the
// look keeps them: the end, except macOS.
func TestTitleBarSides(t *testing.T) {
	for _, l := range titleBarLooks {
		t.Run(l.name, func(t *testing.T) {
			a := mountTitleBar(t, kit.TitleBar{
				Variant: l.v, State: stActive,
				Leading:  kit.Button{Variant: kit.Ghost, Size: kit.IconSm, AriaLabel: "Menu"},
				Title:    h.Text("Notes"),
				Children: kit.Input{Type: "search", AriaLabel: "Search"},
				Trailing: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, AriaLabel: "Share"},
			})
			a.ExpectText("Notes")
			for _, name := range []string{"Menu", "Share"} {
				a.ExpectRole("button", name)
			}
			a.ExpectRole("searchbox", "Search")
			closeX := a.Bounds(btnClose).X
			titleX := a.Bounds(telatest.ByRole("button", "Menu")).X
			atStart := l.v == kit.MacOS
			if atStart != (closeX < titleX) {
				t.Errorf("close at x=%v, menu at x=%v: controls at start = %v, want %v", closeX, titleX, closeX < titleX, atStart)
			}
		})
	}
}

// ControlsSide overrides the look's side.
func TestTitleBarControlsSide(t *testing.T) {
	bar := func(v kit.Variant, side string) float32 {
		a := mountTitleBar(t, kit.TitleBar{Variant: v, State: stActive, ControlsSide: side, Title: h.Text("Notes")})
		return a.Bounds(btnClose).X
	}
	if x := bar(kit.MacOS, "end"); x < 300 {
		t.Errorf("macOS with ControlsSide end: close at x=%v, want the right half", x)
	}
	if x := bar(kit.Windows, "start"); x > 300 {
		t.Errorf("Windows with ControlsSide start: close at x=%v, want the left half", x)
	}
	defer func() {
		if msg := fmt.Sprint(recover()); !strings.Contains(msg, "ControlsSide") {
			t.Errorf("panic %q does not name the field", msg)
		}
	}()
	bar(kit.Windows, "left")
}

// Under rtl the bar mirrors: the controls move to the other side.
func TestTitleBarRTL(t *testing.T) {
	for _, l := range titleBarLooks {
		t.Run(l.name, func(t *testing.T) {
			bar := kit.TitleBar{Variant: l.v, State: stActive, Title: h.Text("Notes")}
			ltr := mountTitleBar(t, bar).Bounds(btnClose).X
			rtl := mountTitleBar(t, h.Div(h.Dir("rtl"), bar)).Bounds(btnClose).X
			if (ltr < 300) == (rtl < 300) {
				t.Errorf("close at x=%v in ltr and x=%v in rtl: the bar did not mirror", ltr, rtl)
			}
		})
	}
}

// A window that is not focused dims the bar and a maximized one loses the
// outline: both change the pixels.
func TestTitleBarStates(t *testing.T) {
	for _, l := range titleBarLooks {
		t.Run(l.name, func(t *testing.T) {
			shot := func(st *tela.WindowState) []byte {
				return mountTitleBar(t, kit.TitleBar{Variant: l.v, State: st, Title: h.Text("Notes")}).Screenshot().Pix
			}
			if bytes.Equal(shot(stActive), shot(stInactive)) {
				t.Error("the inactive bar looks like the active one")
			}
			if bytes.Equal(shot(stActive), shot(stMaximized)) {
				t.Error("the maximized bar looks like the windowed one")
			}
		})
	}
}

// A control button is a real button with the window-control prop: in a
// frameless window a click minimizes, maximizes and restores, and the name
// of the button follows.
func TestTitleBarWindowActions(t *testing.T) {
	theme := kit.Theme()
	a := telatest.MountApp(t, tela.App{
		Width: 600, Height: 200, Frameless: true, Theme: &theme,
		Root: tela.C(func() tela.Node {
			return h.Div(h.Class("w-full bg-background text-foreground"),
				kit.TitleBar{Variant: kit.Windows, Title: h.Text("Notes"),
					Trailing: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, AriaLabel: "Share"}})
		}),
	})
	a.Expect(btnMaximize, telatest.HasName("Maximize"))
	a.Click(btnMaximize)
	a.ExpectRole("button", "Restore")
	a.ExpectCount(btnMaximize, 0)
	a.Click(btnRestore)
	a.ExpectRole("button", "Maximize")
	a.Click(btnMinimize) // nothing to see: it must not panic
}

// The accessible tree of a bar: its buttons and no more roles than those
// of its slots (the bar itself is a generic box).
func TestTitleBarAriaSnapshot(t *testing.T) {
	a := mountTitleBar(t, kit.TitleBar{Variant: kit.Windows, State: stActive, Title: h.Text("Notes"),
		Trailing: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, AriaLabel: "Share"}})
	snap := a.AriaSnapshot()
	for _, want := range []string{`button "Share"`, `group "Window controls"`, `button "Minimize"`, `button "Maximize"`, `button "Close"`} {
		if !strings.Contains(snap, want) {
			t.Errorf("aria snapshot lacks %s:\n%s", want, snap)
		}
	}
}

// The bar's hit regions as the OS gets them: the bar is a caption, the
// slots' buttons are holes in it, and the three controls are what they
// say. Windows' bar at 600 px: the controls are 46 px each at the end
// (minimize 462-508, maximize 508-554, close 554-600), the trailing
// button just before them.
func TestTitleBarHitRegions(t *testing.T) {
	b := headless.New(headless.Options{Size: platform.Size{W: 600, H: 200}})
	theme := kit.Theme()
	host, err := core.NewHost(b, core.Options{
		Frameless: true,
		Config:    core.Config{Dev: true, IDPrefix: "tt-", Theme: (*core.Theme)(&theme)},
		Root: core.NodeOf(tela.C(func() tela.Node {
			return h.Div(h.Class("w-full bg-background text-foreground"),
				kit.TitleBar{Variant: kit.Windows, Title: h.Text("Notes"),
					Trailing: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, AriaLabel: "Share"}})
		})),
	})
	if err != nil {
		t.Fatal(err)
	}
	win := host.Window().(*headless.Window)
	win.SetFocused(true)
	for range 50 {
		b.Drain(host)
		if !b.Pending() && !host.Tree().Dirty() && !host.NeedsFrame() {
			break
		}
		b.Advance(b.FrameInterval())
	}
	regions := win.HitRegions()
	for _, c := range []struct {
		x    float32
		want platform.HitKind
	}{
		{30, platform.HitCaption},
		{300, platform.HitCaption},
		{438, platform.HitClient}, // the trailing button is a hole in the caption
		{485, platform.HitMinimize},
		{531, platform.HitMaximize},
		{577, platform.HitClose},
	} {
		if got := platform.HitTest(regions, platform.Point{X: c.x, Y: 16}); got != c.want {
			t.Errorf("hit test at x=%v: %v, want %v (regions %v)", c.x, got, c.want, regions)
		}
	}
}
