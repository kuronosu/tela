//go:build darwin

// This file is not part of any example. TestExamplesCocoa
// (cmd/tela/examples_cocoa_e2e_test.go) adds it to each example's package
// main with go build -overlay, so that the example's own main runs
// tela.Main on the Cocoa backend unchanged and this init captures what its
// window shows. It does nothing unless telaCaptureEnv names a directory.
//
// Every top-level name starts with telaCapture and every import has an
// unusual alias: the file shares the package scope of an example whose
// declarations it must not collide with. It uses only tela.Post and
// tela.Quit (public API) and internal/darwin, which the examples may import
// because they live in the tela module.
//
// What it writes into the directory:
//
//   - window.png: the content area of the app's window as the window
//     server composited it (CGWindowListCreateImage of its number, cropped
//     to the content view, the title bar off), physical pixels, alpha kept
//     (the window's rounded corners are transparent there);
//   - layer.png: the content view's layer rendered with renderInContext:,
//     what the window holds before the window server;
//   - unstable.png: when the window never settled, the pixels that changed
//     in any read back after the first full one (white): what an endless
//     animation or a clock moves, which no single headless frame can
//     match. A full read back is the first whose window server capture is
//     opaque outside the corners (the window has faded in), or the first
//     layer read back. Absent when the window settled;
//   - report.json: telaCaptureReport.
//
// It waits for the window to settle: it reads the window back every
// telaCaptureEvery and stops when no pixel changed for telaCaptureQuiet
// (longer than a caret blink cycle, 2 x 530 ms, so a blinking caret keeps
// it waiting until the caret stops, visible, 10 s after the focus) or at
// telaCaptureLimit after the window appeared. Then it quits the app with
// tela.Quit.

package main

import (
	tcjson "encoding/json"
	tcfmt "fmt"
	tcimage "image"
	tcpng "image/png"
	tcmath "math"
	tcos "os"
	tcfilepath "path/filepath"
	tcdebug "runtime/debug"
	tcstrings "strings"
	tctime "time"
	tcunsafe "unsafe"

	tctela "github.com/kuronosu/tela"
	tcdarwin "github.com/kuronosu/tela/internal/darwin"

	// The deterministic font set (ADR-244): the window must draw the same
	// text as the headless reference, which never uses the system's fonts.
	_ "github.com/kuronosu/tela/internal/stdfonts"
)

// telaCaptureEnv is the environment variable with the output directory;
// TestExamplesCocoa sets it (examplesCaptureEnv there).
const telaCaptureEnv = "TELA_EXAMPLE_CAPTURE"

const (
	telaCaptureEvery    = 100 * tctime.Millisecond
	telaCaptureQuiet    = 12 // captures without a change (1.2 s)
	telaCaptureMinShown = 1 * tctime.Second
	telaCaptureLimit    = 20 * tctime.Second
	telaCaptureFirst    = 60 * tctime.Second // for the first visible window
	telaCaptureUI       = 30 * tctime.Second // for one posted call
	telaCaptureQuitWait = 15 * tctime.Second // for main to return after tela.Quit
)

// telaCaptureReport is report.json. TestExamplesCocoa reads it into a struct
// with the same JSON names.
type telaCaptureReport struct {
	Error string `json:"error,omitempty"`

	FirstVisibleMS int64    `json:"firstVisibleMs"` // since init
	Captures       int      `json:"captures"`       // read backs since the window appeared
	Settled        bool     `json:"settled"`
	QuietMS        int64    `json:"quietMs"`      // how long the final pixels had not changed
	LastChangeMS   int64    `json:"lastChangeMs"` // since the window appeared
	Timeline       []string `json:"timeline"`     // the changes seen, oldest first (bounded)
	Unstable       int      `json:"unstable"`     // pixels in unstable.png
	BaseMS         int64    `json:"baseMs"`       // the first full read back, since the window appeared
	ShownMS        int64    `json:"shownMs"`      // from the window appearing to the final read back
	Time           string   `json:"time"`         // wall clock of the final capture, RFC 3339

	Windows []telaCaptureWindow `json:"windows"` // the app's windows at the final capture
	Window  telaCaptureWindow   `json:"window"`  // the captured one (the largest visible)

	Primary string          `json:"primary"` // the read back the settling followed: "window" or "layer"
	Capture telaCaptureRead `json:"capture"` // window.png
	Layer   telaCaptureRead `json:"layer"`   // layer.png

	Appearance       string `json:"appearance"` // NSApp.effectiveAppearance.name
	Dark             bool   `json:"dark"`
	ReduceMotion     bool   `json:"reduceMotion"`
	IncreaseContrast bool   `json:"increaseContrast"`
	Accent           string `json:"accent"` // controlAccentColor in sRGB, #rrggbb
	Active           bool   `json:"active"` // NSApp.isActive
}

