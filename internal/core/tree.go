package core

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// EnvDev is the environment variable that turns dev mode on ("1").
const EnvDev = "TELA_DEV"

// Config configures a Tree.
type Config struct {
	// Fonts shapes the text. Nil means text.NewDefaultCollection() (the
	// embedded fonts only; Run also enables the system fonts).
	Fonts *text.Collection
	// Parser compiles the classes, bound to a theme. Nil means
	// style.DefaultParser().
	Parser *style.Parser
	// RootClass are classes for the root box, which fills the window. Its
	// background is the window's; without one the window is white, the
	// browser canvas default.
	RootClass string
	// Dark and ReducedMotion are the initial environment of the dark: and
	// motion-reduce: variants (a Host takes them from the system).
	Dark, ReducedMotion bool
	// Dev turns dev mode on: warnings about lists without keys, and class
	// errors on stderr when Log is nil. TELA_DEV=1 does the same.
	Dev bool
	// Log receives dev messages (class errors, warnings), one per call.
	// Nil means stderr in dev mode, and nothing otherwise.
	Log func(msg string)
	// IDPrefix starts the strings of UseId. Empty means "t<n>-", where
	// <n> numbers the Trees of the process in base 36 (Render uses "r-"),
	// so IDs are unique within the process.
	IDPrefix string
}

// Stats counts the work of a Tree, for tests and benchmarks.
type Stats struct {
	Frames     int // Frame calls that ran the pipeline
	Builds     int // component builds
	Skips      int // component builds skipped (equal props, or the same node)
	Mounts     int // Elements mounted
	Unmounts   int // Elements unmounted
	Styles     int // style computations
	Structures int // box structure passes
	Shapes     int // paragraphs shaped (text.NewLayout)
	Breaks     int // paragraph line breakings (text.Layout.BreakLines)
	Layouts    int // layout passes (layout.Engine.Compute)
	Paints     int // paint passes
}

// Tree is a mounted UI: the Element tree, the box tree and the frame
// pipeline (ARCHITECTURE section 8). Everything except the state setters
// (and SetWake) must be used from one goroutine, the UI thread.
type Tree struct {
	fonts       *text.Collection
	parser      *style.Parser
	cx          style.Context
	rootClasses []string
	dev         bool
	log         func(string)

	root     *Element
	rootBox  *Box
	viewport gfx.Size
	// scale is the display scale the paint phase snaps box edges for
	// (device pixels per logical pixel).
	scale float32

	engine  layout.Engine
	builder *gfx.Builder
	asm     paraAssembler

	// Work queues of the phases (UI thread).
	buildQ, styleQ, boxQ []*Element
	restyled             []*Box
	remapQ               []*Box
	frame                uint64 // frame counter: the validity of per-frame scratch state

	// changed reports work since the last Frame; onDirty requests a frame
	// (once until the next Frame), except while a Frame runs.
	changed bool
	inFrame bool
	asked   bool
	onDirty func()

	// State updates from any goroutine.
	mu      sync.Mutex
	updates []update
	wake    func()

	reported map[string]bool // class strings whose errors were reported

	flatFree [][]flatEntry
	itemFree [][]*Element
	boxFree  [][]*Box

	// Effects (UI thread): queued by builds for the layout commit
	// (layoutFx, passiveFx, with the cleanups of unmounted components),
	// and passive ones committed and waiting for FlushEffects (ready*).
	layoutFx, passiveFx, readyFx       []*effectSlot
	layoutGone, passiveGone, readyGone []func()

	// UseId.
	idPrefix string
	idNext   uint64

	// Value reads (value.go). trackDepth is the depth of the evaluations
	// whose reads are tracked (builds, deferred props); Get reads it from
	// any goroutine and appends to reads under trackMu.
	trackDepth atomic.Int32
	trackMu    sync.Mutex
	reads      []*valueCore
	readsTmp   []*valueCore // UI thread scratch of endTrack
	evalID     uint64       // UI thread: numbers resubscriptions
	// relayoutQ are the boxes whose layout-phase reads changed; paintQ
	// those whose paint-phase reads changed. repaint reports paint work
	// with no other work (a paint-only frame).
	relayoutQ []*Box
	paintQ    []*Box
	repaint   bool

	// Pointer input (input.go) and focus (focus.go).
	in  inputState
	foc focusState

	lastDL *gfx.DisplayList
	stats  Stats
}

// treeSerial numbers the Trees of the process (default UseId prefixes).
var treeSerial atomic.Uint64

