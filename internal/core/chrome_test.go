package core

import (
	"image/png"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// newChromeHarness is newWinHarness on a fake window manager that draws no
// title bars (GNOME on Wayland) when csd is set.
func newChromeHarness(t *testing.T, csd bool, settings platform.SystemSettings, opts Options) *winHarness {
	t.Helper()
	b := headless.New(headless.Options{Size: platform.Size{W: 400, H: 300}, NoServerDecorations: csd, Settings: settings})
	if opts.Fonts == nil {
		opts.Fonts = testFonts
	}
	h, err := NewHost(b, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.stopTimer)
	x := &winHarness{t: t, b: b, h: h}
	x.drain()
	return x
}

// barShown reports whether tr shows the fallback title bar.
func barShown(tr *Tree) bool { return tr.chrome.shown && len(tr.root.children) == 2 }

// The fallback title bar is shown on a framed window that reports
// client-side decorations, and nowhere else (ADR-238).
func TestFallbackTitleBarShown(t *testing.T) {
	content := div(Class("h-full"), Key("content"), Text("Hello"))
	for _, c := range []struct {
		name string
		csd  bool
		spec WindowSpec
		want bool
	}{
		{"server-side decorations", false, WindowSpec{Title: "App"}, false},
		{"client-side decorations", true, WindowSpec{Title: "App"}, true},
		{"frameless", true, WindowSpec{Title: "App", Frameless: true}, false},
		{"title bar overlay", true, WindowSpec{Title: "App", TitleBarOverlay: true}, false},
		{"frameless without csd", false, WindowSpec{Title: "App", Frameless: true}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			x := newChromeHarness(t, c.csd, platform.SystemSettings{}, Options{Window: c.spec, Root: content})
			tr := x.h.Tree()
			if got := barShown(tr); got != c.want {
				t.Fatalf("bar shown = %v, want %v\n%s", got, c.want, tr.AriaSnapshot())
			}
			n := mustFind(t, tr, "content")
			top, height := float32(0), float32(300)
			if c.want {
				top, height = 36, 264
			}
			if n.Rect.Y0 != top || n.Rect.Height() != height {
				t.Errorf("content at y=%v, %v high; want y=%v, %v high", n.Rect.Y0, n.Rect.Height(), top, height)
			}
			snap := tr.AriaSnapshot()
			if has := strings.Contains(snap, `button "Close"`); has != c.want {
				t.Errorf("aria snapshot has a Close button: %v, want %v\n%s", has, c.want, snap)
			}
		})
	}
}

// The content below the bar sees the rest of the window as its viewport:
// h-screen, vh units, fixed boxes and portal layers start below the bar.
func TestFallbackTitleBarViewport(t *testing.T) {
	root := div(
		div(Class("h-screen w-10"), Key("screen")),
		div(Class("fixed inset-x-0 top-0 h-5"), Key("fixed")),
		Portal(div(Class("fixed inset-0"), Key("portal"))),
	)
	x := newChromeHarness(t, true, platform.SystemSettings{}, Options{Window: WindowSpec{Title: "App"}, Root: root})
	tr := x.h.Tree()
	if !barShown(tr) {
		t.Fatal("no bar")
	}
	if n := mustFind(t, tr, "screen"); n.Rect.Y0 != 36 || n.Rect.Height() != 264 {
		t.Errorf("h-screen at y=%v, %v high; want y=36, 264 high", n.Rect.Y0, n.Rect.Height())
	}
	if n := mustFind(t, tr, "fixed"); n.Rect.Y0 != 36 {
		t.Errorf("fixed top-0 at y=%v, want 36", n.Rect.Y0)
	}
	if n := mustFind(t, tr, "portal"); n.Rect.Y0 != 36 || n.Rect.Height() != 264 {
		t.Errorf("portal content at y=%v, %v high; want y=36, 264 high", n.Rect.Y0, n.Rect.Height())
	}
	// The root does not scroll: the content fits.
	if s := tr.rootBox.scr; s != nil && s.max.Y > 0 {
		t.Errorf("the root scrolls by %v", s.max.Y)
	}
	// The bar is the window's top band: a caption with the buttons as
	// holes, and the root under it (a press at y=20 hits the bar).
	main := x.windows()[0]
	regs := main.HitRegions()
	if len(regs) != 4 || regs[0].Kind != platform.HitCaption || regs[0].Rect != (platform.Rect{W: 400, H: 36}) {
		t.Fatalf("hit regions %v, want the bar's caption and three buttons", regs)
	}
	kinds := []platform.HitKind{regs[1].Kind, regs[2].Kind, regs[3].Kind}
	if !slices.Equal(kinds, []platform.HitKind{platform.HitMinimize, platform.HitMaximize, platform.HitClose}) {
		t.Errorf("button regions %v", regs[1:])
	}
	if r := regs[3].Rect; r.X != 400-4-28 || r.Y != 3.5 || r.W != 28 || r.H != 28 {
		t.Errorf("close button at %v, want 28 px at the end, centered", r)
	}
	// A press on the bar's middle is the OS's (a move).
	main.Inject(down(200, 20))
	main.Inject(up(200, 20))
	x.drain()
	if main.CaptionDrags() != 1 {
		t.Errorf("%d caption drags, want 1", main.CaptionDrags())
	}
	// The buttons act by themselves.
	x.click(main, float32(regs[2].Rect.X+10), 18)
	if !main.OSState().Maximized {
		t.Error("the maximize button did not maximize")
	}
	if snap := tr.AriaSnapshot(); !strings.Contains(snap, `button "Restore"`) {
		t.Errorf("maximized, no Restore button:\n%s", snap)
	}
}

