package core

import (
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/style"
)

// DevTools instrumentation of a Tree (ADR-299): what `tela devtools`,
// telatest and `tela mcp` read about a running tree, as Go calls on the
// Tree. A later layer carries them over the tools socket; nothing here
// knows a transport.
//
//   - Elements with identity (Elements, Element): an ElementID per
//     Element, never reused within a Tree; per element its kind as a Go
//     user reads it (main.Settings, kit.Button, div), key, creation site
//     (instrument_site.go), the box it owns and its node in the last
//     semantics tree (InspectNode.Element, ElementOfNode and NodeOfElement
//     join them with tela inspect's JSON and the aria snapshot), its hooks
//     (kind, call site, value), the props of a struct component, its
//     builds since the instrumentation turned on and why its last builds
//     ran (BuildReason, recorded where builds are scheduled: markBuild's
//     callers).
//   - A profile of the last ProfileFrames frames (FrameProfiles,
//     instrument_prof.go).
//   - Pick mode and a highlight drawn by the debug overlays
//     (SetPick, Highlight; instrument_pick.go).
//   - A ring of the dev warnings and errors the tree reports (Logs,
//     instrument_log.go).
//
// # Cost
//
// Off by default (SetInstrument). Off, every hook point is a nil check of
// Tree.ins (one atomic load in the node constructors, insSites): nothing
// allocates per element or per frame, and no type that is allocated per
// element or per node gains a field: the per-element state lives in maps
// of the Tree's instrument, keyed by *Element, created as elements are
// first seen. On, a build costs a map lookup and a few appends, a frame a
// few clock readings, and in dev mode every node constructor captures its
// call stack (8 return addresses, resolved only when listed). The parts
// that capture call stacks (creation sites; hook call sites are dev mode's
// own record, ADR-085) need dev mode: a tree out of dev mode lists no
// sites.
//
// The instrumentation is in every build, not only teladev ones, so that
// telatest and tela mcp can use it on their headless trees; tela dev
// turns it on only while a client is attached (ADR-299), but its children
// capture creation sites from their first build (devrun.go), so the
// elements built before a client attached have theirs.

// ElementID identifies an Element of an instrumented Tree. IDs count up
// from 1 per Tree and are never reused within it: an element keeps its ID
// while it is mounted and the instrumentation stays on (turned off and on
// again, the elements get new IDs). 0 is no element.
type ElementID uint64

// instrument is the state of an instrumented Tree (Tree.ins), on its UI
// thread.
type instrument struct {
	t *Tree
	// els holds the state of the elements seen since the instrumentation
	// turned on, byID the same elements by ID; unmounted elements are
	// removed (Tree.unmount).
	els  map[*Element]*elemIns
	byID map[ElementID]*Element
	// sites caches the resolved creation sites, hookSites the resolved
	// call sites of hooks, by stack.
	sites, hookSites map[callStack]SourceSite
	// dev: the tree is in dev mode and counts in insSites.
	dev  bool
	prof profiler
	pick pickState
}

// elemIns is what the instrumentation keeps of an element.
type elemIns struct {
	id     ElementID
	builds int
	// pend are the causes of the build the element is marked for, in the
	// order they were recorded (maxPendingCauses at most).
	pend []cause
	// hist holds the last maxBuildHistory builds: a ring, oldest at at
	// once full.
	hist [maxBuildHistory]buildRec
	n    int // records in hist
	at   int // next record
}

// buildRec is one build of an element.
type buildRec struct {
	frame  int
	causes []cause
}

const (
	// maxPendingCauses bounds the causes recorded for one build.
	maxPendingCauses = 4
	// maxBuildHistory bounds the builds kept per element.
	maxBuildHistory = 8
)

// insSites counts the instrumented trees in dev mode: while it is above
// zero the node constructors capture their call stacks (instrument_site.go).
// Process-wide: node constructors know no tree.
var insSites atomic.Int32

// SetInstrument turns the DevTools instrumentation of the tree on or off
// (ADR-299). Turned on, the elements get IDs as they are listed or built,
// the next frames are profiled, the dev warnings are kept and, in dev
// mode, nodes made from then on record where they were made (a node made
// before shows no site until its parent builds again). Turned off, every
// element's state is dropped, pick mode ends and the highlight goes. The
// trees of child windows opened while it is on are instrumented too. Call
// it on the UI thread; turn it off before dropping a tree, since the node
// constructors of the process capture call stacks while any tree in dev
// mode is instrumented.
func (t *Tree) SetInstrument(on bool) {
	if on == (t.ins != nil) {
		return
	}
	if !on {
		ins := t.ins
		ins.pick = pickState{}
		t.ins = nil
		t.syncInspectOverlay()
		if ins.dev {
			insSites.Add(-1)
		}
		t.insLog.Store(nil)
		return
	}
	ins := &instrument{t: t, els: map[*Element]*elemIns{}, byID: map[ElementID]*Element{}, dev: t.dev}
	ins.prof.init(t)
	if ins.dev {
		insSites.Add(1)
	}
	t.ins = ins
	t.insLog.Store(newLogRing())
}