type telaCaptureWindow struct {
	Number int     `json:"number"`
	Title  string  `json:"title"`
	W      float64 `json:"w"` // content view bounds, points
	H      float64 `json:"h"`
	Scale  float64 `json:"scale"` // backingScaleFactor
	Key    bool    `json:"key"`
	Style  uint64  `json:"styleMask"`
	PhysW  int     `json:"physW"`
	PhysH  int     `json:"physH"`
	FrameW float64 `json:"frameW"` // the window's frame, points (title bar included)
	// The visible frame of the window's screen (without the menu bar and
	// the Dock), points: AppKit keeps a window's frame inside it.
	ScreenW float64 `json:"screenW"`
	ScreenH float64 `json:"screenH"`
	FrameH  float64 `json:"frameH"`
	Visible bool    `json:"visible"`
	// Controls is the rectangle the window's standard buttons cover over
	// the content (x, y, w, h in points, y down from the content's top):
	// a title bar overlay's traffic lights, which the headless reference
	// leaves free. Zero for other windows.
	Controls [4]float64 `json:"controls"`
	// Class is the window's Objective-C class (NSStatusBarWindow for a
	// tray icon's).
	Class string `json:"class"`
}

type telaCaptureRead struct {
	Path string `json:"path"`
	OK   bool   `json:"ok"`
	Note string `json:"note"`
}

// telaCaptureShot is one read back, taken on the UI thread.
type telaCaptureShot struct {
	windows  []telaCaptureWindow
	win      telaCaptureWindow
	ok       bool // a visible window
	window   *tcimage.RGBA
	layer    *tcimage.RGBA
	capNote  string
	layNote  string
	settings telaCaptureReport // only the settings fields
	panicked string
}

var telaCaptureSel = struct {
	sharedApplication, windows, count, objectAtIndex, windowNumber, title, isVisible telaCaptureSEL
	contentView, bounds, frame, backingScaleFactor, layer, isGeometryFlipped         telaCaptureSEL
	renderInContext, isKeyWindow, styleMask, isActive, effectiveAppearance, name     telaCaptureSEL
	controlAccentColor, sRGBColorSpace, colorUsingColorSpace                         telaCaptureSEL
	redComponent, greenComponent, blueComponent                                      telaCaptureSEL
	sharedWorkspace, reduceMotion, increaseContrast                                  telaCaptureSEL
	screen, visibleFrame                                                             telaCaptureSEL
}{
	sharedApplication:    tcdarwin.Sel("sharedApplication"),
	windows:              tcdarwin.Sel("windows"),
	count:                tcdarwin.Sel("count"),
	objectAtIndex:        tcdarwin.Sel("objectAtIndex:"),
	windowNumber:         tcdarwin.Sel("windowNumber"),
	title:                tcdarwin.Sel("title"),
	isVisible:            tcdarwin.Sel("isVisible"),
	contentView:          tcdarwin.Sel("contentView"),
	bounds:               tcdarwin.Sel("bounds"),
	frame:                tcdarwin.Sel("frame"),
	backingScaleFactor:   tcdarwin.Sel("backingScaleFactor"),
	layer:                tcdarwin.Sel("layer"),
	isGeometryFlipped:    tcdarwin.Sel("isGeometryFlipped"),
	renderInContext:      tcdarwin.Sel("renderInContext:"),
	isKeyWindow:          tcdarwin.Sel("isKeyWindow"),
	styleMask:            tcdarwin.Sel("styleMask"),
	isActive:             tcdarwin.Sel("isActive"),
	effectiveAppearance:  tcdarwin.Sel("effectiveAppearance"),
	name:                 tcdarwin.Sel("name"),
	controlAccentColor:   tcdarwin.Sel("controlAccentColor"),
	sRGBColorSpace:       tcdarwin.Sel("sRGBColorSpace"),
	colorUsingColorSpace: tcdarwin.Sel("colorUsingColorSpace:"),
	redComponent:         tcdarwin.Sel("redComponent"),
	greenComponent:       tcdarwin.Sel("greenComponent"),
	blueComponent:        tcdarwin.Sel("blueComponent"),
	sharedWorkspace:      tcdarwin.Sel("sharedWorkspace"),
	reduceMotion:         tcdarwin.Sel("accessibilityDisplayShouldReduceMotion"),
	increaseContrast:     tcdarwin.Sel("accessibilityDisplayShouldIncreaseContrast"),
	screen:               tcdarwin.Sel("screen"),
	visibleFrame:         tcdarwin.Sel("visibleFrame"),
}

