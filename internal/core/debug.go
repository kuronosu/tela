package core

import (
	"fmt"
	"image"
	"os"
	"strings"
	"time"

	"github.com/kuronosu/tela/internal/editdist"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/text"
)

// Debug overlays (ARCHITECTURE section 17, "Debug overlays"; from Chrome
// DevTools and Flutter DevTools): layout boxes, paint flashing, layer
// borders, a frame meter and semantics bounds, drawn by Tela's own
// renderer on top of the application.
//
// # Composition and damage
//
// The overlays are a display list of their own (debug_draw.go), built
// after the application's frame and rasterized over it. They never enter
// the layers, the frames or the damage of the application (layer.go):
//
//   - RasterizeFrame brings a private copy of the framebuffer (the clean
//     one) to the application's frame exactly as it does without
//     overlays, so LastDamage keeps reporting what the application
//     repainted, which paint-flash shows;
//   - the framebuffer the window presents is that copy plus the overlay
//     list: only the regions that changed are copied again and
//     rasterized over (the application's damage, and the overlay's own,
//     found by matching its commands in order like the damage of
//     frames), so the pixels equal a full rasterization of the
//     application followed by the overlays;
//   - turning the overlays off makes the next frame rasterize the
//     application fully into the window's framebuffer, so its pixels are
//     exactly the ones it would have had without them.
//
// The overlays do not take part in layout, hit-testing, focus or
// semantics: nothing in the tree knows them, and they never receive
// input. They read the tree: the box geometry of the last paint, the
// layers (Tree.Layers), the damage (Tree.LastDamage) and the semantics
// tree.
//
// # Cost
//
// With no flag set (the default) the only added work is one comparison of
// the flags in RasterizeFrame, the Host's frame and the hover update.
// Overlays that change over time ask for frames only while they need them
// (DebugWantsFrame): paint-flash while a flash fades, fps always (a
// window that is not visible receives no frame callbacks, so it costs
// nothing hidden); layout, layers and semantics never do (a hover change
// asks for one frame).

// EnvDebug is the environment variable that selects the debug overlays: a
// comma-separated list of layout, paint-flash, layers, fps and semantics,
// or "all".
const EnvDebug = "TELA_DEBUG"

// DebugFlags is a set of debug overlays.
type DebugFlags uint8

const (
	// DebugLayout draws the box model of every box (margin, border,
	// padding and content in Chrome DevTools' colors), with the hovered
	// element's box model and a label emphasized.
	DebugLayout DebugFlags = 1 << iota
	// DebugPaintFlash flashes the regions each frame repainted.
	DebugPaintFlash
	// DebugLayers draws the borders of the layers with a label each.
	DebugLayers
	// DebugFPS draws a frame meter in a corner.
	DebugFPS
	// DebugSemantics draws the bounds and role "name" of the semantics
	// nodes.
	DebugSemantics

	// DebugAll is every overlay.
	DebugAll = DebugLayout | DebugPaintFlash | DebugLayers | DebugFPS | DebugSemantics

	// DebugOff in Config.Debug means no overlays and also that NewHost
	// must not read TELA_DEBUG (the zero value means "unset: read it").
	DebugOff DebugFlags = 1 << 7
)

// debugNames lists the flags in the order of the documentation.
var debugNames = [...]struct {
	name string
	flag DebugFlags
}{
	{"layout", DebugLayout},
	{"paint-flash", DebugPaintFlash},
	{"layers", DebugLayers},
	{"fps", DebugFPS},
	{"semantics", DebugSemantics},
}

// String returns the flags as ParseDebug reads them: "layout,fps", "all",
// or "" for none.
func (f DebugFlags) String() string {
	f &= DebugAll
	if f == DebugAll {
		return "all"
	}
	var parts []string
	for _, n := range debugNames {
		if f&n.flag != 0 {
			parts = append(parts, n.name)
		}
	}
	return strings.Join(parts, ",")
}