// NewTree returns an empty Tree; Mount gives it content and SetViewport a
// size.
func NewTree(cfg Config) *Tree {
	t := &Tree{
		fonts:    cfg.Fonts,
		parser:   cfg.Parser,
		dev:      cfg.Dev || os.Getenv(EnvDev) == "1",
		log:      cfg.Log,
		builder:  gfx.NewBuilder(),
		reported: map[string]bool{},
		viewport: gfx.Size{W: 800, H: 600},
		scale:    1,
	}
	t.foc.window = true
	if t.fonts == nil {
		t.fonts = text.NewDefaultCollection()
	}
	if t.parser == nil {
		t.parser = style.DefaultParser()
	}
	if cfg.RootClass != "" {
		t.rootClasses = []string{cfg.RootClass}
	}
	if t.log == nil && t.dev {
		t.log = func(msg string) { fmt.Fprintln(os.Stderr, msg) }
	}
	if t.dev {
		devAny.Store(true)
	}
	t.idPrefix = cfg.IDPrefix
	if t.idPrefix == "" {
		t.idPrefix = "t" + strconv.FormatUint(treeSerial.Add(1)-1, 36) + "-"
	}
	t.cx = style.Context{Theme: t.parser.Theme(), Viewport: t.viewport, Dark: cfg.Dark, ReducedMotion: cfg.ReducedMotion}
	return t
}

// SetWake sets the function a state setter calls after queueing an update
// (a backend's Wake), so that the UI thread applies it. Safe from any
// goroutine.
func (t *Tree) SetWake(wake func()) {
	t.mu.Lock()
	t.wake = wake
	t.mu.Unlock()
}

// SetRequestFrame sets the function called on the UI thread when the
// tree needs a frame (a window's RequestFrame). It is called at most once
// between two Frames, and never while Frame runs.
func (t *Tree) SetRequestFrame(f func()) { t.onDirty = f }

// dirty records that the next Frame has work, and requests it.
func (t *Tree) dirty() {
	t.changed = true
	if t.inFrame || t.asked || t.onDirty == nil {
		return
	}
	t.asked = true
	t.onDirty()
}

// Dirty reports whether the next Frame has work to do: state updates,
// changed elements, a new viewport, boxes to repaint.
func (t *Tree) Dirty() bool {
	if t.changed || t.repaint {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.updates) > 0
}

func (t *Tree) logf(format string, args ...any) {
	if t.log != nil {
		t.log(fmt.Sprintf(format, args...))
	}
}

// Stats returns the work counters.
func (t *Tree) Stats() Stats { return t.stats }

// Mount sets the content of the tree: root becomes the child of the root
// box, which fills the viewport. Mounting again reconciles the new root
// with the old one (state is kept where types and keys match). The
// components are built immediately; the other phases run in Frame, and
// the effects of these builds run with the next Frame's commit. Passive
// effects of a previous frame that were not flushed run first.
func (t *Tree) Mount(root Node) {
	t.FlushEffects()
	buildMu.Lock()
	defer buildMu.Unlock()
	n := Node{&desc{kind: nodeElement, elem: style.KindRoot, classes: t.rootClasses, children: []Node{root}}}
	if t.root == nil {
		el := &Element{tree: t, kind: elemPrimitive, node: n, mounted: true}
		el.box = newBox(t, el, style.KindRoot)
		t.root, t.rootBox = el, el.box
		t.markStyle(el)
		t.markBoxes(el)
		t.reconcile(el, n.d.children)
		return
	}
	t.update(t.root, n)
}

// Unmount removes the content of the tree, running the cleanups of its
// effects now (after the passive effects not flushed yet). Effects that
// never ran are dropped. Mount can give the tree content again.
func (t *Tree) Unmount() {
	t.FlushEffects()
	t.resetInput()
	if t.root != nil {
		buildMu.Lock()
		t.update(t.root, Node{&desc{kind: nodeElement, elem: style.KindRoot, classes: t.rootClasses}})
		buildMu.Unlock()
	}
	t.resetFocus()
	t.dropEffects()
}

// Viewport returns the logical size of the viewport.
func (t *Tree) Viewport() gfx.Size { return t.viewport }

// SetViewport sets the logical size of the window: the root box's size,
// the vw and vh units and the breakpoint variants.
func (t *Tree) SetViewport(size gfx.Size) {
	if size == t.viewport {
		return
	}
	t.viewport = size
	t.cx.Viewport = size
	t.restyleAll() // perf: only elements whose classes use vw, vh or breakpoints
}

// Scale returns the display scale the tree paints for.
func (t *Tree) Scale() float32 { return t.scale }

