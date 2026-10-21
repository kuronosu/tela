package core

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// EnvDev is the environment variable that turns dev mode on ("1") or off
// ("0") whatever the build and the app choose (ADR-085).
const EnvDev = "TELA_DEV"

// Config configures a Tree.
type Config struct {
	// Fonts shapes the text. Nil means text.NewDefaultCollection() (the
	// embedded fonts only; Run also enables the system fonts).
	Fonts *text.Collection
	// Parser compiles the classes, bound to a theme. Nil means the
	// parser of Theme and ClassAliases (NewParser), style.DefaultParser()
	// when both are empty.
	Parser *style.Parser
	// Theme holds the color tokens, radius and fonts (theme.go); nil is
	// DefaultTheme.
	Theme *Theme
	// ClassAliases are class names that expand to class strings
	// (style.NewParserAliases).
	ClassAliases map[string]string
	// RootClass are classes for the root box, which fills the window. Its
	// background is the window's; without one the window is white, the
	// browser canvas default. The root is the viewport: it scrolls on
	// both axes (overflow auto) unless these classes set an overflow
	// (scroll.go).
	RootClass string
	// Dark and ReducedMotion are the system's initial preferences (a
	// Host takes them from the platform): the dark: variants, unless
	// ColorScheme overrides the scheme, and motion-reduce: (env.go).
	Dark, ReducedMotion bool
	// ColorScheme is the app's color scheme; the zero value follows the
	// system (Dark).
	ColorScheme ColorScheme
	// Dev turns dev mode on: the hook checks, class errors on stderr when
	// Log is nil, and the dev warnings. TELA_DEV=1 turns it on and
	// TELA_DEV=0 off whatever Dev says; callers resolve the default with
	// ResolveDev (on unless the binary is a release build, ADR-085).
	Dev bool
	// Log receives dev errors (unknown classes, an image that cannot be
	// loaded...), one per call. Nil means stderr in dev mode, and nothing
	// otherwise.
	Log func(msg string)
	// Warn receives dev warnings: code that works as written but is
	// likely a mistake (a list without keys, nil dependencies, a Value
	// read while changes are queued...). It may be called from any
	// goroutine. Nil means Log.
	Warn func(msg string)
	// IDPrefix starts the strings of UseID. Empty means "t<n>-", where
	// <n> numbers the Trees of the process in base 36 (Render uses "r-"),
	// so IDs are unique within the process.
	IDPrefix string
	// ClassicScrollbars draws always-visible classic scrollbars in a
	// reserved gutter (a Windows look) instead of overlay bars; the
	// classes scrollbar-classic and scrollbar-overlay override it per
	// element (scrollbar.go).
	ClassicScrollbars bool
	// InstantScroll makes user scrolling jump, without animation or
	// kinetic flings, except in containers with scroll-smooth
	// (scroll.go).
	InstantScroll bool
	// Debug selects the debug overlays drawn over the application
	// (debug.go). Zero means unset: NewHost then reads TELA_DEBUG;
	// DebugOff means none and ignores the environment.
	Debug DebugFlags
	// DebugClock is the clock the fps overlay times frames with (the
	// work of a frame is the difference of two readings). Nil means a
	// monotonic wall clock; tests pass a deterministic one.
	DebugClock func() time.Duration
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
	warn        func(string)
	// onceMu guards onceSeen, the keys of the messages already given once
	// (Tree.once: warnings, image and animation errors), which may come
	// from any goroutine.
	onceMu   sync.Mutex
	onceSeen map[string]bool
	// svgRecords counts the SVG documents recorded for Img and Icon
	// boxes (media.go; tests).
	svgRecords int
	// devLabels are the Labels mounted since the last check (dev mode,
	// form.go).
	devLabels []*Element
	// intervals are the running UseInterval and UseTimeout slots
	// (interval.go).
	intervals []*intervalSlot
	// locals are the values of TreeLocal (engine.go).
	locals map[any]any
	// autoDirs are the elements with Dir("auto") and whether their text
	// was right to left when they were last styled (direction.go).
	autoDirs map[*Element]bool
	// autoStale: some element of autoDirs is marked autoStale;
	// autoWalks counts the walks of autoRTL (tests).
	autoStale bool
	autoWalks int

	root     *Element
	rootBox  *Box
	viewport gfx.Size
	// scale is the display scale the paint phase snaps box edges for
	// (device pixels per logical pixel).
	scale float32

	engine layout.Engine
	// asm holds the paragraph assemblers: the structure of an atomic
	// inline is built while its paragraph is assembled.
	asm []*paraAssembler

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
	// handlerDepth counts the event dispatches that are running handlers:
	// a click made from a handler (ElementRef.Click) leaves applying the
	// updates and building to the event that is being handled.
	handlerDepth int

	// State updates from any goroutine.
	mu      sync.Mutex
	updates []update
	wake    func()

	// classReported are the class strings whose errors were reported
	// (UI thread); a new theme clears it (setParser).
	classReported map[string]bool

	flatFree [][]flatEntry
	itemFree [][]*Element
	boxFree  [][]*Box

	// Effects (UI thread): queued by builds for the layout commit
	// (layoutFx, passiveFx, with the cleanups of unmounted components),
	// and passive ones committed and waiting for FlushEffects (ready*).
	layoutFx, passiveFx, readyFx       []*effectSlot
	layoutGone, passiveGone, readyGone []func()

	// UseID.
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

	// Pointer input (input.go), focus (focus.go) and scrolling
	// (scroll.go).
	in  inputState
	foc focusState
	scr scrollState
	// virt holds the virtual lists (virtual.go).
	virt virtState

	// bnd holds the UseBounds slots (bounds.go).
	bnd boundsState

	// sem is the semantics tree built last (semantics.go).
	sem semState

	// Text fields (field.go, fieldime.go, form.go): the clipboard, the
	// blinking caret, the IME state, the mounted elements by Id and the
	// mounted Labels by For (form.go).
	clip   platform.Clipboard
	caret  caretState
	ime    imeState
	ids    map[string][]*Element
	labels map[string][]*Element

	// lay holds the layers and the composed frames (layer.go).
	lay layerState
	// stickies are the sticky boxes of the last place, in tree order
	// (sticky.go).
	stickies []*Box
	// portals are the mounted Portal elements, in mount order (portal.go).
	portals []*Element
	// flt is the float pass state (floating.go), ovl the overlays, the
	// interact-outside listeners and the modality (overlay.go), and pubs
	// the Values published in the frame (PublishValue).
	flt  floatTree
	ovl  overlayState
	pubs []*valueCore
	// env is the color scheme and reduced motion (env.go).
	env envState

	// anim is the state of the transitions and keyframe animations
	// (anim.go).
	anim animState
	// la is the state of the layout animations (layoutanim.go).
	la layoutState
	// pres holds the exits of Presence children (presence.go).
	pres presenceState
	// textColorFns counts the mounted boxes with a TextColorFn: while it
	// is 0 paragraphs paint their styles' colors as laid out (paint.go).
	textColorFns int

	// dbg is the state of the debug overlays (debug.go).
	dbg debugState

	lastDL *gfx.DisplayList
	stats  Stats
}