// telaCaptureSEL is a selector (the alias keeps the struct above short).
type telaCaptureSEL = tcdarwin.SEL

func init() {
	dir := tcos.Getenv(telaCaptureEnv)
	if dir == "" {
		return
	}
	go telaCaptureRun(dir, tctime.Now())
}

// telaCaptureRun waits for the window, follows it until it settles, writes
// the results and quits the app.
func telaCaptureRun(dir string, start tctime.Time) {
	rep := &telaCaptureReport{}
	fail := func(format string, args ...any) {
		rep.Error = tcfmt.Sprintf(format, args...)
		telaCaptureWriteReport(dir, rep)
		tcfmt.Fprintf(tcos.Stderr, "examplecapture: %s\n", rep.Error)
		tcos.Exit(3)
	}

	var (
		shown      tctime.Time // the first visible window
		prev       *tcimage.RGBA
		prevKind   string
		prevFull   bool
		lastChange []int32 // per pixel, the capture that last changed it
		lastIdx    = -1    // the last capture with a change
		base       = -1    // the first full read back (lastChange starts there)
		lastAt     tctime.Time
		final      telaCaptureShot
		n          int
	)
	for {
		timeout := telaCaptureUI
		if shown.IsZero() {
			timeout = telaCaptureFirst // the first call waits for Run to start
		}
		shot, ok := telaCaptureOnUI(timeout)
		if !ok {
			fail("the UI thread did not run a posted call within %v (window shown: %v)", timeout, !shown.IsZero())
		}
		if shot.panicked != "" {
			fail("reading the window back panicked: %s", shot.panicked)
		}
		now := tctime.Now()
		if !shot.ok {
			if now.Sub(start) > telaCaptureFirst {
				fail("no visible window %v after start (windows: %+v)", now.Sub(start).Round(tctime.Millisecond), shot.windows)
			}
			tctime.Sleep(telaCaptureEvery)
			continue
		}
		if shown.IsZero() {
			shown, lastAt = now, now
			rep.FirstVisibleMS = now.Sub(start).Milliseconds()
		}
		kind, pix := "window", shot.window
		if pix == nil {
			kind, pix = "layer", shot.layer
		}
		if pix == nil {
			if now.Sub(shown) > telaCaptureLimit {
				fail("no read back worked: capture: %s; layer: %s", shot.capNote, shot.layNote)
			}
			tctime.Sleep(telaCaptureEvery)
			continue
		}
		since := now.Sub(shown).Milliseconds()
		full := kind == "layer" || telaCaptureOpaque(pix)
		switch {
		case prev == nil || prev.Rect != pix.Rect || prevKind != kind || !full || !prevFull:
			lastChange = make([]int32, pix.Rect.Dx()*pix.Rect.Dy())
			for i := range lastChange {
				lastChange[i] = int32(n)
			}
			lastIdx, lastAt = n, now
			base, rep.BaseMS = n, since
			what := "first read back"
			switch {
			case !full:
				what = "not full yet (transparent outside the corners: fading in)"
			case prev != nil && prevFull:
				what = tcfmt.Sprintf("read back changed from %s %v to %s %v", prevKind, prev.Rect.Size(), kind, pix.Rect.Size())
			case prev != nil:
				what = "first full read back"
			}
			telaCaptureLog(rep, tcfmt.Sprintf("%dms #%d: %s (%s, %v)", since, n, what, kind, pix.Rect.Size()))
		default:
			changed, box := 0, tcimage.Rectangle{}
			w := pix.Rect.Dx()
			for y := range pix.Rect.Dy() {
				row, prow := pix.Pix[y*pix.Stride:][:w*4], prev.Pix[y*prev.Stride:][:w*4]
				for x := range w {
					if row[x*4] != prow[x*4] || row[x*4+1] != prow[x*4+1] || row[x*4+2] != prow[x*4+2] || row[x*4+3] != prow[x*4+3] {
						changed++
						lastChange[y*w+x] = int32(n)
						box = box.Union(tcimage.Rect(x, y, x+1, y+1))
					}
				}
			}
			if changed > 0 {
				lastIdx, lastAt = n, now
				telaCaptureLog(rep, tcfmt.Sprintf("%dms #%d: %d pixels changed in %v", since, n, changed, box))
			}
		}
		prev, prevKind, prevFull, final = pix, kind, full, shot
		quiet := n - lastIdx
		n++
		if full && quiet >= telaCaptureQuiet && now.Sub(shown) >= telaCaptureMinShown {
			rep.Settled = true
			break
		}
		if now.Sub(shown) >= telaCaptureLimit {
			break
		}
		tctime.Sleep(telaCaptureEvery)
	}

	end := tctime.Now()
	rep.Captures = n
	rep.QuietMS = end.Sub(lastAt).Milliseconds()
	rep.LastChangeMS = lastAt.Sub(shown).Milliseconds()
	rep.ShownMS = end.Sub(shown).Milliseconds()
	rep.Time = end.Format(tctime.RFC3339Nano)
	rep.Windows, rep.Window, rep.Primary = final.windows, final.win, prevKind
	s := final.settings
	rep.Appearance, rep.Dark, rep.ReduceMotion, rep.IncreaseContrast, rep.Accent, rep.Active =
		s.Appearance, s.Dark, s.ReduceMotion, s.IncreaseContrast, s.Accent, s.Active
	rep.Capture = telaCaptureRead{Path: "window.png", OK: final.window != nil, Note: final.capNote}
	rep.Layer = telaCaptureRead{Path: "layer.png", OK: final.layer != nil, Note: final.layNote}
	for _, f := range []struct {
		name string
		img  tcimage.Image
	}{{"window.png", final.window}, {"layer.png", final.layer}} {
		if img, ok := f.img.(*tcimage.RGBA); ok && img != nil {
			if err := telaCaptureWritePNG(tcfilepath.Join(dir, f.name), img); err != nil {
				fail("writing %s: %v", f.name, err)
			}
		}
	}
	if !rep.Settled && prev != nil {
		mask := tcimage.NewGray(prev.Rect)
		for i, c := range lastChange {
			if c > int32(base) {
				mask.Pix[i] = 0xff
				rep.Unstable++
			}
		}
		if err := telaCaptureWritePNG(tcfilepath.Join(dir, "unstable.png"), mask); err != nil {
			fail("writing unstable.png: %v", err)
		}
	}
	telaCaptureWriteReport(dir, rep)
	// main returns once the windows unmount; a quit that never completes
	// is reported by the exit status.
	tctime.AfterFunc(telaCaptureQuitWait, func() {
		tcfmt.Fprintf(tcos.Stderr, "examplecapture: main did not return within %v of tela.Quit\n", telaCaptureQuitWait)
		tcos.Exit(4)
	})
	tctela.Quit()
}