// ParseDebug reads a list of overlays such as "layout,paint-flash" (white
// space around names is ignored, and so is case; "all" selects every
// overlay, "none" and "" none). It returns the flags of the valid names
// even when it also returns an error, which lists the unknown names and the
// valid ones (with a suggestion for a likely typo): the caller reports it
// once and goes on.
func ParseDebug(s string) (DebugFlags, error) {
	var f DebugFlags
	var bad []string
	var hints []string
	for _, name := range strings.Split(s, ",") {
		name = strings.ToLower(strings.TrimSpace(name))
		switch name {
		case "", "none", "off":
			continue
		case "all":
			f |= DebugAll
			continue
		}
		ok := false
		for _, n := range debugNames {
			if name == n.name {
				f |= n.flag
				ok = true
				break
			}
		}
		if ok {
			continue
		}
		bad = append(bad, fmt.Sprintf("%q", name))
		if sug := suggestDebug(name); sug != "" {
			hints = append(hints, fmt.Sprintf("%q is probably %q", name, sug))
		}
	}
	if len(bad) == 0 {
		return f, nil
	}
	noun := "flag"
	if len(bad) > 1 {
		noun = "flags"
	}
	msg := fmt.Sprintf("%s: unknown debug %s %s", EnvDebug, noun, strings.Join(bad, ", "))
	if len(hints) > 0 {
		msg += " (" + strings.Join(hints, "; ") + ")"
	}
	msg += "; valid flags: layout, paint-flash, layers, fps, semantics, or all (comma-separated, for example " + EnvDebug + "=layout,fps)"
	return f, fmt.Errorf("%s", msg)
}

// suggestDebug returns the valid flag closest to name, or "" when none is
// close enough to be a typo.
func suggestDebug(name string) string {
	best, bestD := "", 1<<30
	for _, n := range debugNames {
		d := editdist.Distance(name, n.name)
		if strings.Contains(n.name, name) || strings.HasPrefix(n.name, name) {
			d = min(d, 1)
		}
		if d < bestD {
			best, bestD = n.name, d
		}
	}
	if bestD <= max(2, len(best)/3) {
		return best
	}
	return ""
}

// DebugFromEnv reads TELA_DEBUG. A malformed value gives the flags that
// are valid and an error to report.
func DebugFromEnv() (DebugFlags, error) { return ParseDebug(os.Getenv(EnvDebug)) }

// DebugFrameKind says how a frame was rendered, for the fps meter.
type DebugFrameKind uint8

const (
	// DebugFrameIdle: nothing of the application changed (the frame
	// only redrew overlays).
	DebugFrameIdle DebugFrameKind = iota
	// DebugFrameIncremental: only the damage was rasterized.
	DebugFrameIncremental
	// DebugFrameFull: the whole window was rasterized.
	DebugFrameFull
)

// flashFade is how long a paint flash takes to fade out.
const flashFade = 300 * time.Millisecond

// maxFlashes bounds the flashes kept (the oldest are dropped).
const maxFlashes = 128

// fpsHistory is the number of frames the meter remembers.
const fpsHistory = 120

// flash is a repainted region (logical pixels) and when it happened.
type flash struct {
	r  gfx.Rect
	at time.Duration
}

// fpsSample is one frame of the meter.
type fpsSample struct {
	at, work time.Duration
	kind     DebugFrameKind
}

// debugState is the overlay state of a Tree (UI thread).
type debugState struct {
	flags DebugFlags
	clock func() time.Duration

	// clean holds the application's pixels, and prev is the overlay list
	// drawn over them in the window's framebuffer (nil: none yet).
	clean     *image.RGBA
	prev      *gfx.DisplayList
	prevScale float32

	// pending: something an overlay shows changed since the last frame
	// (the hovered element).
	pending bool

	flashes []flash
	frames  []fpsSample // ring, oldest first once full
	next    int

	// Drawing state reused between frames.
	b      *gfx.Builder
	labels map[labelKey]*text.Layout
	scr    overlayScratch
}

// debugEpoch is the zero of the default overlay work clock.
var debugEpoch = time.Now()

func defaultDebugClock() time.Duration { return time.Since(debugEpoch) }

// Debug returns the overlays that are on.
func (t *Tree) Debug() DebugFlags { return t.dbg.flags }

