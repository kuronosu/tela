package kit_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// A TitleBarOverlay window (ADR-224) keeps the OS's window buttons over the
// content: the window's state reports their rectangle, and the bar draws no
// controls of its own and keeps the room free.

var (
	menuButton = telatest.ByRole("button", "Menu")
	shareBtn   = telatest.ByRole("button", "Share")
	group      = telatest.ByRole("group", "Window controls")
)

// overlayBar is a title bar with a leading and a trailing button.
func overlayBar(variant, look kit.Variant) kit.TitleBar {
	return kit.TitleBar{
		Variant: variant, Look: look,
		Leading:  kit.Button{Variant: kit.Ghost, Size: kit.Sm, Children: kitText("Menu"), AriaLabel: "Menu"},
		Title:    h.Text("Notes"),
		Trailing: kit.Button{Variant: kit.Ghost, Size: kit.Sm, Children: kitText("Share"), AriaLabel: "Share"},
	}
}

type kitText string

func (s kitText) Build() tela.Node { return h.Text(string(s)) }

// mountOverlay mounts the bar in a TitleBarOverlay window with the buttons
// of a Mac.
func mountOverlay(t *testing.T, root tela.Component, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return mountOverlayPage(t, "bg-background", false, root, opts...)
}

// mountOverlayPage is mountOverlay with a class for the window's surface,
// and, with marked, a translucent pill where the OS's buttons are (a
// golden's stand-in for the traffic lights, which the OS draws).
func mountOverlayPage(t *testing.T, surface string, marked bool, root tela.Component, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return telatest.MountFunc(t, func() tela.Node {
		var pill tela.Child
		if marked {
			r := telatest.MacTitleBarControls
			pill = h.Div(h.Class(fmt.Sprintf("absolute rounded-full bg-red-500/40 left-[%gpx] top-[%gpx] h-[%gpx] w-[%gpx]", r.X, r.Y, r.Height, r.Width)))
		}
		return h.Div(h.Class("relative flex h-full w-full flex-col text-foreground", surface), root,
			h.Div(h.Class("grow p-4 text-sm text-muted-foreground"), h.Text("The note's content sits under the same window.")), pill)
	}, append([]telatest.Option{telatest.Size(480, 160), telatest.Theme(kit.Theme()), telatest.TitleBarOverlay(),
		telatest.PlatformKeys("darwin")}, opts...)...)
}

// The window reports the rectangle, the bar draws no controls and its
// leading content starts after the OS's buttons, in every way to ask for the
// macOS arrangement.
func TestTitleBarLeavesRoomForTheOSButtons(t *testing.T) {
	right := telatest.MacTitleBarControls.X + telatest.MacTitleBarControls.Width
	for name, bar := range map[string]kit.TitleBar{
		"macos": overlayBar(kit.MacOS, ""),
		"plain": overlayBar(kit.Plain, kit.MacOS),
	} {
		t.Run(name, func(t *testing.T) {
			a := mountOverlay(t, bar)
			if got := a.WindowState().TitleBarControls; got != telatest.MacTitleBarControls {
				t.Fatalf("the window reports %v, want %v", got, telatest.MacTitleBarControls)
			}
			a.ExpectCount(group, 0)
			a.ExpectCount(btnClose, 0)
			if x := a.Bounds(menuButton).X; x < right {
				t.Errorf("the leading button starts at x=%v, inside the OS's buttons that end at %v", x, right)
			}
			if x := a.Bounds(menuButton).X; x > right+20 {
				t.Errorf("the leading button starts at x=%v, far from the OS's buttons that end at %v", x, right)
			}
			// The title is still centered in the bar.
			title := a.Bounds(telatest.ByText("Notes"))
			if mid := title.X + title.Width/2; mid < 230 || mid > 250 {
				t.Errorf("the title is centered at x=%v, want about 240", mid)
			}
		})
	}
}

// The OS moving its buttons moves the room.
func TestTitleBarFollowsTheOSButtons(t *testing.T) {
	a := mountOverlay(t, overlayBar(kit.MacOS, ""))
	before := a.Bounds(menuButton).X
	a.SetTitleBarControls(tela.Rect{X: 8, Y: 13, Width: 120, Height: 14})
	// The room's end moves from the Mac's buttons' to 8+120.
	more := 8 + 120 - (telatest.MacTitleBarControls.X + telatest.MacTitleBarControls.Width)
	if after := a.Bounds(menuButton).X; after != before+more {
		t.Errorf("the leading button moved from x=%v to x=%v, want %v more", before, after, more)
	}
	// Fullscreen takes them away: the bar draws its own controls again.
	a.SetTitleBarControls(tela.Rect{})
	a.ExpectCount(group, 1)
	if x := a.Bounds(btnClose).X; x > 30 {
		t.Errorf("the drawn controls are at x=%v, want them at the start", x)
	}
}

