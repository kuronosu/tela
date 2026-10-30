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

// Variant Plain (ADR-201) is the platform's arrangement with a quiet surface.
// Look picks the arrangement, so every one is tested here whatever the OS.
var plainLooks = []struct {
	name string
	look kit.Variant
}{{"windows", kit.Windows}, {"gnome", kit.GNOME}, {"macos", kit.MacOS}}

func plainBar(look kit.Variant, st *tela.WindowState) kit.TitleBar {
	return kit.TitleBar{
		Variant: kit.Plain, Look: look, State: st,
		Leading:  kit.Button{Variant: kit.Ghost, Size: kit.IconSm, AriaLabel: "Menu"},
		Title:    h.Text("Notes"),
		Trailing: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, AriaLabel: "Share"},
	}
}

// The bar paints nothing of its own: its pixels are the page's, at the top,
// in the middle and on the bottom edge, where the platform's bars have an
// outline, a background or a border. Windowed or maximized, active or not.
func TestPlainTitleBarPaintsNothing(t *testing.T) {
	for _, l := range plainLooks {
		for _, st := range []*tela.WindowState{stActive, stInactive, stMaximized} {
			t.Run(fmt.Sprint(l.name, st.Focused, st.Maximized), func(t *testing.T) {
				a := mountTitleBar(t, kit.TitleBar{Variant: kit.Plain, Look: l.look, State: st, Title: h.Text("Notes")})
				img := a.Screenshot()
				page := img.RGBAAt(300, 110)
				bar := a.Bounds(btnClose)
				for _, y := range []int{0, 1, int(bar.Y), int(bar.Y + bar.Height + 1)} {
					for _, x := range []int{3, 150, 450} {
						if got := img.RGBAAt(x, y); got != page {
							t.Fatalf("pixel (%d,%d) is %v, want the page's %v: the bar paints something", x, y, got, page)
						}
					}
				}
			})
		}
	}
}

// Plain keeps the platform's arrangement: the controls at the end, except
// the dots of macOS at the start, in the order of their look; the slots are
// all there; the title is where the look puts it.
func TestPlainTitleBarArrangement(t *testing.T) {
	for _, l := range plainLooks {
		t.Run(l.name, func(t *testing.T) {
			a := mountTitleBar(t, plainBar(l.look, stActive))
			a.ExpectText("Notes")
			a.ExpectRole("group", "Window controls")
			a.ExpectRole("button", "Menu")
			a.ExpectRole("button", "Share")
			a.ExpectCount(btnMinimize, 1)
			a.ExpectCount(btnMaximize, 1)
			closeX, menuX := a.Bounds(btnClose).X, a.Bounds(telatest.ByRole("button", "Menu")).X
			if atStart := l.look == kit.MacOS; atStart != (closeX < menuX) {
				t.Errorf("close at x=%v, menu at x=%v: controls at start = %v, want %v", closeX, menuX, closeX < menuX, atStart)
			}
			title := a.Bounds(telatest.ByText("Notes"))
			if centered := l.look != kit.Windows; centered != (title.X > 200 && title.X < 400) {
				t.Errorf("title at x=%v: centered = %v, want %v", title.X, !centered, centered)
			}
			// The same arrangement as the look's own bar.
			own := mountTitleBar(t, kit.TitleBar{Variant: l.look, State: stActive, Title: h.Text("Notes"),
				Leading: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, AriaLabel: "Menu"}}).Bounds(btnClose)
			if (closeX < 300) != (own.X < 300) {
				t.Errorf("close at x=%v, want the side of the look's own bar (x=%v)", closeX, own.X)
			}
		})
	}
	// The macOS dots are Aqua's order; the others minimize, maximize, close.
	snap := mountTitleBar(t, plainBar(kit.MacOS, stActive)).AriaSnapshot()
	if c, m := strings.Index(snap, `button "Close"`), strings.Index(snap, `button "Minimize"`); c > m || c < 0 {
		t.Errorf("macOS order: close at %d, minimize at %d", c, m)
	}
	snap = mountTitleBar(t, plainBar(kit.GNOME, stActive)).AriaSnapshot()
	if m, c := strings.Index(snap, `button "Minimize"`), strings.Index(snap, `button "Close"`); m > c || m < 0 {
		t.Errorf("GNOME order: minimize at %d, close at %d", m, c)
	}
}

// Without a Look Plain follows the platform the app runs on, like the zero
// Variant.
func TestPlainTitleBarDefaultLook(t *testing.T) {
	a := mountTitleBar(t, kit.TitleBar{Variant: kit.Plain, State: stActive, Title: h.Text("Notes")})
	b := mountTitleBar(t, kit.TitleBar{State: stActive, Title: h.Text("Notes")})
	if pa, pb := a.Bounds(btnClose), b.Bounds(btnClose); pa.X != pb.X {
		t.Errorf("Plain's close at x=%v, the platform's at x=%v", pa.X, pb.X)
	}
}