// SetDebug turns overlays on or off. The next frame draws them (or, when
// none is left, rasterizes the application alone again).
func (t *Tree) SetDebug(f DebugFlags) {
	f &= DebugAll
	d := &t.dbg
	if f == d.flags {
		return
	}
	d.flags = f
	d.clean, d.prev, d.pending = nil, nil, false
	d.flashes = d.flashes[:0]
	// The window's framebuffer no longer holds the application's frame
	// alone: rasterize it again from scratch.
	t.lay.ras = nil
	t.requestRepaint()
}

// DebugNow reads the clock the frame meter times frames with (a
// monotonic clock unless Config.DebugClock replaced it).
func (t *Tree) DebugNow() time.Duration {
	if t.dbg.clock != nil {
		return t.dbg.clock()
	}
	return defaultDebugClock()
}

// DebugRecordFrame adds a frame to the meter: at is the frame clock when
// it ran, work the time its pipeline and rasterization took, kind how it
// was rendered. The Host calls it after every frame while the fps overlay
// is on.
func (t *Tree) DebugRecordFrame(at, work time.Duration, kind DebugFrameKind) {
	d := &t.dbg
	if d.flags&DebugFPS == 0 {
		return
	}
	s := fpsSample{at, work, kind}
	if len(d.frames) < fpsHistory {
		d.frames = append(d.frames, s)
		return
	}
	d.frames[d.next] = s
	d.next = (d.next + 1) % fpsHistory
}

// DebugPending reports that a frame must render although the application
// has nothing new: overlays that change over time, or a hover that moved.
func (t *Tree) DebugPending() bool {
	d := &t.dbg
	return d.flags != 0 && (d.pending || d.flags&DebugFPS != 0 || len(d.flashes) > 0)
}

// DebugWantsFrame reports that the overlays need another frame even if
// nothing else changes: a flash is fading, or the fps meter is on. A
// layout, layers or semantics overlay never does.
func (t *Tree) DebugWantsFrame() bool {
	d := &t.dbg
	return d.flags != 0 && (d.flags&DebugFPS != 0 || len(d.flashes) > 0)
}

// debugTouch records that an overlay input changed (the hovered element)
// and asks for a frame.
func (t *Tree) debugTouch() {
	t.dbg.pending = true
	if t.inFrame || t.asked || t.onDirty == nil {
		return
	}
	t.asked = true
	t.onDirty()
}

// RasterizeFrame brings fb to the last frame of Frame, at the tree's
// scale: fb holds the frame this method rasterized last (its pixels
// outside the damage are kept), unless full is set, the scale or the size
// of fb changed, or nothing was rasterized yet, in which case the frame is
// rasterized fully. The result is bit-identical to a full rasterization of
// the frame (Rasterize), with the debug overlays drawn over it when they
// are on (debug.go). It returns what it did to fb, which is not what
// LastDamage reports when overlays are on: that is what the application
// repainted.
func (t *Tree) RasterizeFrame(fb *image.RGBA, r *raster.Renderer, full bool) Damage {
	if t.dbg.flags == 0 {
		return t.rasterizeApp(fb, r, full)
	}
	return t.rasterizeDebug(fb, r, full)
}

// rasterizeDebug is RasterizeFrame with overlays on.
func (t *Tree) rasterizeDebug(fb *image.RGBA, r *raster.Renderer, full bool) Damage {
	d := &t.dbg
	if t.lay.cur == nil {
		return Damage{}
	}
	if d.clean == nil || d.clean.Rect != fb.Rect {
		d.clean = image.NewRGBA(fb.Rect)
		d.prev = nil
		full = true
	}
	app := t.rasterizeApp(d.clean, r, full)
	scale := t.lay.cur.scale
	if d.flags&DebugPaintFlash != 0 {
		t.addFlashes(app)
	}
	ol := t.buildOverlay(fb.Rect)
	d.pending = false

	var out Damage
	if app.Full || d.prev == nil || d.prevScale != scale {
		copy(fb.Pix, d.clean.Pix)
		r.Render(fb, ol, raster.Options{Scale: scale})
		out.Full = true
	} else {
		rects := make([]image.Rectangle, 0, len(app.Shifts)+len(app.Rects)+4)
		for _, s := range app.Shifts {
			rects = append(rects, s.Rect)
		}
		rects = append(rects, app.Rects...)
		rects = append(rects, overlayDamage(d.prev, ol, scale, fb.Rect)...)
		out.Rects = mergeOverlayRects(rects)
		for _, rc := range out.Rects {
			copyRect(fb, d.clean, rc)
			r.Render(fb, ol, raster.Options{Scale: scale, Region: rc})
		}
	}
	d.prev, d.prevScale = ol, scale
	return out
}