// Without the rectangle (Frameless, or an OS that cannot keep its buttons
// over the content) nothing changes: the controls are drawn.
func TestTitleBarWithoutOSButtonsIsUnchanged(t *testing.T) {
	// TitleBarOverlay on a platform that treats it as Frameless.
	a := telatest.MountFunc(t, func() tela.Node { return h.Div(h.Class("w-full"), overlayBar(kit.MacOS, "")) },
		telatest.Size(480, 160), telatest.Theme(kit.Theme()), telatest.TitleBarOverlay())
	if got := a.WindowState().TitleBarControls; got != (tela.Rect{}) {
		t.Fatalf("the window reports %v, want none", got)
	}
	a.ExpectCount(group, 1)
	a.ExpectCount(btnClose, 1)
	plain := a.Bounds(menuButton).X
	f := telatest.MountFunc(t, func() tela.Node { return h.Div(h.Class("w-full"), overlayBar(kit.MacOS, "")) },
		telatest.Size(480, 160), telatest.Theme(kit.Theme()), telatest.Frameless())
	if x := f.Bounds(menuButton).X; x != plain {
		t.Errorf("leading button at x=%v in an overlay window without buttons, x=%v in a frameless one", plain, x)
	}
}

// A rectangle at the right (an OS that puts its buttons at the end) keeps
// that side free and the leading side as it is; WindowControls alone draws
// nothing.
func TestTitleBarRoomAtTheEnd(t *testing.T) {
	a := mountOverlay(t, overlayBar(kit.Windows, ""), telatest.TitleBarControls(tela.Rect{X: 380, Y: 0, Width: 92, Height: 32}))
	a.ExpectCount(group, 0)
	if x := a.Bounds(menuButton).X; x > 20 {
		t.Errorf("the leading button is at x=%v, want it at the start", x)
	}
	if r := a.Bounds(shareBtn); r.X+r.Width > 480-92 {
		t.Errorf("the trailing button ends at x=%v, inside the OS's buttons that start at 388", r.X+r.Width)
	}
	w := mountOverlay(t, kit.WindowControls{})
	w.ExpectCount(group, 0)
	w.ExpectCount(btnClose, 0)
}

// Under rtl the OS's buttons stay at the window's left, which is the end of
// the bar.
func TestTitleBarRoomUnderRTL(t *testing.T) {
	a := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("w-full"), h.Dir("rtl"), overlayBar(kit.MacOS, ""))
	}, telatest.Size(480, 160), telatest.Theme(kit.Theme()), telatest.TitleBarOverlay(), telatest.PlatformKeys("darwin"))
	right := telatest.MacTitleBarControls.X + telatest.MacTitleBarControls.Width
	a.ExpectCount(group, 0)
	if r := a.Bounds(shareBtn); r.X < right {
		t.Errorf("the trailing button (at the left under rtl) starts at x=%v, inside the OS's buttons that end at %v", r.X, right)
	}
}

// The look of a macOS window with the traffic lights over the bar, and of
// the Plain bar of an app with a design of its own. Look at the PNGs: the
// leading content sits right of the buttons' rectangle and no dots are
// drawn.
func TestTitleBarOverlayGolden(t *testing.T) {
	mountOverlayPage(t, "bg-background", true, overlayBar(kit.MacOS, "")).GoldenScales("titlebar_overlay_macos", 1, 2)
	mountOverlayPage(t, "bg-muted", true, overlayBar(kit.Plain, kit.MacOS)).GoldenScales("titlebar_overlay_plain", 1, 2)
}

// The side kept free follows the rectangle, not the look: a Windows-look bar
// in a macOS window (the traffic lights at the left) keeps the left free,
// so its leading content does not sit under the buttons.
func TestTitleBarRoomFollowsTheRectangleNotTheLook(t *testing.T) {
	a := mountOverlay(t, overlayBar(kit.Windows, ""))
	right := telatest.MacTitleBarControls.X + telatest.MacTitleBarControls.Width
	a.ExpectCount(group, 0)
	if x := a.Bounds(menuButton).X; x < right {
		t.Errorf("the leading button starts at x=%v, inside the OS's buttons that end at %v", x, right)
	}
}