// Instrumented reports whether the instrumentation is on (SetInstrument).
func (t *Tree) Instrumented() bool { return t.ins != nil }

// of returns the state of el, giving it the next ID when it is new. An
// element that is no longer mounted (a box of one stays in the box tree
// until the next frame) or that is in another window's tree (ADR-180; its
// own tree's instrument has it, and forgets it when it unmounts) gets a
// state of its own with ID 0, not kept.
func (ins *instrument) of(el *Element) *elemIns {
	if e := ins.els[el]; e != nil {
		return e
	}
	if !el.mounted || el.tree != ins.t {
		return &elemIns{}
	}
	ins.t.insNext++
	e := &elemIns{id: ins.t.insNext}
	ins.els[el] = e
	ins.byID[e.id] = el
	return e
}

// idOf returns the ID of el (0 for nil).
func (ins *instrument) idOf(el *Element) ElementID {
	if el == nil {
		return 0
	}
	return ins.of(el).id
}

// element returns the mounted element of id, or nil.
func (ins *instrument) element(id ElementID) *Element {
	if el := ins.byID[id]; el != nil && el.mounted {
		return el
	}
	return nil
}

// forget drops the state of el, which unmounts.
func (ins *instrument) forget(el *Element) {
	if e := ins.els[el]; e != nil {
		delete(ins.els, el)
		delete(ins.byID, e.id)
	}
}

// frameNo is the number (Stats.Frames) of the frame a build now belongs
// to: the running frame, else the next one (a Mount builds before it).
func (ins *instrument) frameNo() int {
	if ins.prof.open {
		return ins.prof.rec.Frame
	}
	return ins.t.stats.Frames + 1
}

// ---------------------------------------------------------------------
// Why elements build.

// BuildCause is why a component built.
type BuildCause uint8

const (
	// CauseMount: its first build, as it mounted.
	CauseMount BuildCause = iota + 1
	// CauseReload: its first build after a tela dev reload, with the
	// state saved before it restored (ADR-113).
	CauseReload
	// CauseState: a UseState setter (Hook, HookKind, Site).
	CauseState
	// CauseReducer: a UseReducer dispatch (Hook, HookKind, Site).
	CauseReducer
	// CauseAsync: a UseAsync run ended, or started again (Hook, Site).
	CauseAsync
	// CauseContext: a context it reads got a new value (Context, and
	// Element the Provider).
	CauseContext
	// CauseValue: a Value its build read changed: a UseValue (or a hook
	// built on one: UseSpring, UseScroll, UseBounds...) of the component
	// Element, at Hook, or one of the tree's own (Context names it: the
	// color scheme, the theme, the window state...).
	CauseValue
	// CauseProps: its parent built it with props that are not equal to
	// the previous ones: Fields lists the fields that differ, Uncomparable
	// those that cannot be compared (slices, maps, funcs), which always
	// count as changed.
	CauseProps
	// CauseParent: its parent built and it cannot skip: a component of
	// tela.C, which never skips because of props.
	CauseParent
	// CauseDirection: the direction its UseDirection read changed.
	CauseDirection
	// CauseScheme: the color scheme its UseColorScheme or UseTheme read
	// changed (a Scheme prop above it).
	CauseScheme
	// CausePresence: its Presence child started or stopped exiting
	// (UsePresence).
	CausePresence
	// CauseVirtual: its VirtualList's visible range or measured sizes
	// changed.
	CauseVirtual
	// CauseUnknown: a build no recorded cause explains.
	CauseUnknown
)

var causeNames = [...]string{
	CauseMount: "mount", CauseReload: "reload", CauseState: "state", CauseReducer: "reducer",
	CauseAsync: "async", CauseContext: "context", CauseValue: "value", CauseProps: "props",
	CauseParent: "parent", CauseDirection: "direction", CauseScheme: "scheme",
	CausePresence: "presence", CauseVirtual: "virtual", CauseUnknown: "unknown",
}

