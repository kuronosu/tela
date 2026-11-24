package core

import (
	"fmt"
	"image"
	"path/filepath"
	"runtime"
	"slices"
	"sync/atomic"
	"time"

	"github.com/kuronosu/tela/internal/canvas"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/svg"
	"github.com/kuronosu/tela/internal/text"
)

// Canvases (ADR-319): an element whose content a function of the
// application draws. The drawing API is package canvas; this file decides
// when a canvas draws.
//
// A primitive of style.KindCanvas (h.Canvas) is a replaced element, as a
// Texture: it has no natural size but a canvas's 300x150, and its content is
// clipped to its content box. Its draw function runs in the paint phase
// (layer.go: paintPhase), once the boxes are placed and before anything is
// recorded, and records a display list in the canvas's own coordinates,
// which the box's painting appends (canvasBox.paint). A canvas is a group
// (record.go), so a new list records that group and nothing else.
//
// # When it draws
//
// A canvas is looked at in a paint (canvasBox.look) when it is in the queue
// (canvasTree.q), and it is put there by what can change what it draws:
//
//   - a new node, so a new function: its mount and every build of the
//     component that made the node (updateCanvas);
//   - a Value that the function read the last time it ran (valueChanged). Its
//     reads are tracked like those of a paint-phase deferred prop (value.go),
//     by an observer of their own;
//   - a place of its box (painter.place): a new size, scale, style, theme or
//     scheme marks the box. It draws if what it can see of them changed
//     (canvasKey, sameText), so a hover that changes its background does not
//     draw it;
//   - other fonts for the same styles: the desktop's changed (reshapeText);
//   - a TextColorFn that changes the color it inherits (stampText);
//   - a request for a frame (Canvas.RequestFrame) that the frame serves.
//
// What it draws is compared with what it drew before, command by command. An
// equal list is dropped: nothing is stamped, recorded or damaged.
//
// A canvas that no list paints does not draw until one does: it stays stale
// (and parked: what changes meanwhile asks for no frame). It is not displayed
// or not visible, its box is empty, or it is outside the interest rect of its
// scroll space. Most of those draw when they are placed again; a list that
// paints a stale canvas (culled until now) has it drawn right after the
// recording, and its group recorded again, in the same paint (drawLate).
//
// Nothing runs the function while a list is being recorded, so a panic in it
// leaves the layers whole: the canvas stays stale, with its old list and
// subscriptions, and the next paint records everything (needFull).
//
// # Frames
//
// A draw that calls RequestFrame puts its canvas in canvasTree.wants. After
// a frame, the canvases there that can be seen (textureBoxVisible, the rule
// of the textures) in a window that gets frames are on, and the tree asks
// for a frame for them, with nothing marked: Frame alone finds no work. The
// start of the window's next frame (Tree.Animate) makes the ones that are on
// due, which is work. The others wait: nothing polls, since what can put a
// canvas on screen is a frame or a window shown again (syncOn).
//
// # The clock
//
// Canvas.Time is the sum of the Deltas, and Delta the frame time since the
// previous draw while the canvas is running: that draw called RequestFrame,
// and since then the canvas was on at the end of every frame. So neither a
// rest nor a wait is a jump: the first draw after one has Delta 0. A gap of
// more than canvasGap between two draws of a running canvas is a wait of the
// window that the platform could not tell (a minimized window under a
// compositor that only stops its frame callbacks, a machine that slept).
//
// # What is linked
//
// The rest of core reaches canvases through interfaces, whose
// implementations only a node with CanvasDraw makes: a program without
// canvases links none of this file but them, and nothing of package canvas
// (examples/hello's size budget, ADR-290).

// canvasGap is the longest time between two draws that counts as time that
// the canvas ran.
const canvasGap = time.Second

// canvasDrawer is the draw function of a node: a canvasDraw.
type canvasDrawer interface {
	// check panics for a primitive of kind that cannot draw.
	check(kind style.Kind, d *desc)
	// mount returns the canvas of box b.
	mount(b *Box) canvasItem
}

// canvasItem is the canvas of a box (boxMedia.cnv): a *canvasBox.
type canvasItem interface {
	// update gives it the function of a new node, and drop ends it: its box
	// unmounts.
	update(d canvasDrawer)
	drop()
	// queue makes the next paint (the one in progress, before it draws) look
	// at it.
	queue()
	// valueChanged marks it after a change of a Value that its last draw
	// read, and reports whether that needs a frame.
	valueChanged() bool
	// fontsChanged marks it after a change of the fonts that its text
	// resolves to.
	fontsChanged()
	// paint paints what it drew into b, in its content box c; ref is
	// painter.ref.
	paint(b *gfx.Builder, c gfx.RRect, ref bool)
}

