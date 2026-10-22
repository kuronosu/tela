package main

import (
	"fmt"
	"image"
	"regexp"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
)

// The picture of the error overlay (ADR-117): a Tela tree rendered
// headless, with the embedded fonts, onto a transparent image of the
// window's size, which the overlay view composites over the frame. The
// geometry (where the panel and its close button are, how many lines fit)
// is computed here in logical pixels, before the tree is built, so the
// input routing needs no layout pass and the tree has no elastic parts.

// Metrics of the panel in logical pixels.
const (
	ovLineH      = 18  // height of one line of text
	ovHeaderH    = 76  // title and summary
	ovFooterH    = 34  // key hints
	ovBodyPadY   = 8   // above and below the lines
	ovBorderTop  = 2   // the accent line
	ovMinDocked  = 220 // least height of the docked panel
	ovDockedPct  = 40  // docked height as a percentage of the window's
	ovMaxWidth   = 960 // widest full panel
	ovMargin     = 24  // around the full panel
	ovCloseSize  = 28  // the "×" button
	ovCloseInset = 12  // its distance to the panel's top and right edges
	ovShadowPad  = 48  // how far the docked panel's shadow reaches above it
)

// ovRect is a rectangle in logical pixels.
type ovRect struct{ x0, y0, x1, y1 float32 }

func (r ovRect) contains(p platform.Point) bool {
	return p.X >= r.x0 && p.X < r.x1 && p.Y >= r.y0 && p.Y < r.y1
}

func (r ovRect) w() float32 { return r.x1 - r.x0 }
func (r ovRect) h() float32 { return r.y1 - r.y0 }

// ovSeg is a run of text of one line, in the Tailwind classes that color it.
type ovSeg struct{ text, class string }

// ovLine is a line of the full text.
type ovLine struct{ segs []ovSeg }

// overlayModel is a problem ready to be drawn: its title and the lines of
// its text, parsed once.
type overlayModel struct {
	p      problem
	title  string
	sub    string
	accent string // border color class
	tone   string // title color class
	lines  []ovLine
	docked bool
}

// ovGeom is where the model's panel goes for a window size.
type ovGeom struct {
	docked bool
	panel  ovRect
	close  ovRect
	// visible is how many lines are drawn, and hidden how many are left
	// out ("… N more lines").
	visible, hidden int
	// paint is the area of the window the picture covers (the panel and
	// its shadow when docked, the whole window otherwise).
	paint ovRect
}

func newOverlayModel(p problem) *overlayModel {
	m := &overlayModel{p: p, docked: p.live}
	switch p.kind {
	case problemBuild:
		m.title, m.accent, m.tone = "Build failed", "border-red-500", "text-red-400"
		if p.live {
			m.sub = "The previous build keeps running above."
		} else {
			m.sub = "Nothing is running yet."
		}
		m.lines = compilerLines(p.text)
	case problemPanic:
		m.title, m.accent, m.tone = "The app panicked", "border-red-500", "text-red-400"
		m.sub = sentence(strings.TrimPrefix(p.summary, "the app panicked: "))
		m.lines = panicLines(p.text)
	case problemExit:
		m.title, m.accent, m.tone = "The app exited", "border-amber-500", "text-amber-400"
		m.sub = sentence(strings.TrimPrefix(p.summary, "the app "))
	default:
		m.title, m.accent, m.tone = "The app did not start", "border-amber-500", "text-amber-400"
		m.sub = sentence(p.summary)
		m.lines = compilerLines(p.text)
	}
	return m
}

// geometry places the panel in a window of the given logical size.
func (m *overlayModel) geometry(size platform.Size) ovGeom {
	W, H := size.W, size.H
	g := ovGeom{docked: m.docked}
	// chrome is what the panel needs besides the lines; with no lines there
	// is no body at all.
	chrome := float32(ovBorderTop + ovHeaderH + ovFooterH)
	if len(m.lines) > 0 {
		chrome += 2 * ovBodyPadY
	}
	want := chrome + float32(len(m.lines)*ovLineH)
	if g.docked {
		// Up to 40% of the window, less when the text is short.
		ph := min(max(H*ovDockedPct/100, ovMinDocked), H, want)
		g.panel = ovRect{0, H - ph, W, H}
		g.paint = ovRect{0, max(H-ph-ovShadowPad, 0), W, H}
	} else {
		mg := float32(ovMargin)
		if W < 480 {
			mg = ovMargin / 2
		}
		pw := min(W-2*mg, ovMaxWidth)
		maxH := H - 2*mg
		ph := min(maxH, want)
		x0, y0 := (W-pw)/2, (H-ph)/2
		g.panel = ovRect{x0, y0, x0 + pw, y0 + ph}
		g.paint = ovRect{0, 0, W, H}
	}
	g.close = ovRect{
		g.panel.x1 - ovCloseInset - ovCloseSize, g.panel.y0 + ovBorderTop + ovCloseInset - 4,
		g.panel.x1 - ovCloseInset, g.panel.y0 + ovBorderTop + ovCloseInset - 4 + ovCloseSize,
	}
	fit := int((g.panel.h() - chrome) / ovLineH)
	if fit < 0 {
		fit = 0
	}
	switch {
	case len(m.lines) <= fit:
		g.visible = len(m.lines)
	case fit == 0:
		g.visible, g.hidden = 0, len(m.lines)
	default:
		g.visible = fit - 1
		g.hidden = len(m.lines) - g.visible
	}
	return g
}