// String names the cause: "mount", "state", "props"...
func (c BuildCause) String() string {
	if int(c) < len(causeNames) && causeNames[c] != "" {
		return causeNames[c]
	}
	return "unknown"
}

// MarshalText names the cause in JSON ("state"), as String does.
func (c BuildCause) MarshalText() ([]byte, error) { return []byte(c.String()), nil }

// UnmarshalText reads a name of MarshalText; a name it does not know is
// CauseUnknown.
func (c *BuildCause) UnmarshalText(b []byte) error {
	*c = CauseUnknown
	for i, n := range causeNames {
		if n != "" && n == string(b) {
			*c = BuildCause(i)
			break
		}
	}
	return nil
}

// cause is a recorded reason of a build.
type cause struct {
	kind     BuildCause
	hook     int32      // hook slot, -1 for none
	hookKind string     // its kind
	site     SourceSite // its call site
	el       ElementID  // the Value's owner, the Provider, in its own tree
	other    bool       // that tree is another window's
	what     string     // the context, the tree value, a detail
	// fields and noncmp are the props that differ and those that cannot
	// be compared.
	fields, noncmp []string
}

func (c *cause) same(d *cause) bool {
	return c.kind == d.kind && c.hook == d.hook && c.el == d.el && c.other == d.other && c.what == d.what
}

// note records cause c for the next build of el.
func (ins *instrument) note(el *Element, c cause) {
	e := ins.of(el)
	for i := range e.pend {
		if e.pend[i].same(&c) {
			return
		}
	}
	if len(e.pend) < maxPendingCauses {
		e.pend = append(e.pend, c)
	}
}

// insCauser is a hook slot whose updates build its component
// (Tree.ApplyUpdates): it says which cause they are.
type insCauser interface{ insCause() BuildCause }

func (s *stateSlot[T]) insCause() BuildCause      { return CauseState }
func (s *reducerSlot[S, A]) insCause() BuildCause { return CauseReducer }
func (s *asyncSlot[T]) insCause() BuildCause      { return CauseAsync }

// noteUpdate records the update of hook slot slot of el that marked it
// for building (ApplyUpdates).
func (ins *instrument) noteUpdate(el *Element, slot any) {
	kind := CauseState
	if c, ok := slot.(insCauser); ok {
		kind = c.insCause()
	}
	ins.noteHook(el, slot, kind)
}

// noteVirtual records that virtual list vl marked its component for
// building.
func (t *Tree) noteVirtual(vl *vlist) {
	if t.ins != nil {
		t.ins.noteHook(vl.el, vl, CauseVirtual)
	}
}

// noteHook records cause kind for el, from its hook slot slot.
func (ins *instrument) noteHook(el *Element, slot any, kind BuildCause) {
	c := cause{kind: kind, hook: -1}
	if i := hookIndex(el, slot); i >= 0 {
		c.hook = int32(i)
		c.hookKind = ins.hookKindName(el, i)
		c.site = ins.hookSite(el, i)
	}
	ins.note(el, c)
}

// hookIndex returns the index of slot among el's hook slots, or -1.
func hookIndex(el *Element, slot any) int {
	if slot == nil {
		return -1
	}
	for i, h := range el.hooks {
		if h == slot {
			return i
		}
	}
	return -1
}

// valueHolder is a hook slot that holds a Value: UseValue's own, and the
// hooks built on one (UseSpring, UseTween, UseBounds, UseSize,
// UseScroll).
type valueHolder interface{ holdsValue(v *valueCore) bool }

func (v *valueCore) holdsValue(c *valueCore) bool     { return v == c }
func (s *motionSlot[T]) holdsValue(c *valueCore) bool { return &s.v.valueCore == c }
func (f *boundsFollow) holdsValue(c *valueCore) bool  { return &f.v.valueCore == c }
func (f *sizeFollow) holdsValue(c *valueCore) bool    { return &f.v.valueCore == c }
func (f *scrollFollow) holdsValue(c *valueCore) bool {
	return &f.x.valueCore == c || &f.y.valueCore == c
}

// noteValue records that a change of v marked el, which read it in its
// build, for building (valueChangedSkip). The owner of v may be in another
// window's tree (Values are tracked app-wide, ADR-180): its ID is the one
// of its own tree, as noteContext gives the Provider's.
func (ins *instrument) noteValue(el *Element, v *valueCore) {
	c := cause{kind: CauseValue, hook: -1}
	if o := v.el; o != nil {
		c.other = o.tree != ins.t
		if o.kind == elemComponent {
			if oi := o.tree.ins; oi != nil {
				c.el = oi.idOf(o)
			}
			for i, h := range o.hooks {
				if vh, ok := h.(valueHolder); ok && vh.holdsValue(v) {
					c.hook = int32(i)
					c.hookKind = ins.hookKindName(o, i)
					c.site = ins.hookSite(o, i)
					break
				}
			}
		} else {
			c.what = treeValueName(o.tree, v)
		}
	}
	ins.note(el, c)
}