// canvasSet is the canvases of a Tree (Tree.cnv, nil for none): a
// *canvasTree. (Nothing here takes a *painter, which a call through an
// interface would move to the heap, once per frame.)
type canvasSet interface {
	// draw draws the queued canvases, before the recording, and drawLate
	// those that it found not to have drawn, after it.
	draw()
	drawLate()
	// tick starts a frame of the window, afterFrame ends a Frame, and syncOn
	// looks at which canvases the window shows.
	tick()
	afterFrame()
	syncOn()
	// running counts the canvases on screen that asked for a frame.
	running() int
	// queued returns the length of the queue, and unqueue cuts it to n
	// (checkPlace).
	queued() int
	unqueue(n int)
	// spare reports whether it keeps scratch, which trim releases.
	spare() bool
	trim()
}

// canvasDraw is the draw function of a Canvas.
type canvasDraw func(*canvas.Canvas)

// CanvasDraw sets the draw function of a style.KindCanvas primitive.
func CanvasDraw(draw func(*canvas.Canvas)) Prop {
	if draw == nil {
		panic("tela: h.Canvas(nil): pass the function that draws, such as Canvas(func(c *tela.Canvas) { c.FillRect(0, 0, c.Width, c.Height) })")
	}
	return Prop{name: "CanvasDraw", apply: func(d *desc) { d.mediaProps().draw = canvasDraw(draw) }}
}

func (canvasDraw) check(kind style.Kind, d *desc) {
	if kind != style.KindCanvas {
		panic(fmt.Sprintf("core: CanvasDraw on <%v>: use Canvas", kind))
	}
	for _, c := range d.children {
		if c.d != nil {
			panic("tela: h.Canvas takes no children: what it shows is what its function draws. Place an overlay beside it: Div(Class(\"relative\"), Canvas(draw, Class(\"w-full\")), Div(Class(\"absolute inset-0\"), ...))")
		}
	}
}

func (canvasDraw) mount(b *Box) canvasItem {
	t := b.tree
	if t.cnv == nil {
		t.cnv = &canvasTree{t: t}
	}
	return &canvasBox{box: b, set: t.cnv.(*canvasTree)}
}

// updateCanvas gives the box of a Canvas element the function of its node:
// it mounts, or its component built a new one.
func (t *Tree) updateCanvas(el *Element) {
	b, d := el.box, el.node.d.media.draw
	if b.med == nil {
		b.med = &boxMedia{cnv: d.mount(b)}
	}
	b.med.cnv.update(d)
	t.requestRepaint()
}

// canvasBox is the state of a Canvas box. UI thread.
type canvasBox struct {
	box  *Box
	set  *canvasTree
	draw canvasDraw
	// key is what the last draw saw of the element, obs the Values it read;
	// what it recorded is boxMedia.drawing.
	key canvasKey
	obs *observer
	// stale: something it draws with changed, besides key. parked: it did
	// not draw in the last paint that looked at it, as nothing paints it, so
	// nothing needs a frame for it. inQueue: it is in canvasTree.q.
	stale, parked, inQueue bool
	// drawn: it drew once; frame is Tree.frameStart then.
	drawn bool
	frame uint64
	// wants: its last draw asked for a frame; listed: it is in
	// canvasTree.wants; visible: it can be seen, as of the last frame, and
	// on: in a window that gets frames; due: the frame in progress serves
	// the request.
	wants, listed, visible, on, due bool
	// running: its clock runs since its last draw, at frame time last; time
	// is the clock.
	running    bool
	time, last time.Duration
	// fill and stroke are key's colors as a canvas takes them.
	fill, stroke canvas.Style
	// text is the style of its text at the last draw (nothing that only
	// paints), in direction dir, and font what a canvas sees of it.
	text text.TextStyle
	dir  text.Direction
	font canvas.CanvasFont
	// uses holds the images of the last draw (assets.go, "Releasing
	// images"), using those of the one that runs.
	uses, using []*imageAsset
}

// canvasKey is what a draw function sees of its element.
type canvasKey struct {
	w, h         int // device pixels
	scale        float32
	fill, stroke gfx.Color
	lineWidth    float32
	theme        *style.Theme
	dark         bool
}