// The bar hides while the window is fullscreen, and comes back.
func TestFallbackTitleBarFullscreen(t *testing.T) {
	x := newChromeHarness(t, true, platform.SystemSettings{}, Options{Window: WindowSpec{Title: "App"}, Root: div(Class("h-full"), Key("content"))})
	tr := x.h.Tree()
	main := x.windows()[0]
	main.SetFullscreen(true)
	x.drain()
	if barShown(tr) {
		t.Fatal("the bar is shown in fullscreen")
	}
	if n := mustFind(t, tr, "content"); n.Rect.Y0 != 0 {
		t.Errorf("fullscreen content at y=%v, want 0", n.Rect.Y0)
	}
	main.SetFullscreen(false)
	x.drain()
	if !barShown(tr) {
		t.Fatal("the bar did not come back after fullscreen")
	}
	if n := mustFind(t, tr, "content"); n.Rect.Y0 != 36 {
		t.Errorf("content at y=%v, want 36", n.Rect.Y0)
	}
}

// An app with its own title bar (a WindowDrag area) hides the fallback
// in the same frame its bar mounts, and gets it back in the frame it
// unmounts: no frame shows both bars, or none. The app's content keeps
// its state through both.
func TestFallbackTitleBarHidesForAppBar(t *testing.T) {
	var setOwn func(bool)
	var mounts int
	counter := Component(func() Node {
		UseMount(func() func() { mounts++; return nil })
		return div(Key("counter"), Text("count"))
	})
	app := Component(func() Node {
		own, set := UseState(false)
		setOwn = set.Set
		var bar Node
		if own {
			bar = div(Class("h-10"), WindowDrag(), Key("own"), Text("My bar"))
		}
		return div(Class("flex h-full flex-col"), bar, counter)
	})
	x := newChromeHarness(t, true, platform.SystemSettings{}, Options{Window: WindowSpec{Title: "App"}, Root: app})
	tr := x.h.Tree()
	if !barShown(tr) {
		t.Fatal("no fallback bar")
	}
	var bad []string
	frames := 0
	x.h.AfterFrame = func() {
		frames++
		if tr.chrome.shown == (tr.chrome.drags > 0) {
			bad = append(bad, strings.TrimSpace(tr.AriaSnapshot()))
		}
	}
	setOwn(true)
	x.drain()
	if barShown(tr) {
		t.Fatal("the fallback bar stays with the app's own bar")
	}
	if n := mustFind(t, tr, "own"); n.Rect.Y0 != 0 {
		t.Errorf("the app's bar at y=%v, want 0 (the top of the window)", n.Rect.Y0)
	}
	setOwn(false)
	x.drain()
	if !barShown(tr) {
		t.Fatal("the fallback bar did not come back")
	}
	if n := mustFind(t, tr, "counter"); n.Rect.Y0 != 36 {
		t.Errorf("content at y=%v, want 36", n.Rect.Y0)
	}
	if mounts != 1 {
		t.Errorf("the content mounted %d times, want once", mounts)
	}
	if frames < 2 {
		t.Errorf("%d frames, want one per change at least", frames)
	}
	if len(bad) > 0 {
		t.Errorf("frames with both bars or none:\n%s", strings.Join(bad, "\n---\n"))
	}
}