// copyRect copies the pixels of rc from src to dst, which share bounds.
func copyRect(dst, src *image.RGBA, rc image.Rectangle) {
	rc = rc.Intersect(dst.Rect)
	if rc.Empty() {
		return
	}
	n := 4 * rc.Dx()
	for y := rc.Min.Y; y < rc.Max.Y; y++ {
		o := dst.PixOffset(rc.Min.X, y)
		copy(dst.Pix[o:o+n], src.Pix[o:o+n])
	}
}

// maxOverlayRects bounds the rectangles of an overlay frame: beyond it,
// they are merged into one.
const maxOverlayRects = 24

// mergeOverlayRects drops empty rectangles and, when there are many,
// replaces them with their bounding box.
func mergeOverlayRects(rects []image.Rectangle) []image.Rectangle {
	out := rects[:0]
	for _, r := range rects {
		if !r.Empty() {
			out = append(out, r)
		}
	}
	if len(out) <= maxOverlayRects {
		return out
	}
	u := out[0]
	for _, r := range out[1:] {
		u = u.Union(r)
	}
	return []image.Rectangle{u}
}

// overlayDamage returns the device rectangles where two overlay lists
// differ: the lists are matched by their common prefix and suffix, and the
// commands in between are damage (overlay lists hold draw commands only).
func overlayDamage(a, b *gfx.DisplayList, scale float32, bounds image.Rectangle) []image.Rectangle {
	na, nb := a.Len(), b.Len()
	pre := 0
	for pre < na && pre < nb && gfx.Equal(a.At(pre), b.At(pre)) {
		pre++
	}
	suf := 0
	for suf < na-pre && suf < nb-pre && gfx.Equal(a.At(na-1-suf), b.At(nb-1-suf)) {
		suf++
	}
	var out []image.Rectangle
	add := func(l *gfx.DisplayList, from, to int) {
		for i := from; i < to; i++ {
			out = append(out, deviceRect(l.BoundsAt(i), scale, bounds))
		}
	}
	add(a, pre, na-suf)
	add(b, pre, nb-suf)
	return out
}

// deviceRect maps a logical rectangle to device pixels, grown by one
// pixel for antialiasing and clipped to bounds.
func deviceRect(r gfx.Rect, scale float32, bounds image.Rectangle) image.Rectangle {
	if r.Empty() {
		return image.Rectangle{}
	}
	s := gfx.Rect{
		X0: float32(r.X0 * scale), Y0: float32(r.Y0 * scale),
		X1: float32(r.X1 * scale), Y1: float32(r.Y1 * scale),
	}
	return s.RoundOut().Inset(-1).Intersect(bounds)
}

// addFlashes records the regions the last application frame repainted.
func (t *Tree) addFlashes(app Damage) {
	d := &t.dbg
	now := t.scr.now
	if app.Full {
		d.flashes = append(d.flashes, flash{gfx.Rect{X1: t.viewport.W, Y1: t.viewport.H}, now})
	} else {
		sc := float32(t.lay.cur.scale)
		for _, r := range app.Rects {
			f := gfx.RectFromImage(r)
			d.flashes = append(d.flashes, flash{gfx.Rect{X0: f.X0 / sc, Y0: f.Y0 / sc, X1: f.X1 / sc, Y1: f.Y1 / sc}, now})
		}
	}
	if n := len(d.flashes) - maxFlashes; n > 0 {
		d.flashes = append(d.flashes[:0], d.flashes[n:]...)
	}
}