// canvasTree is the state of a Tree's canvases (UI thread).
type canvasTree struct {
	t *Tree
	// q are the canvases the next paint looks at, late those that the paint
	// in progress recorded before they drew.
	q, late []*canvasBox
	// wants are the canvases that asked for a frame, and frames the frame
	// count at the last look at which are on screen.
	wants  []*canvasBox
	frames int
	// more: a draw function left work for another frame (run).
	more bool
	// fonts counts the changes of the fonts that text resolves to.
	fonts uint64
	// rec records the draws, made at the first one.
	rec *canvas.Recorder
}

func (c *canvasBox) update(d canvasDrawer) {
	c.draw = d.(canvasDraw)
	c.stale = true
	c.queue()
}

func (c *canvasBox) drop() {
	dropObserver(c.obs)
	c.box.med.drawing = nil
	c.draw, c.wants, c.on, c.due = nil, false, false, false
	c.uses = nil
}

func (c *canvasBox) queue() {
	if !c.inQueue {
		c.inQueue = true
		c.set.q = append(c.set.q, c)
	}
}

func (c *canvasBox) valueChanged() bool {
	if c.stale && c.parked {
		return false // it draws when something paints it
	}
	c.stale = true
	c.queue()
	c.set.t.requestRepaint()
	return true
}

func (c *canvasBox) fontsChanged() {
	c.set.fonts++
	c.valueChanged()
}

func (s *canvasTree) queued() int { return len(s.q) }

func (s *canvasTree) unqueue(n int) {
	for _, c := range s.q[n:] {
		c.inQueue = false
	}
	s.q = s.q[:n]
}

func (s *canvasTree) spare() bool { return s.rec != nil }

func (s *canvasTree) trim() { s.rec = nil }

// ---------------------------------------------------------------------
// Drawing.

// draw looks at the queued canvases. What their functions queue is for the
// next paint.
func (s *canvasTree) draw() {
	end := len(s.q)
	if end == 0 {
		return
	}
	p := painter{t: s.t, scale: float64(s.t.scale)}
	n := 0
	defer func() {
		if n < end {
			// A function panicked: its canvas and those after it wait for
			// the next paint, which records what this one leaves unrecorded.
			s.q[n].inQueue = true
			s.t.lay.needFull = true
		}
		rest := copy(s.q, s.q[n:])
		clear(s.q[rest:])
		s.q = s.q[:rest]
	}()
	for ; n < end; n++ {
		s.q[n].inQueue = false
		s.q[n].look(&p)
	}
}

// drawLate draws the canvases that the lists just recorded paint and that
// had not drawn, and records their groups again.
func (s *canvasTree) drawLate() {
	if len(s.late) == 0 {
		return
	}
	for i, c := range s.late {
		c.queue()
		s.late[i] = nil
	}
	s.late = s.late[:0]
	t := s.t
	L := &t.lay
	L.epoch++ // what they stamp is newer than the recording
	s.draw()
	if p := (painter{t: t, scale: float64(t.scale)}); L.nchg > 0 && !t.recordDirty(&p) {
		t.recordAll(&p)
	}
}

// look runs the function of canvas c if something it draws with changed and
// something paints it.
func (c *canvasBox) look(p *painter) {
	due := c.due
	c.due = false
	b, t := c.box, c.set.t
	if b.el == nil || !b.el.mounted {
		return
	}
	if b.pcs == nil || !t.inTree(b) {
		c.parked = true // it is placed when it is displayed
		return
	}
	key := c.keyOf(p)
	tp := &b.pcs.Text
	sameText := c.sameText(tp)
	if !c.stale && !due && key == c.key && sameText {
		return
	}
	if key.w <= 0 || key.h <= 0 || b.pcs.Visibility != style.VisibilityVisible || b.hiddenAtom ||
		b.rinN == 0 && !b.ink.Intersects(p.interestOf(b.space)) {
		c.stale, c.parked = true, true
		return
	}
	if c.fill == nil || key.fill != c.key.fill {
		c.fill = key.fill
	}
	if c.stroke == nil || key.stroke != c.key.stroke {
		c.stroke = key.stroke
	}
	if !sameText {
		c.setText(b.pcs)
	}
	now := t.anim.tk.Now()
	var delta time.Duration
	if c.running {
		if delta = now - c.last; delta < 0 || delta > canvasGap {
			delta = 0
		}
	}
	f := canvas.Frame{
		Width: float64(key.w) / float64(key.scale), Height: float64(key.h) / float64(key.scale),
		Time: c.time + delta, Delta: delta,
		Fill: c.fill, Stroke: c.stroke, LineWidth: float64(key.lineWidth),
		Font: c.font, LetterSpacing: float64(tp.LetterSpacing),
		Scale: float64(key.scale), Fonts: t.fonts, FontsGen: c.set.fonts, Text: &c.text, Dir: c.dir,
		Env: c, Dev: t.dev,
	}
	c.parked = false
	list, again := c.run(&f)
	c.key = key
	c.time, c.last, c.running = f.Time, now, again
	c.wants = again || c.wants && c.drawn && c.frame == t.frameStart // a draw of the same frame asked
	c.drawn, c.frame = true, t.frameStart
	if c.wants && !c.listed {
		c.listed = true
		c.set.wants = append(c.set.wants, c)
	}
	if list.Len() == 0 {
		list = nil
	}
	if sameDrawing(b.med.drawing, list) {
		return
	}
	b.med.drawing = list
	if b.rinN > 0 && b.rin.drawing != list {
		b.rin.drawing = list
		if L := &t.lay; b.rinAt != L.epoch {
			b.rinAt = L.epoch
			L.nchg++
		}
	}
}