// The controls are ghost buttons that follow the theme tokens: an accent
// hover and a destructive hover on close, none of the look's own colors;
// the quiet bar still dims when the window is not focused.
func TestPlainWindowControlsClasses(t *testing.T) {
	for _, l := range plainLooks {
		t.Run(l.name, func(t *testing.T) {
			a := mountTitleBar(t, plainBar(l.look, stActive))
			hover, closeHover := "hover:bg-accent", "hover:bg-destructive/15"
			if l.look == kit.MacOS {
				hover, closeHover = "hover:bg-foreground/30", "hover:bg-destructive"
			}
			a.ExpectCount(btnMinimize.Filter(telatest.HasClass(hover)), 1)
			a.ExpectCount(btnClose.Filter(telatest.HasClass(closeHover)), 1)
			for _, bad := range []string{"hover:bg-[#c42b1c]", "bg-[#ff5f57]", "bg-foreground/7"} {
				a.ExpectCount(btnClose.Filter(telatest.HasClass(bad)), 0)
			}
			active := a.Screenshot().Pix
			if bytes.Equal(active, mountTitleBar(t, plainBar(l.look, stInactive)).Screenshot().Pix) {
				t.Error("the inactive plain bar looks like the active one")
			}
		})
	}
}

// Plain WindowControls alone, in each arrangement, with HideMinimize and the
// Restore name while maximized.
func TestPlainWindowControls(t *testing.T) {
	for _, l := range plainLooks {
		t.Run(l.name, func(t *testing.T) {
			a := mountTitleBar(t, kit.WindowControls{Variant: kit.Plain, Look: l.look, State: stActive})
			a.ExpectRole("group", "Window controls")
			a.ExpectCount(btnMinimize, 1)
			a.ExpectCount(btnMaximize, 1)
			a.ExpectCount(btnClose, 1)
			m := mountTitleBar(t, kit.WindowControls{Variant: kit.Plain, Look: l.look, State: stMaximized, HideMinimize: true})
			m.ExpectCount(btnMinimize, 0)
			m.ExpectCount(btnRestore, 1)
		})
	}
}

// An unknown Look panics in dev mode, naming the recipe it is missing from.
func TestPlainUnknownLook(t *testing.T) {
	defer func() {
		if msg := fmt.Sprint(recover()); !strings.Contains(msg, "WindowControls.Variant") {
			t.Errorf("panic %q does not name the field", msg)
		}
	}()
	telatest.Mount(t, kit.WindowControls{Variant: kit.Plain, Look: "glass"})
}

// ControlsSide and rtl work as in the other looks.
func TestPlainTitleBarSideAndRTL(t *testing.T) {
	bar := plainBar(kit.Windows, stActive)
	bar.ControlsSide = "start"
	if x := mountTitleBar(t, bar).Bounds(btnClose).X; x > 300 {
		t.Errorf("ControlsSide start: close at x=%v, want the left half", x)
	}
	for _, l := range plainLooks {
		ltr := mountTitleBar(t, plainBar(l.look, stActive)).Bounds(btnClose).X
		rtl := mountTitleBar(t, h.Div(h.Dir("rtl"), plainBar(l.look, stActive))).Bounds(btnClose).X
		if (ltr < 300) == (rtl < 300) {
			t.Errorf("%s: close at x=%v in ltr and x=%v in rtl: the bar did not mirror", l.name, ltr, rtl)
		}
	}
}

// The window actions work as in the other looks: in a frameless window a
// click maximizes and restores, and the accessible tree has the same
// buttons.
func TestPlainTitleBarWindowActions(t *testing.T) {
	theme := kit.Theme()
	a := telatest.MountApp(t, tela.App{
		Width: 600, Height: 200, Frameless: true, Theme: &theme,
		Root: tela.C(func() tela.Node {
			return h.Div(h.Class("w-full bg-background text-foreground"),
				kit.TitleBar{Variant: kit.Plain, Look: kit.Windows, Title: h.Text("Notes"),
					Trailing: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, AriaLabel: "Share"}})
		}),
	})
	a.Click(btnMaximize)
	a.ExpectRole("button", "Restore")
	a.Click(btnRestore)
	a.ExpectRole("button", "Maximize")
	a.Click(btnMinimize) // nothing to see: it must not panic

	snap := a.AriaSnapshot()
	for _, want := range []string{`button "Share"`, `group "Window controls"`, `button "Minimize"`, `button "Maximize"`, `button "Close"`} {
		if !strings.Contains(snap, want) {
			t.Errorf("aria snapshot lacks %s:\n%s", want, snap)
		}
	}
}

// The hit regions are the other looks': the bar is a caption, the buttons
// of the slots are holes in it, the three controls are what they say.
func TestPlainTitleBarHitRegions(t *testing.T) {
	b := headless.New(headless.Options{Size: platform.Size{W: 600, H: 200}})
	theme := kit.Theme()
	host, err := core.NewHost(b, core.Options{
		Window: core.WindowSpec{Frameless: true},
		Config: core.Config{Dev: true, IDPrefix: "tp-", Theme: (*core.Theme)(&theme)},
		Root: core.NodeOf(tela.C(func() tela.Node {
			return h.Div(h.Class("w-full bg-background text-foreground"),
				kit.TitleBar{Variant: kit.Plain, Look: kit.Windows, Title: h.Text("Notes"),
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
		{438, platform.HitClient},
		{485, platform.HitMinimize},
		{531, platform.HitMaximize},
		{577, platform.HitClose},
	} {
		if got := platform.HitTest(regions, platform.Point{X: c.x, Y: 16}); got != c.want {
			t.Errorf("hit test at x=%v: %v, want %v (regions %v)", c.x, got, c.want, regions)
		}
	}
}