// treeValueName names a Value of the tree itself (owned by its root).
func treeValueName(t *Tree, v *valueCore) string {
	is := func(p *valueCore) bool { return p == v }
	e := &t.env
	switch {
	case e.dark != nil && is(&e.dark.valueCore):
		return "the color scheme"
	case e.reduced != nil && is(&e.reduced.valueCore):
		return "reduced motion"
	case e.contrast != nil && is(&e.contrast.valueCore):
		return "high contrast"
	case e.textScale != nil && is(&e.textScale.valueCore):
		return "the text scale"
	case e.accentV != nil && is(&e.accentV.valueCore):
		return "the accent color"
	case e.theme != nil && is(&e.theme.valueCore):
		return "the theme"
	case t.wstate != nil && is(&t.wstate.valueCore):
		return "the window state"
	}
	return "a value of the tree"
}

// noteContext records that Provider prov gave consumer el a new value. A
// consumer in a child window reads a Provider of the window that opened it
// (Tree.outer): the Provider's ID is the one of its own tree.
func (ins *instrument) noteContext(el, prov *Element) {
	c := cause{kind: CauseContext, hook: -1, what: contextName(prov.comp.ctx), other: prov.tree != ins.t}
	if prov.tree.ins != nil {
		c.el = prov.tree.ins.idOf(prov)
	}
	for i, h := range el.hooks {
		if s, ok := h.(interface{ ctxKey() any }); ok && s.ctxKey() == prov.comp.ctx {
			c.hook = int32(i)
			c.hookKind = ins.hookKindName(el, i)
			c.site = ins.hookSite(el, i)
			break
		}
	}
	ins.note(el, c)
}

func (s *ctxSlot[T]) ctxKey() any { return s.ctx }

// contextName names a context by its value type: "Context[app.Theme]".
func contextName(ctx any) string {
	rt := reflect.TypeOf(ctx)
	if rt == nil {
		return "Context"
	}
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	return goName(rt.Name())
}

// noteParent records why component el builds although its parent gave it
// a new node (updateComponent): old and nw are its old and new component
// descriptions. Equal props record nothing (the build has other causes).
func (ins *instrument) noteParent(el *Element, old, nw *compDesc) {
	switch {
	case nw.mode == 0 || old.mode != nw.mode:
		ins.note(el, cause{kind: CauseParent, hook: -1})
	case nw.mode == compMemo:
		if !nw.equal(old.props, nw.props) {
			ins.note(el, cause{kind: CauseProps, hook: -1, what: "Memo's equal reported a change"})
		}
	case nw.mode == compProps && safeEqual(old.props, nw.props),
		nw.mode == compStruct && componentEqual(old.props, nw.props):
	default:
		fields, noncmp := propsDiff(old.props, nw.props)
		ins.note(el, cause{kind: CauseProps, hook: -1, fields: fields, noncmp: noncmp})
	}
}

// propsDiff lists the fields of two struct props values that differ (as
// componentEqual compares them) and those that cannot be compared.
// Props that are not structs give neither.
func propsDiff(a, b any) (fields, noncmp []string) {
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	if !va.IsValid() || !vb.IsValid() || va.Type() != vb.Type() || va.Kind() != reflect.Struct {
		return nil, nil
	}
	rt := va.Type()
	for i := range rt.NumField() {
		f := rt.Field(i)
		if f.Name == "_" {
			continue
		}
		if eqKindOf(f.Type) == eqNever {
			noncmp = append(noncmp, f.Name)
			continue
		}
		if !valuesEqual(va.Field(i), vb.Field(i)) {
			fields = append(fields, f.Name)
		}
	}
	return fields, noncmp
}