// keyOf returns what the draw function sees of the element, whose box is
// placed.
func (c *canvasBox) keyOf(p *painter) canvasKey {
	b, t := c.box, c.set.t
	cs := b.pcs
	g := p.geom(b)
	r := p.contentRRect(b, &g).Rect
	cur := cs.Text.Color
	if t.textColorFns > 0 {
		if col, ok := t.fnTextColor(b.el); ok {
			cur = col
		}
	}
	k := canvasKey{
		w:     int(gfx.RoundDevice(float64(r.X1)*p.scale) - gfx.RoundDevice(float64(r.X0)*p.scale)),
		h:     int(gfx.RoundDevice(float64(r.Y1)*p.scale) - gfx.RoundDevice(float64(r.Y0)*p.scale)),
		scale: t.scale,
		fill:  canvasColor(cs.Fill, cur), stroke: canvasColor(cs.Stroke, cur),
		lineWidth: 1,
		theme:     t.cx.Theme, dark: cs.IsDark(&t.cx),
	}
	if cs.StrokeWidthSet {
		k.lineWidth = cs.StrokeWidth
	}
	return k
}

// sameText reports whether the style of text tp is that of the last draw, as
// far as a canvas sees it.
func (c *canvasBox) sameText(tp *style.TextProps) bool {
	s := &c.text
	return s.Size == tp.FontSize && s.Weight == tp.FontWeight && s.Style == tp.FontStyle && s.Stretch == tp.FontStretch &&
		s.LetterSpacing == tp.LetterSpacing && s.WordSpacing == tp.WordSpacing && s.Numeric == tp.Numeric &&
		s.Language == tp.Language && c.dir == tp.Direction && slices.Equal(s.Families, tp.FontFamily)
}

// setText takes the style of text of cs.
func (c *canvasBox) setText(cs *style.ComputedStyle) {
	c.text, c.dir = cs.TextStyle(), cs.Text.Direction
	c.text.Paint, c.text.Decoration, c.text.Transform = nil, text.TextDecoration{}, text.TransformNone
	c.font = canvas.CanvasFont{
		Family: canvas.JoinFamilies(c.text.Families),
		Size:   float64(c.text.Size), Weight: float64(c.text.Weight), Italic: c.text.Style != text.StyleNormal,
	}
}

// canvasColor returns the color of the fill or the stroke p of an element
// whose text color is cur: the text color without one.
func canvasColor(p style.SVGPaint, cur gfx.Color) gfx.Color {
	switch p.Kind {
	case style.SVGPaintNone:
		return gfx.Transparent
	case style.SVGPaintColor, style.SVGPaintCurrent:
		return p.Resolve(cur)
	}
	return cur
}