// windowsControls is where Windows 11 keeps its caption buttons over a
// TitleBarOverlay window 480 px wide at 100% (the win32 backend reports
// DWM's three 46 px buttons at the top right, ADR-236).
var windowsControls = tela.Rect{X: 480 - 138, Y: 0, Width: 138, Height: 32}

// mountWindowsOverlay mounts the bar in a TitleBarOverlay window with the
// Windows arrangement whose OS's buttons move to the end, as the window
// reports them on Windows 11, and a translucent box where the window says
// they are (the golden's stand-in for DWM's buttons, which the OS draws).
func mountWindowsOverlay(t *testing.T, surface string, root tela.Component) *telatest.App {
	t.Helper()
	a := telatest.MountFunc(t, func() tela.Node {
		var marker tela.Child
		if r := tela.UseWindow().State().TitleBarControls; r.Width > 0 {
			marker = h.Div(h.Class(fmt.Sprintf("absolute bg-red-500/40 left-[%gpx] top-[%gpx] h-[%gpx] w-[%gpx]", r.X, r.Y, r.Height, r.Width)))
		}
		return h.Div(h.Class("relative flex h-full w-full flex-col text-foreground", surface), root,
			h.Div(h.Class("grow p-4 text-sm text-muted-foreground"), h.Text("The note's content sits under the same window.")), marker)
	}, telatest.Size(480, 160), telatest.Theme(kit.Theme()), telatest.TitleBarOverlay(), telatest.PlatformKeys("windows"))
	// Until the OS reports buttons, the bar draws its own.
	a.ExpectCount(group, 1)
	a.SetTitleBarControls(windowsControls)
	return a
}

// The OS's buttons reported at the end of the bar (Windows 11): the bar
// draws no controls, the trailing content ends before the buttons and the
// leading content stays at the start; taking the buttons away (fullscreen)
// brings the drawn controls back at the end.
func TestTitleBarRoomAtTheEndFromTheOS(t *testing.T) {
	for name, bar := range map[string]kit.TitleBar{
		"windows": overlayBar(kit.Windows, ""),
		"plain":   overlayBar(kit.Plain, kit.Windows),
	} {
		t.Run(name, func(t *testing.T) {
			a := mountWindowsOverlay(t, "bg-background", bar)
			if got := a.WindowState().TitleBarControls; got != windowsControls {
				t.Fatalf("the window reports %v, want %v", got, windowsControls)
			}
			a.ExpectCount(group, 0)
			a.ExpectCount(btnClose, 0)
			share := a.Bounds(shareBtn)
			if end := share.X + share.Width; end > windowsControls.X {
				t.Errorf("the trailing button ends at x=%v, inside the OS's buttons that start at %v", end, windowsControls.X)
			} else if end < windowsControls.X-40 {
				t.Errorf("the trailing button ends at x=%v, far from the OS's buttons that start at %v", end, windowsControls.X)
			}
			if title := a.Bounds(telatest.ByText("Notes")); title.X+title.Width > windowsControls.X {
				t.Errorf("the title ends at x=%v, inside the OS's buttons", title.X+title.Width)
			}
			if x := a.Bounds(menuButton).X; x > 20 {
				t.Errorf("the leading button is at x=%v, want it at the start", x)
			}
			a.SetTitleBarControls(tela.Rect{})
			a.ExpectCount(group, 1)
			if r := a.Bounds(btnClose); r.X+r.Width < 470 {
				t.Errorf("the drawn close button ends at x=%v, want it at the end", r.X+r.Width)
			}
		})
	}
}

// The look of a Windows 11 window with DWM's buttons over the bar (the red
// box): Windows' bar and the Plain bar of an app with a design of its own.
// Look at the PNGs: the Share button ends left of the box and no drawn
// controls are under it.
func TestTitleBarOverlayWindowsGolden(t *testing.T) {
	mountWindowsOverlay(t, "bg-background", overlayBar(kit.Windows, "")).GoldenScales("titlebar_overlay_windows", 1, 2)
	mountWindowsOverlay(t, "bg-muted", overlayBar(kit.Plain, kit.Windows)).GoldenScales("titlebar_overlay_windows_plain", 1, 2)
}
