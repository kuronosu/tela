package core

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/style"
)

// System settings v2 (env.go, ADR-209, ADR-211): high contrast, the text
// scale and the accent color, from the platform's settings.

// contrast-more: follows the system's high contrast in every window,
// those opened before and after the change.
func TestContrastMoreFollowsTheSystem(t *testing.T) {
	box := func() Node { return div(Key("box"), Class("border border-gray-300 contrast-more:border-black")) }
	var setOpen Setter[bool]
	root := Component(func() Node {
		open, set := UseState(false)
		setOpen = set
		return div(box(), If(open, win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "C"}, OnClose: func() {}, Root: box()})))
	})
	x := newWinHarness(t, Options{Root: root})
	border := func(tr *Tree) gfx.Color { return keyed(tr, "box").box.cs.Border.Top.Color }
	gray := style.DefaultTheme().Colors["gray-300"]
	if c := border(x.h.Tree()); c != gray {
		t.Fatalf("border %v, want gray-300", c)
	}
	x.b.SetSettings(platform.SystemSettings{HighContrast: true})
	x.drain()
	if c := border(x.h.Tree()); c != gfx.Black {
		t.Errorf("high contrast: border %v, want black", c)
	}
	setOpen.Set(true)
	x.drain()
	ct := x.h.TreeOf(x.child(0))
	if c := border(ct); c != gfx.Black {
		t.Errorf("a window opened in high contrast: border %v, want black", c)
	}
	x.b.SetSettings(platform.SystemSettings{})
	x.drain()
	if a, b := border(x.h.Tree()), border(ct); a != gray || b != gray {
		t.Errorf("high contrast off: borders %v and %v, want gray-300", a, b)
	}
}

// remLayout is a screen whose sizes are in rem (w-16 h-4, text-base),
// in px (w-[100px] h-[10px]), and switched at the md: breakpoint (48rem).
func remLayout() Node {
	return div(Class("flex flex-col items-start"),
		div(Key("rem"), Class("w-16 h-4")),
		div(Key("px"), Class("w-[100px] h-[10px]")),
		p(Key("txt"), Class("text-base"), Text("Hello")),
		div(Key("bp"), Class("h-[1px] w-[10px] md:w-[20px]")),
	)
}

// sizes returns the laid-out sizes of remLayout's elements.
func sizes(tr *Tree) string {
	var b strings.Builder
	for _, k := range []string{"rem", "px", "txt", "bp"} {
		r := tr.elemBounds(keyed(tr, k))
		fmt.Fprintf(&b, "%s=%gx%g ", k, r.Width(), r.Height())
	}
	return strings.TrimSpace(b.String())
}

// The text scale multiplies rem like a browser's default font size: text,
// rem lengths and the rem breakpoints grow, px lengths do not; a change
// relays out everything, back and forth; the app can ignore it.
func TestTextScaleMultipliesRem(t *testing.T) {
	const (
		at1   = "rem=64x16 px=100x10 txt=%gx24 bp=20x1"
		at125 = "rem=80x20 px=100x10 txt=%gx30 bp=10x1"
	)
	bo := headless.Options{Size: platform.Size{W: 800, H: 300}}
	x := newBackendHarness(t, bo, Options{Root: remLayout()})
	tr := x.h.Tree()
	w1 := tr.elemBounds(keyed(tr, "txt")).Width()
	if got, want := sizes(tr), fmt.Sprintf(at1, w1); got != want {
		t.Fatalf("at scale 1: %s, want %s", got, want)
	}
	x.b.SetSettings(platform.SystemSettings{TextScale: 1.25})
	x.drain()
	if s := tr.TextScale(); s != 1.25 {
		t.Errorf("TextScale %g", s)
	}
	w125 := tr.elemBounds(keyed(tr, "txt")).Width()
	if got, want := sizes(tr), fmt.Sprintf(at125, w125); got != want || !(w125 > w1) {
		t.Errorf("at scale 1.25: %s, want %s (text wider than %g)", got, want, w1)
	}
	x.b.SetSettings(platform.SystemSettings{})
	x.drain()
	if got, want := sizes(tr), fmt.Sprintf(at1, w1); got != want {
		t.Errorf("back at scale 1: %s, want %s", got, want)
	}

	// Initial settings, and an app that ignores the scale.
	bo.Settings = platform.SystemSettings{TextScale: 1.25}
	y := newBackendHarness(t, bo, Options{Root: remLayout()})
	if got, want := sizes(y.h.Tree()), fmt.Sprintf(at125, w125); got != want {
		t.Errorf("started at scale 1.25: %s, want %s", got, want)
	}
	z := newBackendHarness(t, bo, Options{Config: Config{IgnoreTextScale: true}, Root: remLayout()})
	if got, want := sizes(z.h.Tree()), fmt.Sprintf(at1, w1); got != want || z.h.Tree().TextScale() != 1 {
		t.Errorf("IgnoreTextScale: %s (scale %g), want %s", got, z.h.Tree().TextScale(), want)
	}
	z.b.SetSettings(platform.SystemSettings{TextScale: 2})
	z.drain()
	if got, want := sizes(z.h.Tree()), fmt.Sprintf(at1, w1); got != want {
		t.Errorf("IgnoreTextScale after a change: %s, want %s", got, want)
	}
}