// treeSerial numbers the Trees of the process (default UseID prefixes).
var treeSerial atomic.Uint64

// NewTree returns an empty Tree; Mount gives it content and SetViewport a
// size.
func NewTree(cfg Config) *Tree {
	t := &Tree{
		fonts:         cfg.Fonts,
		parser:        cfg.Parser,
		dev:           configDev(cfg.Dev),
		log:           cfg.Log,
		warn:          cfg.Warn,
		classReported: map[string]bool{},
		viewport:      gfx.Size{W: 800, H: 600},
		scale:         1,
	}
	t.foc.window = true
	t.dbg.flags = cfg.Debug & DebugAll
	t.dbg.clock = cfg.DebugClock
	t.scr.classic, t.scr.instant = cfg.ClassicScrollbars, cfg.InstantScroll
	if t.fonts == nil {
		t.fonts = text.NewDefaultCollection()
	}
	if t.parser == nil {
		t.parser = themeParser(&cfg)
	}
	if cfg.RootClass != "" {
		t.rootClasses = []string{cfg.RootClass}
	}
	if t.log == nil && t.dev {
		t.log = func(msg string) { fmt.Fprintln(os.Stderr, msg) }
	}
	if t.warn == nil {
		t.warn = t.log
	}
	noteTreeDev(t.dev)
	t.idPrefix = cfg.IDPrefix
	if t.idPrefix == "" {
		t.idPrefix = "t" + strconv.FormatUint(treeSerial.Add(1)-1, 36) + "-"
	}
	checkScheme(cfg.ColorScheme)
	t.env = envState{sysDark: cfg.Dark, sysReduced: cfg.ReducedMotion, scheme: cfg.ColorScheme}
	t.cx = style.Context{Theme: t.parser.Theme(), Viewport: t.viewport, Dark: t.env.effectiveDark(), ReducedMotion: cfg.ReducedMotion}
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
	t.lockBuild()
	defer unlockBuild()
	n := Node{&desc{kind: nodeElement, elem: style.KindRoot, classes: t.rootClasses, children: []Node{root}}}
	if t.root == nil {
		el := &Element{tree: t, kind: elemPrimitive, node: n, mounted: true}
		el.box = newBox(t, el, style.KindRoot)
		t.root, t.rootBox = el, el.box
		t.initEnv()
		t.markStyle(el)
		t.markBoxes(el)
		t.reconcile(el, n.d.children)
		t.syncModal()
		return
	}
	t.update(t.root, n)
	t.syncModal()
}