// --- the text ---

const (
	clsPlain  = "text-zinc-200"
	clsDim    = "text-zinc-500"
	clsFaint  = "text-zinc-600"
	clsLoc    = "text-sky-300 font-medium"
	clsStrong = "text-red-300 font-semibold"
)

var compilerLoc = regexp.MustCompile(`^(\s*)(\S+?\.(?:go|s|mod|sum):\d+(?::\d+)?)(:.*)?$`)

// compilerLines splits compiler output: "file:line:col" prefixes are
// emphasized and the "# package" headers dimmed.
func compilerLines(text string) []ovLine {
	var out []ovLine
	for _, l := range splitLines(text) {
		switch {
		case strings.HasPrefix(l, "# "):
			out = append(out, ovLine{[]ovSeg{{l, clsDim}}})
		case compilerLoc.MatchString(l):
			sm := compilerLoc.FindStringSubmatch(l)
			segs := []ovSeg{}
			if sm[1] != "" {
				segs = append(segs, ovSeg{sm[1], clsPlain})
			}
			segs = append(segs, ovSeg{sm[2], clsLoc})
			if sm[3] != "" {
				segs = append(segs, ovSeg{sm[3], clsPlain})
			}
			out = append(out, ovLine{segs})
		default:
			out = append(out, ovLine{[]ovSeg{{l, "text-zinc-300"}}})
		}
	}
	return out
}

var panicLoc = regexp.MustCompile(`^(\s*)(\S+?\.go:\d+)(.*)$`)

// panicLines splits a panic with its stack: the value in red, goroutine
// headers dimmed, and the frames of the runtime and of Tela itself dimmed
// so the app's own frames stand out.
func panicLines(text string) []ovLine {
	var out []ovLine
	frameworkFrame := false
	for i, l := range splitLines(text) {
		switch {
		case i == 0 && strings.HasPrefix(l, "panic"):
			out = append(out, ovLine{[]ovSeg{{l, clsStrong}}})
		case strings.HasPrefix(l, "goroutine ") && strings.HasSuffix(l, ":"):
			out = append(out, ovLine{[]ovSeg{{l, clsDim}}})
		case strings.HasPrefix(l, "\t") || strings.HasPrefix(l, "    "):
			// The file line of the frame above.
			if frameworkFrame {
				out = append(out, ovLine{[]ovSeg{{l, clsFaint}}})
				break
			}
			if sm := panicLoc.FindStringSubmatch(l); sm != nil {
				out = append(out, ovLine{[]ovSeg{{sm[1], clsPlain}, {sm[2], clsLoc}, {sm[3], clsFaint}}})
			} else {
				out = append(out, ovLine{[]ovSeg{{l, clsPlain}}})
			}
		case l == "":
			out = append(out, ovLine{})
		default:
			frameworkFrame = isFrameworkFrame(l)
			if frameworkFrame {
				out = append(out, ovLine{[]ovSeg{{l, clsFaint}}})
			} else {
				out = append(out, ovLine{[]ovSeg{{l, clsPlain}}})
			}
		}
	}
	return out
}

// isFrameworkFrame reports whether a frame's function line belongs to the
// runtime, the standard library plumbing or Tela itself (the examples and
// the app are the user's).
func isFrameworkFrame(fn string) bool {
	fn = strings.TrimPrefix(fn, "created by ")
	for _, p := range []string{"runtime.", "runtime/", "panic(", "reflect.", "sync.", "testing.", "github.com/kuronosu/tela/", "github.com/kuronosu/tela."} {
		if strings.HasPrefix(fn, p) {
			return !strings.HasPrefix(fn, "github.com/kuronosu/tela/examples/")
		}
	}
	return false
}

// splitLines splits text into lines, dropping the trailing newline and the
// carriage returns, and turning tabs into four spaces.
func splitLines(text string) []string {
	text = strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if text == "" {
		return nil
	}
	text = strings.ReplaceAll(text, "\t", "    ")
	return strings.Split(text, "\n")
}

// --- the tree ---