func telaCaptureLog(rep *telaCaptureReport, s string) {
	const keep = 60
	if len(rep.Timeline) == keep {
		rep.Timeline = append(rep.Timeline[:keep-10], append([]string{"..."}, rep.Timeline[keep-9:]...)...)
	}
	rep.Timeline = append(rep.Timeline, s)
}

func telaCaptureWriteReport(dir string, rep *telaCaptureReport) {
	b, err := tcjson.MarshalIndent(rep, "", "  ")
	if err == nil {
		err = tcos.WriteFile(tcfilepath.Join(dir, "report.json"), b, 0o644)
	}
	if err != nil {
		tcfmt.Fprintf(tcos.Stderr, "examplecapture: writing the report: %v\n", err)
	}
}

func telaCaptureWritePNG(path string, img tcimage.Image) error {
	f, err := tcos.Create(path)
	if err != nil {
		return err
	}
	if err := tcpng.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// telaCaptureOpaque reports whether a window server capture is opaque
// outside its corners (the 32-pixel squares the rounded corners cut): the
// window has faded in.
func telaCaptureOpaque(img *tcimage.RGBA) bool {
	const corner = 32
	w, h := img.Rect.Dx(), img.Rect.Dy()
	for y := range h {
		row := img.Pix[y*img.Stride:]
		for x := range w {
			if row[x*4+3] != 0xff && !((x < corner || x >= w-corner) && (y < corner || y >= h-corner)) {
				return false
			}
		}
	}
	return true
}

// telaCaptureOnUI reads the window back on the UI thread (tela.Post; a call
// posted before Run starts runs once the window is up) and waits for it at
// most timeout.
func telaCaptureOnUI(timeout tctime.Duration) (telaCaptureShot, bool) {
	ch := make(chan telaCaptureShot, 1)
	tctela.Post(func() {
		var s telaCaptureShot
		defer func() {
			if r := recover(); r != nil {
				s.panicked = tcfmt.Sprintf("%v\n%s", r, tcdebug.Stack())
			}
			ch <- s
		}()
		pool := tcdarwin.PoolPush()
		defer tcdarwin.PoolPop(pool)
		s = telaCaptureTake()
	})
	select {
	case s := <-ch:
		return s, true
	case <-tctime.After(timeout):
		return telaCaptureShot{}, false
	}
}

// telaCaptureTake lists the app's windows and reads the largest visible one
// back both ways. Main thread, inside a pool.
func telaCaptureTake() telaCaptureShot {
	var s telaCaptureShot
	sel := &telaCaptureSel
	app := tcdarwin.SendClass(tcdarwin.GetClass("NSApplication"), sel.sharedApplication)
	arr := tcdarwin.Send(app, sel.windows)
	var best tcdarwin.ID
	for i := range tcdarwin.SendUint(arr, sel.count) {
		ns := tcdarwin.Send(arr, sel.objectAtIndex, uint(i))
		w := telaCaptureWindow{
			Number:  tcdarwin.SendInt(ns, sel.windowNumber),
			Title:   tcdarwin.GoString(tcdarwin.Send(ns, sel.title)),
			Visible: tcdarwin.SendBool(ns, sel.isVisible),
			Scale:   tcdarwin.SendFloat(ns, sel.backingScaleFactor),
			Key:     tcdarwin.SendBool(ns, sel.isKeyWindow),
			Style:   tcdarwin.SendUint(ns, sel.styleMask),
			Class:   tcdarwin.GoString(tcdarwin.Send(ns, tcdarwin.Sel("className"))),
		}
		fr := tcdarwin.SendRect(ns, sel.frame)
		w.FrameW, w.FrameH = fr.Size.Width, fr.Size.Height
		if scr := tcdarwin.Send(ns, sel.screen); scr != 0 {
			vf := tcdarwin.SendRect(scr, sel.visibleFrame)
			w.ScreenW, w.ScreenH = vf.Size.Width, vf.Size.Height
		}
		if v := tcdarwin.Send(ns, sel.contentView); v != 0 {
			b := tcdarwin.SendRect(v, sel.bounds)
			w.W, w.H = b.Size.Width, b.Size.Height
			w.Controls = telaCaptureControls(ns, v, w.H, w.Style)
		}
		// platform.PhysicalSize: logical x scale, rounded half away from zero.
		w.PhysW, w.PhysH = int(tcmath.Round(w.W*w.Scale)), int(tcmath.Round(w.H*w.Scale))
		s.windows = append(s.windows, w)
		if !w.Visible || w.Number <= 0 {
			continue
		}
		if best == 0 || w.W*w.H > s.win.W*s.win.H {
			best, s.win = ns, w
		}
	}
	s.settings = telaCaptureSettings(app)
	if best == 0 || s.win.PhysW <= 0 || s.win.PhysH <= 0 {
		return s
	}
	s.ok = true
	space := tcdarwin.CGColorSpaceCreateWithName(tcdarwin.CGColorSpaceSRGB)
	if space == 0 {
		s.capNote, s.layNote = "no sRGB color space", "no sRGB color space"
		return s
	}
	defer tcdarwin.CGColorSpaceRelease(space)
	s.window, s.capNote = telaCaptureWindowServer(s.win, space)
	s.layer, s.layNote = telaCaptureLayer(best, s.win, space)
	return s
}

// telaCaptureSettings reads what the cocoa backend reads for
// platform.SystemSettings (settings_darwin.go), for the log and the
// reference's color scheme.
func telaCaptureSettings(app tcdarwin.ID) telaCaptureReport {
	sel := &telaCaptureSel
	var r telaCaptureReport
	if a := tcdarwin.Send(app, sel.effectiveAppearance); a != 0 {
		r.Appearance = tcdarwin.GoString(tcdarwin.Send(a, sel.name))
		r.Dark = tcstrings.Contains(r.Appearance, "Dark")
	}
	if c := tcdarwin.SendClass(tcdarwin.GetClass("NSColor"), sel.controlAccentColor); c != 0 {
		space := tcdarwin.SendClass(tcdarwin.GetClass("NSColorSpace"), sel.sRGBColorSpace)
		if rgb := tcdarwin.Send(c, sel.colorUsingColorSpace, space); rgb != 0 {
			ch := func(v float64) uint8 { return uint8(tcmath.Round(tcmath.Max(0, tcmath.Min(1, v)) * 255)) }
			r.Accent = tcfmt.Sprintf("#%02x%02x%02x", ch(tcdarwin.SendFloat(rgb, sel.redComponent)),
				ch(tcdarwin.SendFloat(rgb, sel.greenComponent)), ch(tcdarwin.SendFloat(rgb, sel.blueComponent)))
		}
	}
	if ws := tcdarwin.SendClass(tcdarwin.GetClass("NSWorkspace"), sel.sharedWorkspace); ws != 0 {
		r.ReduceMotion = tcdarwin.SendBool(ws, sel.reduceMotion)
		r.IncreaseContrast = tcdarwin.SendBool(ws, sel.increaseContrast)
	}
	r.Active = tcdarwin.SendBool(app, sel.isActive)
	return r
}

// telaCaptureWindowServer captures the window as the window server
// composited it and crops the content area (the capture holds the whole
// frame, title bar on top; a borderless window is its content), as
// cmd/tela's readWindow and the cocoa package's captureWindow do.
func telaCaptureWindowServer(w telaCaptureWindow, space uintptr) (*tcimage.RGBA, string) {
	img := tcdarwin.CGWindowListCreateImage(tcdarwin.CGRectNull, tcdarwin.CGWindowListOptionIncludingWindow, uint32(w.Number),
		tcdarwin.CGWindowImageBoundsIgnoreFraming|tcdarwin.CGWindowImageBestResolution)
	if img == 0 {
		return nil, "the capture returned nil (refused, or the function is gone)"
	}
	defer tcdarwin.CGImageRelease(img)
	iw, ih := tcdarwin.CGImageGetWidth(img), tcdarwin.CGImageGetHeight(img)
	full := telaCaptureDraw(img, iw, ih, space)
	if full == nil || iw < w.PhysW || ih < w.PhysH {
		return nil, tcfmt.Sprintf("capture %dx%d does not hold the %dx%d content area", iw, ih, w.PhysW, w.PhysH)
	}
	x0, y0 := (iw-w.PhysW)/2, ih-w.PhysH
	out := tcimage.NewRGBA(tcimage.Rect(0, 0, w.PhysW, w.PhysH))
	for y := range w.PhysH {
		copy(out.Pix[y*out.Stride:][:w.PhysW*4], full.Pix[full.PixOffset(x0, y0+y):])
	}
	// A refused capture of the app's own window comes back blank.
	first, blank := out.RGBAAt(0, 0), true
	for y := 0; y < w.PhysH && blank; y += 7 {
		for x := 0; x < w.PhysW; x += 7 {
			if out.RGBAAt(x, y) != first {
				blank = false
				break
			}
		}
	}
	note := tcfmt.Sprintf("window %d: capture %dx%d, content at (%d,%d)", w.Number, iw, ih, x0, y0)
	if blank {
		return nil, note + tcfmt.Sprintf(", blank (%v): treated as refused", first)
	}
	return out, note
}

// telaCaptureControls returns the union of the shown standard buttons of
// a window whose content view is its whole frame (the full-size content
// view style of a TitleBarOverlay window), in the content view's points
// with y down; zero otherwise (a titled window's buttons are above the
// content, a frameless window's are hidden).
func telaCaptureControls(ns, content tcdarwin.ID, h float64, style uint64) [4]float64 {
	const fullSizeContentView = 1 << 15
	if style&fullSizeContentView == 0 {
		return [4]float64{}
	}
	var x0, y0, x1, y1 float64
	n := 0
	for b := range 3 {
		btn := tcdarwin.Send(ns, tcdarwin.Sel("standardWindowButton:"), uintptr(b))
		if btn == 0 || tcdarwin.SendBool(btn, tcdarwin.Sel("isHidden")) {
			continue
		}
		r := tcdarwin.SendRect(content, tcdarwin.Sel("convertRect:fromView:"), tcdarwin.SendRect(btn, telaCaptureSel.bounds), btn)
		// The content view is not flipped: y up from its bottom.
		top := h - (r.Origin.Y + r.Size.Height)
		if n == 0 {
			x0, y0, x1, y1 = r.Origin.X, top, r.Origin.X+r.Size.Width, top+r.Size.Height
		} else {
			x0, y0 = tcmath.Min(x0, r.Origin.X), tcmath.Min(y0, top)
			x1, y1 = tcmath.Max(x1, r.Origin.X+r.Size.Width), tcmath.Max(y1, top+r.Size.Height)
		}
		n++
	}
	if n == 0 {
		return [4]float64{}
	}
	return [4]float64{x0, y0, x1 - x0, y1 - y0}
}

// telaCaptureLayer renders the content view's layer into a bitmap context.
func telaCaptureLayer(ns tcdarwin.ID, w telaCaptureWindow, space uintptr) (*tcimage.RGBA, string) {
	sel := &telaCaptureSel
	layer := tcdarwin.Send(tcdarwin.Send(ns, sel.contentView), sel.layer)
	if layer == 0 {
		return nil, "the content view has no layer"
	}
	ctx := tcdarwin.CGBitmapContextCreate(nil, w.PhysW, w.PhysH, 8, w.PhysW*4, space, tcdarwin.CGImageAlphaPremultipliedLast)
	if ctx == 0 {
		return nil, "CGBitmapContextCreate failed"
	}
	defer tcdarwin.CGContextRelease(ctx)
	// renderInContext: draws in the layer's coordinates and a bitmap
	// context has y up: a flipped layer goes through a flipped transform.
	flipped := tcdarwin.SendBool(layer, sel.isGeometryFlipped)
	if flipped {
		tcdarwin.CGContextTranslateCTM(ctx, 0, float64(w.PhysH))
		tcdarwin.CGContextScaleCTM(ctx, 1, -1)
	}
	tcdarwin.CGContextScaleCTM(ctx, w.Scale, w.Scale)
	tcdarwin.Send(layer, sel.renderInContext, ctx)
	pix := telaCaptureContext(ctx, w.PhysW, w.PhysH)
	if pix == nil {
		return nil, "the bitmap context has no data"
	}
	return pix, tcfmt.Sprintf("layer %dx%d at scale %g, geometryFlipped %v", w.PhysW, w.PhysH, w.Scale, flipped)
}

func telaCaptureDraw(img uintptr, w, h int, space uintptr) *tcimage.RGBA {
	ctx := tcdarwin.CGBitmapContextCreate(nil, w, h, 8, w*4, space, tcdarwin.CGImageAlphaPremultipliedLast)
	if ctx == 0 {
		return nil
	}
	defer tcdarwin.CGContextRelease(ctx)
	tcdarwin.CGContextDrawImage(ctx, tcdarwin.CGRect{Size: tcdarwin.CGSize{Width: float64(w), Height: float64(h)}}, img)
	return telaCaptureContext(ctx, w, h)
}

func telaCaptureContext(ctx uintptr, w, h int) *tcimage.RGBA {
	data := tcdarwin.CGBitmapContextGetData(ctx)
	if data == nil {
		return nil
	}
	out := tcimage.NewRGBA(tcimage.Rect(0, 0, w, h))
	copy(out.Pix, tcunsafe.Slice((*byte)(data), w*h*4))
	return out
}