// run runs the function of canvas c, tracking the Values it reads.
func (c *canvasBox) run(f *canvas.Frame) (list *gfx.DisplayList, again bool) {
	s, t := c.set, c.set.t
	if s.rec == nil {
		s.rec = &canvas.Recorder{}
	}
	// The function may mark the tree (it moves the focus, scrolls): the end
	// of the frame would take the marks for what it just did.
	changed, repaint := t.changed, t.repaint
	t.changed, t.repaint = false, false
	c.stale = false // unless a Value it read changes meanwhile
	c.using = nil
	mark := t.beginTrack()
	done := false
	defer func() {
		if !done { // it panicked: keep the old subscriptions
			c.stale = true
			t.endTrack(mark)
		}
		s.more = s.more || t.changed || t.repaint
		t.changed, t.repaint = t.changed || changed, t.repaint || repaint
	}()
	if t.dev {
		if drawGuard.Load() == nil {
			guard := guardDraw
			drawGuard.Store(&guard)
		}
		drawFrame(func() { list, again = s.rec.Record(f, c.draw) })
	} else {
		list, again = s.rec.Record(f, c.draw)
	}
	done = true
	c.uses, c.using = c.using, nil
	t.observe(&c.obs, obsCanvas, nil, c.box, t.endTrack(mark))
	return list, again
}

// Color implements canvas.Env: a color of the theme, in the element's
// scheme.
func (c *canvasBox) Color(token string) gfx.Color {
	t := c.set.t
	return ThemeValues{th: t.cx.Theme, dark: c.box.pcs.IsDark(&t.cx), dev: t.dev}.color("Canvas", token)
}

// Image implements canvas.Env: img converted, once while a canvas or a
// background shows it.
func (c *canvasBox) Image(img image.Image) *gfx.Image {
	a := convertBgImage(img)
	c.use(a)
	return a.raster
}

// Icon implements canvas.Env: the document of an icon, parsed once while
// something shows it, as h.Icon's.
func (c *canvasBox) Icon(svgText string) *svg.Doc {
	s := srcSpec{kind: srcIcon, icon: svgText}
	a, err := loadSource(&s)
	if err != nil {
		c.set.t.reportOnce(s.reportKey(), "tela: Canvas.DrawIcon: "+err.Error())
		return nil
	}
	c.use(a)
	return a.doc
}

// use holds an image for as long as what the draw in progress draws.
func (c *canvasBox) use(a *imageAsset) {
	if !slices.Contains(c.using, a) {
		c.using = append(c.using, a)
	}
}

// sameDrawing reports whether two lists (nil for none) draw the same.
func sameDrawing(a, b *gfx.DisplayList) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Len() != b.Len() {
		return false
	}
	for i := range a.Len() {
		if a.BoundsAt(i) != b.BoundsAt(i) || !gfx.Equal(a.At(i), b.At(i)) {
			return false
		}
	}
	return true
}

// paint paints what the canvas drew in its content box r, clipped to its
// rounded corners (the list is clipped to the box's edges).
func (c *canvasBox) paint(b *gfx.Builder, r gfx.RRect, ref bool) {
	if c.stale && !ref {
		c.set.late = append(c.set.late, c) // drawLate
	}
	list := c.box.med.drawing
	if list == nil {
		return
	}
	round := !r.IsRect()
	if round {
		b.PushClipRRect(r)
	}
	b.PushTransform(gfx.Translate(r.Rect.X0, r.Rect.Y0))
	b.AppendList(list, nil)
	b.PopTransform()
	if round {
		b.PopClip()
	}
}

// ---------------------------------------------------------------------
// Frames.

// tick makes the canvases on screen that asked for a frame due: one starts.
func (s *canvasTree) tick() {
	for _, c := range s.wants {
		if c.wants && c.on {
			c.due = true
			c.queue()
			s.t.repaint = true
		}
	}
}

// afterFrame ends a Frame, outside it: the work that a draw function left is
// another frame's, and the canvases on screen that asked for a frame get
// one.
func (s *canvasTree) afterFrame() {
	t := s.t
	if s.more {
		s.more = false
		t.dirty()
	}
	if len(s.wants) == 0 {
		return
	}
	if s.frames != t.stats.Frames {
		s.frames = t.stats.Frames
		s.wants = slices.DeleteFunc(s.wants, func(c *canvasBox) bool {
			c.listed = c.wants
			c.visible = c.wants && t.textureBoxVisible(c.box)
			return !c.wants
		})
	}
	s.syncOn()
}

// syncOn finds which of the canvases that asked for a frame are on screen in
// a window that gets frames, after a frame and when the window is hidden,
// minimized or shown again (Host), and asks for a frame for them. The clocks
// of the others stop.
func (s *canvasTree) syncOn() {
	if len(s.wants) == 0 {
		return
	}
	t := s.t
	paused := t.wv != nil && t.wv.paused()
	ask := false
	for _, c := range s.wants {
		c.on = c.wants && c.visible && !paused
		if c.on {
			ask = true
		} else {
			c.running = false
		}
	}
	if ask {
		t.requestFrame()
	}
}