// view builds the tree of the picture for a window of the given size.
func (m *overlayModel) view(size platform.Size, g ovGeom) tela.Node {
	px := func(v float32) string { return fmt.Sprintf("[%gpx]", v) }
	panel := []string{
		"absolute flex flex-col overflow-hidden bg-zinc-950 border-t-2", m.accent,
		"left-" + px(g.panel.x0), "top-" + px(g.panel.y0), "w-" + px(g.panel.w()), "h-" + px(g.panel.h()),
	}
	if g.docked {
		panel = append(panel, "rounded-t-xl shadow-[0_-10px_30px_rgba(0,0,0,0.45)]")
	} else {
		panel = append(panel, "rounded-xl border-x border-b border-zinc-800 shadow-2xl")
	}

	body := make([]tela.Child, 0, g.visible+1)
	for _, l := range m.lines[:g.visible] {
		body = append(body, lineNode(l))
	}
	if g.hidden > 0 {
		noun := "lines"
		if g.hidden == 1 {
			noun = "line"
		}
		body = append(body, lineNode(ovLine{[]ovSeg{{fmt.Sprintf("… %d more %s", g.hidden, noun), clsDim + " italic"}}}))
	}

	panelKids := []tela.Child{h.Class(panel...)}
	panelKids = append(panelKids,
		h.Div(h.Class("shrink-0 flex flex-col gap-1 px-5 pt-4 pr-14 h-"+px(ovHeaderH)),
			h.Div(h.Class("text-base leading-6 font-semibold truncate", m.tone), h.Text(m.title)),
			h.Div(h.Class("text-[13px] leading-5 text-zinc-300 truncate"), h.Text(oneLine(m.sub))),
		))
	if len(m.lines) > 0 {
		panelKids = append(panelKids,
			h.Div(append([]tela.Child{h.Class("flex-1 min-h-0 overflow-hidden px-5 py-2 font-mono text-xs bg-zinc-900")}, body...)...))
	} else {
		panelKids = append(panelKids, h.Div(h.Class("flex-1")))
	}
	panelKids = append(panelKids,
		h.Div(h.Class("shrink-0 flex items-center px-5 text-xs text-zinc-500 border-t border-zinc-800 h-"+px(ovFooterH)),
			h.Text("Save a change to rebuild  \u00b7  r reload (in the terminal)  \u00b7  Esc dismiss")),
		h.Div(h.Class("absolute flex items-center justify-center rounded-lg bg-white/10 text-zinc-200 text-lg leading-none",
			"left-"+px(g.close.x0-g.panel.x0), "top-"+px(g.close.y0-g.panel.y0), "w-"+px(ovCloseSize), "h-"+px(ovCloseSize)),
			h.Text("\u00d7")),
	)
	root := []tela.Child{h.Class("relative w-full h-full overflow-hidden")}
	if !g.docked {
		root = append(root, h.Div(h.Class("absolute inset-0 bg-black/60")))
	}
	root = append(root, h.Div(panelKids...))
	return h.Div(root...)
}

// lineNode is one line of the body: fixed height, never wrapped, cut with
// an ellipsis at the panel's edge.
func lineNode(l ovLine) tela.Node {
	kids := []tela.Child{h.Class("whitespace-pre truncate text-zinc-200 h-[18px]")}
	if len(l.segs) == 0 {
		kids = append(kids, h.Text(" "))
	}
	for _, s := range l.segs {
		kids = append(kids, h.Span(h.Class(s.class), h.Text(s.text)))
	}
	return h.Div(kids...)
}

// oneLine keeps the first line of s.
func oneLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// render draws the model for a window of the given logical size at scale:
// a premultiplied image of the window's physical size, transparent where
// nothing is drawn.
//
// A Tela tree always paints its canvas first, opaque, so the picture is
// rendered twice, over a black canvas and over a white one, and the alpha
// of each pixel recovered from the difference: over black a pixel is its
// premultiplied color c, over white it is c + (255 - a).
func (m *overlayModel) render(size platform.Size, scale float64) (img *image.RGBA, g ovGeom, err error) {
	g = m.geometry(size)
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("the error overlay failed to render: %v", r)
		}
	}()
	onBlack := m.renderOn("bg-black", size, scale, g)
	onWhite := m.renderOn("bg-white", size, scale, g)
	for i := 0; i+3 < len(onBlack.Pix); i += 4 {
		// The three channels agree up to rounding; the largest difference
		// is the most precise.
		d := 0
		for c := 0; c < 3; c++ {
			d = max(d, int(onWhite.Pix[i+c])-int(onBlack.Pix[i+c]))
		}
		a := uint8(255 - min(d, 255))
		onBlack.Pix[i+3] = a
		// Premultiplied colors cannot exceed the alpha.
		for c := 0; c < 3; c++ {
			onBlack.Pix[i+c] = min(onBlack.Pix[i+c], a)
		}
	}
	return onBlack, g, nil
}

// renderOn renders the tree over an opaque canvas of the given class.
func (m *overlayModel) renderOn(canvas string, size platform.Size, scale float64, g ovGeom) *image.RGBA {
	t := core.NewTree(core.Config{
		Debug:     core.DebugOff,
		IDPrefix:  "ov-",
		Dark:      true,
		RootClass: canvas,
	})
	t.SetViewport(gfx.Size{W: size.W, H: size.H})
	t.SetScale(float32(scale))
	t.Mount(core.NodeOf(m.view(size, g)))
	dl, _ := t.Frame()
	for i := 1; i < core.MaxRenderFrames && t.Dirty(); i++ {
		dl, _ = t.Frame()
	}
	img := t.Rasterize(dl, float32(scale))
	t.FlushEffects()
	t.Unmount()
	return img
}

// sentence capitalizes the first letter of s.
func sentence(s string) string {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}