// An app that draws its bar from the first build never shows the
// fallback: not even one frame.
func TestFallbackTitleBarAppBarFromStart(t *testing.T) {
	var presented []bool
	root := div(Class("flex h-full flex-col"),
		div(Class("h-10"), WindowDrag(), Text("My bar")),
		div(Class("grow")))
	b := headless.New(headless.Options{Size: platform.Size{W: 400, H: 300}, NoServerDecorations: true})
	h, err := NewHost(b, Options{Config: Config{Fonts: testFonts}, Window: WindowSpec{Title: "App"}, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.stopTimer)
	h.AfterFrame = func() { presented = append(presented, h.Tree().chrome.shown) }
	x := &winHarness{t: t, b: b, h: h}
	x.drain()
	if len(presented) == 0 {
		t.Fatal("no frame")
	}
	if slices.Contains(presented, true) {
		t.Errorf("frames with the fallback bar: %v", presented)
	}
}

// A child window gets its own fallback bar, with its title, which follows
// the Window's Title; a modal one has only a close button.
func TestFallbackTitleBarChildWindow(t *testing.T) {
	var setTitle func(string)
	app := Component(func() Node {
		title, set := UseState("Prefs")
		setTitle = set.Set
		return div(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: title}, Root: div(Text("child")), OnClose: func() {}}),
			win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "Dialog"}, Modal: true, Root: div(Text("modal")), OnClose: func() {}}))
	})
	x := newChromeHarness(t, true, platform.SystemSettings{}, Options{Window: WindowSpec{Title: "Main"}, Root: app})
	child, modal := x.child(0), x.child(1)
	snap := x.snapshot(child)
	for _, want := range []string{"Prefs", `button "Minimize"`, `button "Maximize"`, `button "Close"`} {
		if !strings.Contains(snap, want) {
			t.Errorf("child window lacks %s:\n%s", want, snap)
		}
	}
	setTitle("Settings")
	x.b.Wake()
	x.drain()
	if snap := x.snapshot(child); !strings.Contains(snap, "Settings") {
		t.Errorf("the bar did not follow the title:\n%s", snap)
	}
	snap = x.snapshot(modal)
	if strings.Contains(snap, "Minimize") || strings.Contains(snap, "Maximize") || !strings.Contains(snap, `button "Close"`) {
		t.Errorf("modal window's bar:\n%s", snap)
	}
}

// The buttons follow the desktop's button layout, and its changes.
func TestFallbackTitleBarButtonLayout(t *testing.T) {
	x := newChromeHarness(t, true, platform.SystemSettings{TitleBarButtons: "close,minimize:"}, Options{Window: WindowSpec{Title: "App"}, Root: div()})
	main := x.windows()[0]
	regs := main.HitRegions()
	if len(regs) != 3 || regs[1].Kind != platform.HitClose || regs[2].Kind != platform.HitMinimize || regs[1].Rect.X != 4 {
		t.Fatalf("hit regions %v, want close then minimize at the start", regs)
	}
	x.b.SetSettings(platform.SystemSettings{TitleBarButtons: "appmenu:close"})
	x.drain()
	regs = main.HitRegions()
	if len(regs) != 2 || regs[1].Kind != platform.HitClose || regs[1].Rect.X != 400-4-28 {
		t.Fatalf("hit regions %v, want close alone at the end", regs)
	}
}