func (s *canvasTree) running() int {
	n := 0
	for _, c := range s.wants {
		if c.wants && c.on {
			n++
		}
	}
	return n
}

// canvasesRunning counts the canvases on screen that asked for a frame.
func (t *Tree) canvasesRunning() int {
	if t.cnv == nil {
		return 0
	}
	return t.cnv.running()
}

// OnlyCanvasFrames reports that the frame the tree's window was asked for is
// one that canvases asked for (Canvas.RequestFrame), and endless animations,
// and nothing else needs one: as with those, nothing waits for such frames
// (telatest does not settle for them: a canvas that asks for a frame from
// every draw never ends).
func (t *Tree) OnlyCanvasFrames() bool {
	return t.canvasesRunning() > 0 && !t.changed && !t.repaint && !t.updatesQueued() &&
		(!t.Animating() || t.anim.tk.OnlyEndless()) && !t.scrollAnimating()
}

// ---------------------------------------------------------------------
// What a draw function must not do.

// drawGuard is guardDraw once a draw function ran in dev mode, nil before.
var drawGuard atomic.Pointer[func(call string, hook bool)]

// checkDraw panics when call, which changes the app, is made in a draw
// function in dev mode; otherwise what it changes applies to the next frame.
// Any goroutine.
func checkDraw(call string) {
	if guard := drawGuard.Load(); guard != nil {
		(*guard)(call, false)
	}
}

// checkDrawHook panics when a hook is called in a draw function in dev mode.
func checkDrawHook(hook string) {
	if guard := drawGuard.Load(); guard != nil {
		(*guard)(hook, true)
	}
}

// guardDraw panics when call, a hook or a call that changes the app, is made
// in a draw function in dev mode.
func guardDraw(call string, hook bool) {
	switch {
	case !inDraw():
	case hook:
		panic(fmt.Sprintf("tela: %s called in the draw function of a Canvas%s, which runs while the frame is painted, not while the component builds: call hooks in the component's body, before the element, and use what they return in the function", call, drawSite()))
	default:
		panic(fmt.Sprintf("tela: %s called in the draw function of a Canvas%s, which runs while the frame is painted: what it changes is not in that frame. Change state in event handlers and effects; what a simulation moves from draw to draw goes in a tela.UseRef, and c.RequestFrame() asks for the next draw", call, drawSite()))
	}
}

// drawSite returns where the application made the call that guardDraw
// reports, " (file.go:12)", or "".
func drawSite() string {
	var pcs [32]uintptr
	frames := runtime.CallersFrames(pcs[:runtime.Callers(3, pcs[:])])
	for {
		f, more := frames.Next()
		if f.Function != "" && !frameworkFrame(f.Function, f.File) {
			return fmt.Sprintf(" (%s:%d)", filepath.Base(f.File), f.Line)
		}
		if !more {
			return ""
		}
	}
}

// drawing counts the draw functions running in dev mode in the process: only
// then does a write look at its stack.
var drawing atomic.Int32

// drawFrame runs f, a draw function in dev mode, as the frame inDraw looks
// for.
//
//go:noinline
func drawFrame(f func()) {
	drawing.Add(1)
	defer drawing.Add(-1)
	f()
}

// drawFramePC returns the return address of drawFrame's call of f, as
// runtime.Callers records it for the frame of drawFrame: the same for every
// f, since drawFrame makes one call.
func drawFramePC() uintptr {
	var pc [1]uintptr
	drawFrame(func() { runtime.Callers(2, pc[:]) })
	return pc[0]
}

// inDraw reports whether the caller runs in a draw function of a tree in dev
// mode, on its goroutine: drawFrame is on its stack. Unlike the reads of a
// Value, whose tracking cannot tell one goroutine from another (value.go),
// this has no false positives: a goroutine that sets state while a draw runs
// has a stack of its own. Any goroutine.
func inDraw() bool {
	if drawing.Load() == 0 {
		return false
	}
	at := drawFramePC()
	var buf [64]uintptr
	pcs := buf[:]
	for {
		n := runtime.Callers(2, pcs)
		if slices.Contains(pcs[:n], at) {
			return true
		}
		if n < len(pcs) {
			return false
		}
		pcs = make([]uintptr, 4*len(pcs)) // a deep stack: all of it
	}
}