// A window opened while the text is scaled lays out at that scale too,
// and a theme set while the text is scaled keeps the scale.
func TestTextScaleInChildWindowsAndThemes(t *testing.T) {
	bo := headless.Options{Size: platform.Size{W: 800, H: 300}, Settings: platform.SystemSettings{TextScale: 1.5}}
	root := Component(func() Node {
		return div(Key("main"), Class("w-16 h-1"), win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "C"}, OnClose: func() {},
			Root: div(Key("child"), Class("w-16 h-1"))}))
	})
	x := newBackendHarness(t, bo, Options{Root: root})
	ct := x.h.TreeOf(x.child(0))
	if w := ct.elemBounds(keyed(ct, "child")).Width(); w != 96 {
		t.Errorf("child window: w-16 is %g, want 96", w)
	}
	x.h.Tree().SetTheme(Theme{Radius: 4})
	x.drain()
	for _, tc := range []struct {
		tr  *Tree
		key string
	}{{x.h.Tree(), "main"}, {ct, "child"}} {
		if w := tc.tr.elemBounds(keyed(tc.tr, tc.key)).Width(); w != 96 {
			t.Errorf("%s after SetTheme: w-16 is %g, want 96", tc.key, w)
		}
	}
}

// After a text scale change the focused field's caret, which the IME is
// told about, follows the larger text.
func TestTextScaleMovesTheCaret(t *testing.T) {
	root := div(input(Key("f"), AutoFocus(true), DefaultValue("abc"), Class("text-base")))
	x := newWinHarness(t, Options{Root: root})
	w := x.windows()[0]
	w.SetFocused(true)
	x.drain()
	st := w.TextInput()
	if st == nil || st.CursorArea.H <= 0 {
		t.Fatalf("no caret for the focused field: %+v", st)
	}
	h1 := st.CursorArea.H
	x.b.SetSettings(platform.SystemSettings{TextScale: 2})
	x.drain()
	if st := w.TextInput(); st == nil || st.CursorArea.H < 1.9*h1 {
		t.Errorf("caret area %+v after scale 2, want about twice %g tall", st, h1)
	}
}

// UseHighContrast, UseTextScale and UseAccentColor build their component
// again when the setting changes, and only theirs.
func TestSettingsHooksRebuild(t *testing.T) {
	readerBuilds, otherBuilds := 0, 0
	var got string
	reader := Component(func() Node {
		readerBuilds++
		hc, ts := UseHighContrast(), UseTextScale()
		ac, ok := UseAccentColor()
		got = fmt.Sprintf("hc=%v ts=%g accent=%v/%v", hc, ts, ac, ok)
		return div(Text(got))
	})
	other := Component(func() Node {
		otherBuilds++
		return div(Text("other"))
	})
	x := newWinHarness(t, Options{Root: div(reader, other)})
	if got != "hc=false ts=1 accent=#00000000/false" {
		t.Errorf("first build: %s", got)
	}
	r0, o0 := readerBuilds, otherBuilds
	x.b.SetSettings(platform.SystemSettings{HighContrast: true, TextScale: 1.5,
		AccentColor: color.NRGBA{R: 0x35, G: 0x84, B: 0xe4, A: 0xff}, HasAccentColor: true})
	x.drain()
	if got != "hc=true ts=1.5 accent=#3584e4/true" {
		t.Errorf("after the change: %s", got)
	}
	if readerBuilds != r0+1 || otherBuilds != o0 {
		t.Errorf("builds: reader %d (want %d), other %d (want %d)", readerBuilds, r0+1, otherBuilds, o0)
	}
	x.b.SetSettings(platform.SystemSettings{HighContrast: true, TextScale: 1.5,
		AccentColor: color.NRGBA{R: 0x35, G: 0x84, B: 0xe4, A: 0xff}, HasAccentColor: true})
	x.drain()
	if readerBuilds != r0+1 {
		t.Errorf("an equal setting rebuilt the reader (%d builds)", readerBuilds)
	}
}