func TestParseButtonLayout(t *testing.T) {
	const (
		mn = WindowMinimize
		mx = WindowMaximize
		cl = WindowClose
	)
	for _, c := range []struct {
		in         string
		start, end []WindowControl
	}{
		{"", nil, []WindowControl{mn, mx, cl}},
		{"  ", nil, []WindowControl{mn, mx, cl}},
		{"appmenu:close", nil, []WindowControl{cl}},
		{"close,minimize,maximize:", []WindowControl{cl, mn, mx}, nil},
		{"close:minimize,maximize", []WindowControl{cl}, []WindowControl{mn, mx}},
		{"icon,menu:spacer,minimize, maximize ,close", nil, []WindowControl{mn, mx, cl}},
		{"close,minimize", []WindowControl{cl, mn}, nil},
		{"close:close,maximize:minimize", []WindowControl{cl}, []WindowControl{mx}},
		{"appmenu:", nil, nil},
		{"bogus", nil, nil},
	} {
		l := parseButtonLayout(c.in)
		if !slices.Equal(l.start, c.start) || !slices.Equal(l.end, c.end) {
			t.Errorf("parseButtonLayout(%q) = %v : %v, want %v : %v", c.in, l.start, l.end, c.start, c.end)
		}
	}
}

// The bar dims while the window is not focused.
func TestFallbackTitleBarFocus(t *testing.T) {
	x := newChromeHarness(t, true, platform.SystemSettings{}, Options{Window: WindowSpec{Title: "App"}, Root: div()})
	tr := x.h.Tree()
	main := x.windows()[0]
	classes := func() string {
		for _, c := range tr.root.children[1].children {
			if c.kind == elemPrimitive && c.node.d.chrome {
				return strings.Join(c.node.d.classes, " ")
			}
		}
		t.Fatal("no bar element")
		return ""
	}
	main.Inject(platform.FocusEvent{Focused: true})
	x.drain()
	if cl := classes(); strings.Contains(cl, "/60") {
		t.Errorf("focused bar dimmed: %s", cl)
	}
	main.Inject(platform.FocusEvent{Focused: false})
	x.drain()
	if cl := classes(); !strings.Contains(cl, "text-muted-foreground/60") {
		t.Errorf("unfocused bar not dimmed: %s", cl)
	}
}

// writeChromePNG writes the main window's framebuffer for a look, when
// TELA_CHROME_PNG names a directory (development aid).
func writeChromePNG(t *testing.T, x *winHarness, name string) {
	dir := os.Getenv("TELA_CHROME_PNG")
	if dir == "" {
		return
	}
	fb := x.h.main.fb
	f, err := os.Create(dir + "/" + name + ".png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, fb); err != nil {
		t.Fatal(err)
	}
}

func TestFallbackTitleBarLook(t *testing.T) {
	x := newChromeHarness(t, true, platform.SystemSettings{}, Options{Window: WindowSpec{Title: "Notes"}, Root: div(Class("h-full p-4"), Text("Content"))})
	x.windows()[0].Inject(platform.FocusEvent{Focused: true})
	x.drain()
	writeChromePNG(t, x, "chrome")
}

// The bar follows the direction of the window's content (ADR-252): the
// outermost Dir along the wrappers that are the whole content, through
// components, fragments and portals; not a Dir on a part of the screen.
func TestFallbackTitleBarDirection(t *testing.T) {
	rtlPage := func(c ...Child) Node { return div(append([]Child{Dir("rtl"), Class("h-full")}, c...)...) }
	provider := func(kids ...Child) Node { // a component that renders a fragment
		return Component(func() Node { return Fragment(kids...) })
	}
	for _, c := range []struct {
		name string
		root Node
		rtl  bool
	}{
		{"no Dir", div(Class("h-full"), Text("Hello")), false},
		{"Dir on the outermost element", rtlPage(Text("مرحبا")), true},
		{"Dir on the only child of the outermost element", div(Class("h-full"), rtlPage(Text("مرحبا"))), true},
		{"Dir below components, a fragment and a portal",
			Component(func() Node {
				return provider(provider(div(Class("h-full"), rtlPage(Text("مرحبا")))), Portal(div(Text("toasts"))))
			}), true},
		{"the outermost Dir wins", div(Dir("ltr"), Class("h-full"), rtlPage(Text("مرحبا"))), false},
		{"auto, resolved by its text", div(Class("h-full"), div(Dir("auto"), Class("h-full"), Text("مرحبا"))), true},
		{"Dir on one of several children", div(Class("flex h-full"), div(Text("Side")), rtlPage(Text("مرحبا"))), false},
		{"Dir below text", div(Class("h-full"), Text("Title"), rtlPage(Text("مرحبا"))), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			x := newChromeHarness(t, true, platform.SystemSettings{}, Options{Window: WindowSpec{Title: "App"}, Root: c.root})
			tr := x.h.Tree()
			if !barShown(tr) {
				t.Fatal("no bar")
			}
			if tr.chrome.props.rtl != c.rtl {
				t.Errorf("bar rtl = %v, want %v", tr.chrome.props.rtl, c.rtl)
			}
		})
	}

	// The bar follows a change of that Dir, and the close button moves to
	// the start (the left) under rtl.
	var setDir func(string)
	app := Component(func() Node {
		dir, set := UseState("ltr")
		setDir = set.Set
		return div(Class("h-full"), div(Dir(dir), Class("h-full"), Text("Hello")))
	})
	x := newChromeHarness(t, true, platform.SystemSettings{}, Options{Window: WindowSpec{Title: "App"}, Root: app})
	tr := x.h.Tree()
	closeX := func() float32 {
		t.Helper()
		for _, r := range x.windows()[0].HitRegions() {
			if r.Kind == platform.HitClose {
				return r.Rect.X
			}
		}
		t.Fatal("no close button region")
		return 0
	}
	if tr.chrome.props.rtl || closeX() < 200 {
		t.Fatalf("ltr content: rtl %v, close at x=%v; want ltr, at the right", tr.chrome.props.rtl, closeX())
	}
	setDir("rtl")
	x.drain()
	if !tr.chrome.props.rtl || closeX() > 200 {
		t.Errorf("rtl content: rtl %v, close at x=%v; want rtl, at the left", tr.chrome.props.rtl, closeX())
	}
}