// SetScale sets the display scale (device pixels per logical pixel; zero
// or less means 1). The display list stays in logical pixels, but the
// paint phase snaps box edges to device pixels at this scale, so a new
// scale repaints the tree.
func (t *Tree) SetScale(scale float32) {
	if !(scale > 0) || math.IsInf(float64(scale), 0) {
		scale = 1
	}
	if scale == t.scale {
		return
	}
	t.scale = scale
	t.dirty()
}

// SetEnvironment sets dark mode and reduced motion (the dark: and
// motion-reduce: variants).
func (t *Tree) SetEnvironment(dark, reducedMotion bool) {
	if dark == t.cx.Dark && reducedMotion == t.cx.ReducedMotion {
		return
	}
	t.cx.Dark, t.cx.ReducedMotion = dark, reducedMotion
	t.restyleAll()
}

// restyleAll queues every primitive for the style phase.
func (t *Tree) restyleAll() {
	t.dirty()
	if t.root == nil {
		return
	}
	var walk func(el *Element)
	walk = func(el *Element) {
		if el.kind == elemPrimitive {
			t.markStyle(el)
		}
		for _, c := range el.children {
			walk(c)
		}
	}
	walk(t.root)
}

// Frame runs the pipeline (ARCHITECTURE section 8) up to paint: the
// passive effects of a previous frame that were not flushed, state and
// Value updates and posted functions, Build (components marked
// needsBuild, in tree order), Style (dirty primitives; inherited changes
// propagate), box structure and paragraphs, Layout (dirty boxes), the
// layout effects and Paint. When the only work is repainting (Values read
// by paint-phase deferred props changed), the phases before paint are
// skipped. It returns the display list of the whole tree, in logical
// pixels, and whether it changed since the previous Frame (when nothing
// was dirty the previous list is returned). The passive effects of the
// frame wait for FlushEffects.
func (t *Tree) Frame() (dl *gfx.DisplayList, changed bool) {
	t.inFrame = true
	defer func() { t.inFrame = false }()
	t.asked = false
	t.FlushEffects()
	t.ApplyUpdates()
	if !t.changed && !t.repaint && t.lastDL != nil {
		t.commitLayout()
		return t.lastDL, false
	}
	t.frame++
	t.stats.Frames++
	if t.changed || t.lastDL == nil {
		t.buildPhase()
		t.stylePhase()
		if t.focusPending() {
			// Focus fix-up, AutoFocus and scope restoration need the new
			// styles; the states they change are styled now.
			t.focusWork()
			t.stylePhase()
		}
		t.boxPhase()
		t.layoutPhase()
	}
	t.commitLayout()
	if len(t.styleQ) > 0 || len(t.boxQ) > 0 {
		// A layout effect moved the focus (ElementRef.Focus): style and
		// lay out again before painting.
		t.stylePhase()
		t.fixFocus()
		t.stylePhase()
		t.boxPhase()
		t.layoutPhase()
	}
	t.lastDL = t.paintPhase()
	t.changed, t.repaint = false, false
	return t.lastDL, true
}

// buildPhase builds the components marked needsBuild in tree order,
// descending from the root along the dirtyKids marks (so a component is
// built before the components below it, also through components that
// skip their build, and effects run children first); building a
// component updates its subtree, so a descendant that was also marked is
// built once. The queue catches whatever the descent did not reach,
// shallowest first.
func (t *Tree) buildPhase() {
	if len(t.buildQ) == 0 {
		return
	}
	buildMu.Lock()
	defer buildMu.Unlock()
	for len(t.buildQ) > 0 {
		q := t.buildQ
		t.buildQ = nil
		if t.root != nil && t.root.dirtyKids {
			t.descend(t.root)
		}
		sortByDepth(q)
		for _, el := range q {
			if el.mounted && el.needsBuild {
				t.build(el)
			}
		}
	}
}

// layoutPhase maps the changed layout styles and lays the tree out into
// the viewport.
func (t *Tree) layoutPhase() {
	if t.rootBox == nil {
		return
	}
	t.remapPhase()
	t.stats.Layouts++
	t.engine.Compute(t.rootBox, layout.DefiniteSpace(t.viewport.W), layout.DefiniteSpace(t.viewport.H))
}

// CanvasColor returns the color of the window background: the root box's
// background over white.
func (t *Tree) CanvasColor() gfx.Color {
	c := gfx.White
	if t.rootBox == nil || t.rootBox.cs == nil {
		return c
	}
	bg := t.rootBox.cs.BackgroundColor.Clamp()
	if bg.A >= 1 {
		return bg
	}
	k := 1 - bg.A
	return gfx.Color{
		R: float32(bg.R*bg.A) + k,
		G: float32(bg.G*bg.A) + k,
		B: float32(bg.B*bg.A) + k,
		A: 1,
	}
}