// built records a build of el that starts now (Tree.build); first is its
// first build.
func (ins *instrument) built(el *Element, first bool) {
	e := ins.of(el)
	e.builds++
	var causes []cause
	if first {
		k := CauseMount
		if devStateEnabled && ins.t.devReloading() {
			k = CauseReload
		}
		causes = append(causes, cause{kind: k, hook: -1})
	}
	causes = append(causes, e.pend...)
	clear(e.pend)
	e.pend = e.pend[:0]
	if len(causes) == 0 {
		causes = append(causes, cause{kind: CauseUnknown, hook: -1})
	}
	e.hist[e.at] = buildRec{frame: ins.frameNo(), causes: causes}
	e.at = (e.at + 1) % maxBuildHistory
	e.n = min(e.n+1, maxBuildHistory)
	ins.prof.builtID(e.id)
}

// ---------------------------------------------------------------------
// Listing.

// SourceSite is a place in the source: the function and the file:line of
// a call.
type SourceSite struct {
	// Func is the function, without its package path ("main.Settings",
	// "kit.Button.Build").
	Func string `json:"func,omitempty"`
	// File is the file's full path, Line its line.
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
}

// String returns "settings.go:42", or "" for no site.
func (s SourceSite) String() string {
	if s.File == "" {
		return ""
	}
	i := strings.LastIndexAny(s.File, `/\`)
	return fmt.Sprintf("%s:%d", s.File[i+1:], s.Line)
}

// IsZero reports a missing site.
func (s SourceSite) IsZero() bool { return s.File == "" }

// ElementInfo describes one element of an instrumented tree (Tree.Elements,
// Tree.Element).
type ElementInfo struct {
	ID     ElementID `json:"id"`
	Parent ElementID `json:"parent,omitempty"`
	Depth  int       `json:"depth"`
	// Kind is the element as a Go user reads it: a primitive ("div",
	// "button", "Portal", "root"), "text", a component's type or function
	// without its package path ("main.Settings", "kit.Button",
	// "main.App.func1"), "Context[app.Theme].Provider" or "Presence".
	Kind      string `json:"kind"`
	Component bool   `json:"component,omitempty"`
	// Key is the key formatted like %v, "" without one.
	Key string `json:"key,omitempty"`
	// Text is a text element's text, shortened.
	Text string `json:"text,omitempty"`
	// Classes are a primitive's class strings, joined by spaces.
	Classes string `json:"classes,omitempty"`
	// Site is where the node of its last build or update was made (dev
	// mode; see instrument_site.go for the rule).
	Site SourceSite `json:"site,omitzero"`
	// Node is its node in the last semantics tree, 0 without one.
	Node semantics.NodeID `json:"node,omitempty"`
	// Bounds is the border box it owns as painted in window coordinates
	// after the last frame (InspectNode.Bounds, or Rect when that is
	// nil); for an inline element laid out in a paragraph (a span, a
	// link), the bounding rectangle of its fragments and content, as the
	// semantics tree bounds it; nil for an element without a box of its
	// own (a component, a text, display: contents) or with no area.
	Bounds *gfx.Rect `json:"bounds,omitempty"`
	// Builds counts its builds since the instrumentation turned on.
	Builds int `json:"builds,omitempty"`
	// The details, filled by Element and by Elements(true): the hooks of
	// a component, the props of a struct component (or of tela.CP and
	// tela.Memo, or a Provider's value) and its last builds, oldest first.
	Hooks  []HookInfo    `json:"hooks,omitempty"`
	Props  []PropInfo    `json:"props,omitempty"`
	Recent []BuildRecord `json:"recent,omitempty"`
}

// HookInfo describes a hook slot of a component.
type HookInfo struct {
	// Index is the slot's position (0 for the first hook).
	Index int `json:"index"`
	// Kind is the hook ("UseState", "UseEffect"...; the slot's type out
	// of dev mode: "UseState[int]").
	Kind string `json:"kind"`
	// Site is where the hook was first called (dev mode).
	Site SourceSite `json:"site,omitzero"`
	// Value is the slot's value, shortened like %v (the state, the
	// Value's current value, a Ref's Current, a memo, an effect's
	// dependencies...); "" for slots without one.
	Value string `json:"value,omitempty"`
}

// PropInfo is a prop of a component: a field of a struct component.
type PropInfo struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// BuildRecord is one build of a component and its causes.
type BuildRecord struct {
	// Frame is the frame it built for (Stats.Frames; FrameProfile.Frame).
	Frame   int           `json:"frame"`
	Reasons []BuildReason `json:"reasons"`
}

// BuildReason is a cause of a build.
type BuildReason struct {
	Cause BuildCause `json:"cause"`
	// Hook is the hook slot that caused it (CauseState, CauseReducer,
	// CauseAsync, CauseValue, CauseContext...), -1 for none; HookKind
	// names the hook and Site is where it is called (dev mode).
	Hook     int        `json:"hook"`
	HookKind string     `json:"hookKind,omitempty"`
	Site     SourceSite `json:"site,omitzero"`
	// Element is the component whose Value changed (CauseValue) or the
	// Provider (CauseContext), as an ID of the tree it is in: the tree of
	// the element that built, or, when OtherWindow is set, the tree of
	// another window of the app (ADR-180: a Value or a context reaches
	// across the windows), whose listing has it. 0 when that tree is not
	// instrumented.
	Element ElementID `json:"element,omitempty"`
	// OtherWindow reports that the Value's owner (CauseValue: Element, or
	// the tree whose Value Context names) or the Provider (CauseContext)
	// is in another window than the element that built.
	OtherWindow bool `json:"otherWindow,omitempty"`
	// Context is the context (CauseContext), the tree's value
	// (CauseValue without Element) or a detail of CauseProps.
	Context string `json:"context,omitempty"`
	// Fields and Uncomparable are the props fields that differ and those
	// that cannot be compared (CauseProps).
	Fields       []string `json:"fields,omitempty"`
	Uncomparable []string `json:"uncomparable,omitempty"`
}

// String describes the reason in a few words: "UseState #1 at
// counter.go:12 was set", "props Title, Children differ".
func (r BuildReason) String() string {
	hook := func() string {
		s := fmt.Sprintf("%s #%d", r.HookKind, r.Hook+1)
		if site := r.Site.Rel(); site != "" {
			s += " at " + site
		}
		return s
	}
	switch r.Cause {
	case CauseMount:
		return "mounted"
	case CauseReload:
		return "mounted by a tela dev reload"
	case CauseState:
		return hook() + " was set"
	case CauseReducer:
		return hook() + " got an action"
	case CauseAsync:
		return hook() + " changed"
	case CauseContext:
		if r.OtherWindow {
			return r.Context + " has a new value (its Provider is in another window)"
		}
		return r.Context + " has a new value"
	case CauseValue:
		var where string
		if r.OtherWindow {
			where = " (in another window)"
		}
		if r.Hook >= 0 {
			return "the " + hook() + " it reads changed" + where
		}
		return r.Context + " changed" + where
	case CauseProps:
		var parts []string
		if len(r.Fields) > 0 {
			parts = append(parts, strings.Join(r.Fields, ", ")+" differ")
		}
		if len(r.Uncomparable) > 0 {
			parts = append(parts, strings.Join(r.Uncomparable, ", ")+" cannot be compared")
		}
		if r.Context != "" {
			parts = append(parts, r.Context)
		}
		if len(parts) == 0 {
			return "new props not equal"
		}
		return "props: " + strings.Join(parts, "; ")
	case CauseParent:
		return "its parent built (tela.C never skips)"
	case CauseDirection:
		return "the direction of " + hook() + " changed"
	case CauseScheme:
		return "the color scheme of " + hook() + " changed"
	case CausePresence:
		return "its Presence child started or stopped exiting"
	case CauseVirtual:
		return "its visible range or measured sizes changed"
	}
	return "unknown"
}

// Elements returns the mounted elements of an instrumented tree in tree
// order (each after its parent), or nil when the instrumentation is off.
// detail fills Hooks, Props and Recent (formatting every hook's value:
// for a big tree, ask Element for the one shown instead). Call it on the
// UI thread, after a Frame (the bounds are the last frame's).
func (t *Tree) Elements(detail bool) []ElementInfo {
	ins := t.ins
	if ins == nil || t.root == nil {
		return nil
	}
	out := make([]ElementInfo, 0, max(len(ins.els), 64))
	var walk func(el *Element, parent ElementID, depth int)
	walk = func(el *Element, parent ElementID, depth int) {
		info := ins.info(el, parent, depth, detail)
		out = append(out, info)
		for _, c := range el.children {
			walk(c, info.ID, depth+1)
		}
	}
	walk(t.root, 0, 0)
	return out
}

// Element returns the element of id with its details (hooks, props, last
// builds); false when it is not mounted or the instrumentation is off.
func (t *Tree) Element(id ElementID) (ElementInfo, bool) {
	ins := t.ins
	if ins == nil {
		return ElementInfo{}, false
	}
	el := ins.element(id)
	if el == nil {
		return ElementInfo{}, false
	}
	var parent ElementID
	if el.parent != nil {
		parent = ins.idOf(el.parent)
	}
	depth := 0
	for p := el.parent; p != nil; p = p.parent {
		depth++
	}
	return ins.info(el, parent, depth, true), true
}

// ElementOfNode returns the element of semantics node id in the last
// semantics tree (Tree.Semantics), 0 when there is none or the
// instrumentation is off.
func (t *Tree) ElementOfNode(id semantics.NodeID) ElementID {
	if t.ins == nil {
		return 0
	}
	return t.ins.idOf(t.liveElement(id))
}

// NodeOfElement returns the node of element id in the last semantics tree,
// 0 when it has none (call Semantics first when the tree changed).
func (t *Tree) NodeOfElement(id ElementID) semantics.NodeID {
	if t.ins == nil {
		return 0
	}
	return t.semNodeOf(t.ins.element(id))
}

// semNodeOf returns el's node in the last semantics tree, or 0.
func (t *Tree) semNodeOf(el *Element) semantics.NodeID {
	if el == nil || el.semID == 0 || t.sem.byID[semantics.NodeID(el.semID)] != el {
		return 0
	}
	return semantics.NodeID(el.semID)
}

// InspectElementID is InspectElement for element id of an instrumented
// tree: the box it owns, or its nearest ancestor's.
func (t *Tree) InspectElementID(id ElementID) (InspectNode, error) {
	var el *Element
	if t.ins != nil {
		el = t.ins.element(id)
	}
	if el == nil {
		return InspectNode{}, fmt.Errorf("%w: element %d", ErrNodeGone, id)
	}
	return t.inspectOf(el, fmt.Sprintf("element %d", id))
}

// info describes el.
func (ins *instrument) info(el *Element, parent ElementID, depth int, detail bool) ElementInfo {
	t := ins.t
	e := ins.of(el)
	info := ElementInfo{ID: e.id, Parent: parent, Depth: depth, Builds: e.builds}
	info.Kind, info.Component = elementKind(el)
	if el.keyed {
		info.Key = keyString(el.key)
	}
	info.Site = ins.siteOf(el)
	switch el.kind {
	case elemText:
		info.Text = truncateRunes(el.text, 80)
	case elemPrimitive:
		info.Classes = strings.Join(el.node.d.classes, " ")
		if b := el.box; b != nil && b.cs != nil {
			switch {
			case t.ownBox(el, false) && t.inTree(b):
				r := t.boxWindowRect(b)
				info.Bounds = &r
			case b.inPara != nil && b.cs.Display != style.DisplayNone:
				if r := t.elemBounds(el); !r.Empty() {
					info.Bounds = &r
				}
			}
		}
	}
	info.Node = t.semNodeOf(el)
	if !detail {
		return info
	}
	if el.kind == elemComponent {
		info.Hooks = ins.hooksOf(el)
		info.Props = propsOf(el)
	}
	for k := range e.n {
		r := &e.hist[(e.at-e.n+k+maxBuildHistory)%maxBuildHistory]
		rec := BuildRecord{Frame: r.frame}
		for i := range r.causes {
			rec.Reasons = append(rec.Reasons, r.causes[i].reason())
		}
		info.Recent = append(info.Recent, rec)
	}
	return info
}

func (c *cause) reason() BuildReason {
	return BuildReason{Cause: c.kind, Hook: int(c.hook), HookKind: c.hookKind, Site: c.site, Element: c.el,
		OtherWindow: c.other, Context: c.what, Fields: c.fields, Uncomparable: c.noncmp}
}

// elementKind names el as ElementInfo.Kind does, and reports a component.
func elementKind(el *Element) (string, bool) {
	switch el.kind {
	case elemText:
		return "text", false
	case elemPrimitive:
		switch {
		case el.parent == nil:
			return "root", false
		case el.box.portal:
			return "Portal", false
		}
		return el.box.kind.String(), false
	}
	c := el.comp
	switch {
	case c.ctx != nil:
		return contextName(c.ctx) + ".Provider", true
	case c.presence:
		return "Presence", true
	case c.typ == virtualTyp:
		return "VirtualList", true
	}
	return goName(c.name()), true
}

// keyString formats a key like InspectNode.Key (%v, shortened), with the
// keys of a keyed fragment's children as "fragment/child".
func keyString(k any) string {
	if p, ok := k.(keyPair); ok {
		if p.outer == nil {
			return keyString(p.inner)
		}
		return keyString(p.outer) + "/" + keyString(p.inner)
	}
	return truncateRunes(fmt.Sprint(k), maxValueString)
}

// goName removes the package paths from a qualified name, keeping the
// package's own name: "github.com/kuronosu/tela/kit.Button" is
// "kit.Button", "app.List[example.com/x/y.Item]" is "app.List[y.Item]".
func goName(s string) string {
	var b strings.Builder
	start := 0
	for i := 0; i <= len(s); i++ {
		if i < len(s) && strings.IndexByte("[]*,() ", s[i]) < 0 {
			continue
		}
		tok := s[start:i]
		if j := strings.LastIndexByte(tok, '/'); j >= 0 {
			tok = tok[j+1:]
		}
		b.WriteString(tok)
		if i < len(s) {
			b.WriteByte(s[i])
		}
		start = i + 1
	}
	return b.String()
}

// hookKindName names hook slot i of el: its kind in dev mode, else the
// slot's type.
func (ins *instrument) hookKindName(el *Element, i int) string {
	if i < len(el.hookDev) {
		return el.hookDev[i].kind.String()
	}
	if el.hooks[i] == nil {
		return "hook"
	}
	return goName(slotName(el.hooks[i]))
}

// hookSite returns where hook slot i of el was first called (dev mode).
func (ins *instrument) hookSite(el *Element, i int) SourceSite {
	if i >= len(el.hookDev) {
		return SourceSite{}
	}
	s := el.hookDev[i].stack
	if site, ok := ins.hookSites[s]; ok {
		return site
	}
	var site SourceSite
	if f, ok := s.userFrame(); ok {
		site = SourceSite{Func: goName(f.Function), File: f.File, Line: f.Line}
	}
	if ins.hookSites == nil || len(ins.hookSites) >= maxSiteCache {
		ins.hookSites = map[callStack]SourceSite{}
	}
	ins.hookSites[s] = site
	return site
}

// hooksOf describes the hook slots of component el.
func (ins *instrument) hooksOf(el *Element) []HookInfo {
	var out []HookInfo
	for i, h := range el.hooks {
		hi := HookInfo{Index: i, Kind: ins.hookKindName(el, i), Site: ins.hookSite(el, i)}
		if v, ok := h.(insValuer); ok {
			hi.Value = v.insValue()
		}
		out = append(out, hi)
	}
	return out
}

// propsOf describes the props of component el: the fields of a struct
// component (or of the props of tela.CP and tela.Memo when they are a
// struct; "props" otherwise) and a Provider's value.
func propsOf(el *Element) []PropInfo {
	c := el.comp
	if el.prov != nil {
		return []PropInfo{{Name: "value", Value: shortValue(el.prov.value)}}
	}
	switch c.mode {
	case compStruct, compProps, compMemo:
	default:
		return nil
	}
	v := reflect.ValueOf(c.props)
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return []PropInfo{{Name: "props", Value: shortValue(c.props)}}
	}
	rt := v.Type()
	out := make([]PropInfo, 0, rt.NumField())
	for i := range rt.NumField() {
		f := rt.Field(i)
		if f.Name == "_" {
			continue
		}
		out = append(out, PropInfo{Name: f.Name, Value: shortRV(v.Field(i))})
	}
	return out
}

// ---------------------------------------------------------------------
// Hook values.

// insValuer is a hook slot whose value the instrumentation shows: what
// the dev state keeps of it (ADR-113: the state, a Value's current value,
// a Ref's Current, UseAsync's data), and the values of the other hooks
// that have one.
type insValuer interface{ insValue() string }

func (s *stateSlot[T]) insValue() string { return shortValue(s.value) }
func (v *Value[T]) insValue() string {
	if p := v.cur.Load(); p != nil {
		return shortValue(*p)
	}
	return ""
}
func (r *Ref[T]) insValue() string        { return shortValue(r.Current) }
func (s *memoSlot[T]) insValue() string   { return shortValue(s.value) }
func (s *onceSlot[T]) insValue() string   { return shortValue(s.value) }
func (s *idSlot) insValue() string        { return shortValue(s.id) }
func (s *motionSlot[T]) insValue() string { return s.v.insValue() }
func (s *ctxSlot[T]) insValue() string {
	if p := s.sub.prov; p != nil {
		return shortValue(p.prov.value)
	}
	return shortValue(s.ctx.def)
}
func (s *asyncSlot[T]) insValue() string {
	switch {
	case s.loading:
		return "loading"
	case s.err != nil:
		return "error: " + truncateRunes(s.err.Error(), maxValueLen)
	}
	return shortValue(s.data)
}
func (s *dirSlot) insValue() string {
	if s.rtl {
		return "rtl"
	}
	return "ltr"
}
func (s *effectSlot) insValue() string {
	if s.deps == nil {
		return ""
	}
	return "deps " + shortValue(s.deps)
}