// Unmount removes the content of the tree, running the cleanups of its
// effects now (after the passive effects not flushed yet). Effects that
// never ran are dropped. Mount can give the tree content again.
func (t *Tree) Unmount() {
	t.FlushEffects()
	t.resetInput()
	if t.root != nil {
		t.lockBuild()
		t.update(t.root, Node{&desc{kind: nodeElement, elem: style.KindRoot, classes: t.rootClasses}})
		unlockBuild()
		t.syncModal()
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
	defer t.afterFrame()
	t.asked = false
	t.FlushEffects()
	t.ApplyUpdates()
	// The scroll events of the offsets changed since the previous frame
	// (the DOM's scroll steps, before the frame's build).
	if t.fireScrollEvents() {
		t.ApplyUpdates()
	}
	if !t.changed && !t.repaint && t.lastDL != nil {
		t.commitLayout()
		return t.lastDL, false
	}
	t.frame++
	t.stats.Frames++
	// A frame that builds, restyles or lays out records every layer; one
	// that only repaints records only what changed (layer.go).
	full := t.changed || t.lastDL == nil
	if full {
		t.buildPhase()
		if len(t.devLabels) > 0 {
			t.checkLabels()
		}
		t.stylePhase()
		if t.presencePhase() {
			t.stylePhase() // the structural variants of the siblings
		}
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
		full = true
	}
	full = t.scrollPhase(full)
	if full {
		t.syncSizes() // UseSize (bounds.go): paint-phase readers see it now
	}
	t.lastDL = t.paintPhase(full)
	t.changed, t.repaint = false, false
	return t.lastDL, true
}

// afterFrame ends Frame: the work that must request the next frame runs
// outside it (scroll.go's scrollAfterFrame).
func (t *Tree) afterFrame() {
	t.inFrame = false
	t.publishAfterFrame()
	t.boundsAfterFrame()
	t.scrollAfterFrame()
	t.animAfterFrame()
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
	t.lockBuild()
	defer unlockBuild()
	// The builds may mount, unmount or exit overlays (overlay.go).
	defer t.syncModal()
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