// A click on the bar's drag area is a press outside the top overlay
// (ADR-252): its outside callbacks run and a dismissable dialog closes,
// modal or not; a drag on it moves the window and is no press.
func TestFallbackTitleBarOutsidePress(t *testing.T) {
	for _, modal := range []bool{false, true} {
		var starts, outs int
		var setOpen func(bool)
		app := Component(func() Node {
			open, set := UseState(true)
			setOpen = set.Set
			var dialog Node
			if open {
				dialog = Portal(div(Key("dialog"), Class("fixed left-20 top-20 h-20 w-40"), Overlay(OverlayOptions{
					Modal:                  modal,
					OnInteractOutsideStart: func(PointerEvent) { starts++ },
					OnInteractOutside:      func(PointerEvent) { outs++; set.Set(false) },
				}), Text("Dialog")))
			}
			return div(Class("h-full"), Text("Content"), dialog)
		})
		x := newChromeHarness(t, true, platform.SystemSettings{}, Options{Window: WindowSpec{Title: "App"}, Root: app})
		tr := x.h.Tree()
		main := x.windows()[0]
		if !barShown(tr) {
			t.Fatal("no bar")
		}
		open := func() bool {
			root := tr.Inspect()
			return find(&root, "dialog") != nil
		}
		// A drag on the bar moves the window: the dialog stays.
		main.Inject(move(150, 18))
		main.Inject(down(150, 18))
		main.Inject(moveHeld(180, 40))
		main.Inject(ptr(platform.PointerUp, 180, 40, platform.ButtonPrimary, 0))
		x.drain()
		if main.CaptionDrags() != 1 || starts != 0 || outs != 0 || !open() {
			t.Fatalf("modal %v: after a drag: %d caption drags, %d starts, %d outside presses, open %v; want a drag and no press",
				modal, main.CaptionDrags(), starts, outs, open())
		}
		// A click on it closes the dialog, both callbacks running once.
		x.b.Advance(time.Second) // not a double click with the drag's press
		x.click(main, 150, 18)
		if starts != 1 || outs != 1 || open() {
			t.Errorf("modal %v: after a click on the bar: %d starts, %d outside presses, open %v; want 1, 1 and closed", modal, starts, outs, open())
		}
		// Without an overlay a click does nothing.
		x.b.Advance(time.Second)
		x.click(main, 150, 18)
		if starts != 1 || outs != 1 || main.OSState().Maximized {
			t.Errorf("modal %v: a click with no overlay: %d starts, %d outside presses, maximized %v", modal, starts, outs, main.OSState().Maximized)
		}
		setOpen(true)
		x.drain()
		if !open() {
			t.Errorf("modal %v: the dialog did not open again", modal)
		}
	}
}
