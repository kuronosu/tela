//go:build teladev

package core

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/kuronosu/tela/internal/text/edit"
)

// The state of a running tree that `tela dev` keeps across a reload
// (ARCHITECTURE section 17, ADR-111 and ADR-113). The dev loop runs the
// app as child process N; on a save it builds N+1, asks N for a snapshot
// of its state (CaptureDevState), starts N+1 with it (SetDevSeed) and N+1
// mounts with that state, as React Fast Refresh keeps state when a module
// is edited. This file and the other devstate_*.go files compile only with
// the teladev build tag; without it devstate_off.go makes devStateEnabled
// false, so the hook points guarded by it are removed and release builds
// do not change.
//
// # What is kept
//
// Hook slots: the values of UseState (and UseStateSet, UseStateUpdate),
// UseReducer, UseValue and UseRef, the hooks of Tela's own packages built
// on them included (the open or selected state of package aria's hooks).
// A slot is identified by the path of Elements from the root to its
// component (devStep: each component's identity, a primitive's tag, and
// a key or a position among the siblings), its index in the component and
// its signature entry (the hook's kind and the value's full type). A
// component keeps its slots only when its whole hook signature (the kind
// and type of every slot, effects and memos included) is the saved one:
// otherwise it and its subtree mount fresh (devFirstBuild; Fast Refresh's
// rule).
//
// Values are encoded with encoding/gob, one slot at a time, after a check
// by reflection that gob can encode them without dropping anything
// (devKeepOf); a slot that cannot be kept resets, and dev mode warns once
// per slot. UseSpring and UseTween start at rest at their target, a Value
// that Animate moves is kept at its target, and effects run again.
//
// Engine state: the text, selection and undo history of text fields, the
// scroll offsets of scroll containers (the root's too), the focused
// element and whether it showed as focus-visible, and the color scheme
// and theme set at run time with SetColorScheme and SetTheme. Exiting
// Presence children are dropped. The window's size and scale come from
// the platform.

// devStateVersion is the version of the snapshot format: a snapshot of
// another version starts the app fresh, with a warning.
const devStateVersion = 1

// devMagic starts every snapshot.
var devMagic = []byte("tela-devstate\x00")

const devStateEnabled = true

// devSeedFrames is how many frames after the first mount the saved state
// still seeds the components and elements that mount (the items a
// VirtualList builds for a restored offset, components shown after a
// measurement), unless everything saved was found before.
const devSeedFrames = 4

// ---------------------------------------------------------------------
// The format.

// devSnapshot is a tree's state, gob-encoded after devMagic and the
// version byte.
type devSnapshot struct {
	// Idents holds the strings that nodes refer to by index: component
	// identities and signature entries.
	Idents []string
	// Root is the root primitive's node (the viewport's scroll offset) with
	// the app below it.
	Root devNode
	// Scheme is the color scheme set with SetColorScheme (HasScheme).
	Scheme    uint8
	HasScheme bool
	// Theme is the theme set with SetTheme, nil for none.
	Theme *Theme
	// FocusType and FocusKey are the focus-visible heuristic's state
	// (focusState.lastType, hadKey).
	FocusType uint8
	FocusKey  bool
	// Warned are the keys of the warnings about slots that cannot be kept,
	// given in this dev session: each is given once, not on every reload.
	Warned []string
	// Extra holds the state saved by RegisterDevState's functions, by name.
	Extra map[string][]byte
}

// devNode is the saved state of an Element and its subtree. Only Elements
// with state in their subtree are saved.
type devNode struct {
	// Ident and ID are the Element's identity among its siblings
	// (devStep): Ident indexes devSnapshot.Idents ("F:pkg.Counter",
	// "P:div"), ID is its key or its position.
	Ident uint32
	ID    string
	// Comp marks a component; Sig is its hook signature (indexes into
	// Idents), Slots the slots it keeps.
	Comp  bool
	Sig   []uint32
	Slots []devSlotRec
	// Engine state of a primitive: a text field's editor and, for an
	// Input, its own horizontal scroll; a scroll container's offset; the
	// focus.
	Field  *edit.DevState
	FieldX float32
	Scroll *devPoint
	Focus  bool
	Kids   []devNode
}

// devSlotRec is a kept hook slot: its index and its encoded value
// (devEncodeValue).
type devSlotRec struct {
	Index int
	Data  []byte
}

type devPoint struct{ X, Y float32 }

func encodeDevSnapshot(s *devSnapshot) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write(devMagic)
	buf.WriteByte(devStateVersion)
	if err := devGobEncode(&buf, s); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decodeDevSnapshot reads a snapshot, reporting a foreign, damaged or
// other-version one as an error (never a panic).
func decodeDevSnapshot(b []byte) (*devSnapshot, error) {
	rest, ok := bytes.CutPrefix(b, devMagic)
	if !ok {
		return nil, errors.New("not a tela dev state")
	}
	if len(rest) == 0 {
		return nil, errors.New("the state is truncated")
	}
	if v := rest[0]; v != devStateVersion {
		return nil, fmt.Errorf("it has format version %d and this build reads version %d", v, devStateVersion)
	}
	s := new(devSnapshot)
	if err := devGobDecode(rest[1:], s); err != nil {
		return nil, fmt.Errorf("the state is damaged: %v", err)
	}
	if err := s.check(); err != nil {
		return nil, fmt.Errorf("the state is damaged: %v", err)
	}
	return s, nil
}

// check validates the indexes of s. Every node but the root has an
// identity (the root's is never read).
func (s *devSnapshot) check() error {
	n := uint32(len(s.Idents))
	var walk func(d *devNode) error
	walk = func(d *devNode) error {
		if d != &s.Root && d.Ident >= n {
			return fmt.Errorf("identity %d of %d", d.Ident, n)
		}
		for _, x := range d.Sig {
			if x >= n {
				return fmt.Errorf("signature entry %d of %d", x, n)
			}
		}
		for i := range d.Kids {
			if err := walk(&d.Kids[i]); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(&s.Root)
}

// ---------------------------------------------------------------------
// The API (for the dev loop's child runtime, T3 of phase 9).

// DevStateReport describes a capture: what was kept and what resets.
type DevStateReport struct {
	// Components is the number of component Elements visited.
	Components int
	// Kept is the number of hook slots kept.
	Kept int
	// Reset lists the slots that cannot be kept, with the reason.
	Reset []DevSlotReset
	// Fields, Scrolls and Focus count the engine state kept: text fields,
	// scroll offsets and the focused element.
	Fields, Scrolls int
	Focus           bool
	// Extra names the state of RegisterDevState kept, ExtraErrors the
	// functions that failed ("name: error").
	Extra, ExtraErrors []string
	// Bytes is the size of the snapshot.
	Bytes int
}

// DevSlotReset is a hook slot that resets on reload.
type DevSlotReset struct {
	// Component names the component ("Counter", "a function literal in
	// app.Main"), Hook the hook ("UseState") and Index its position in the
	// component (1 for the first hook).
	Component, Hook string
	Index           int
	// Type is the slot's value type and Reason why it cannot be kept
	// ("holds a func()").
	Type, Reason string
	// Silent marks a slot that resets without a warning: Tela's own state
	// (handles such as Setter, Callback or ElementRef, which the new build
	// makes again, and the internal state of Tela's hooks).
	Silent bool
}

// String summarizes the report in one line.
func (r DevStateReport) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "kept %d hook slot%s", r.Kept, plural(r.Kept))
	var parts []string
	if r.Fields > 0 {
		parts = append(parts, fmt.Sprintf("%d text field%s", r.Fields, plural(r.Fields)))
	}
	if r.Scrolls > 0 {
		parts = append(parts, fmt.Sprintf("%d scroll offset%s", r.Scrolls, plural(r.Scrolls)))
	}
	if r.Focus {
		parts = append(parts, "the focus")
	}
	for _, p := range parts {
		b.WriteString(", ")
		b.WriteString(p)
	}
	fmt.Fprintf(&b, " (%d bytes)", r.Bytes)
	if n := len(r.Reset); n > 0 {
		fmt.Fprintf(&b, "; %d slot%s reset", n, plural(n))
		for i, s := range r.Reset {
			if i == 3 {
				b.WriteString(", ...")
				break
			}
			sep := ": "
			if i > 0 {
				sep = ", "
			}
			fmt.Fprintf(&b, "%s%s #%d of %s %s", sep, s.Hook, s.Index, s.Component, s.Reason)
		}
	}
	return b.String()
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// DevSeedReport describes how a tree used a snapshot (Tree.DevSeedReport).
type DevSeedReport struct {
	// Err is why the snapshot could not be read; the tree started fresh.
	Err string
	// Restored is the number of hook slots restored.
	Restored int
	// Remounted lists the components that mounted fresh, with their
	// subtrees, because their hook signature changed ("Counter: hook 2 was
	// UseState[int], is UseState[string]").
	Remounted []string
	// Fields, Scrolls and Focus count the engine state restored.
	Fields, Scrolls int
	Focus           bool
	// Unused is the number of saved components with state that no element
	// took within the seed frames (removed, moved, or of another type).
	Unused int
}

// CaptureDevState returns a snapshot of the tree's state for `tela dev`
// to start the next build with (SetDevSeed, SeedDevState), and what it
// kept. Call it on the UI thread, between frames; updates still queued
// are not included. A slot that cannot be kept is reported and, in dev
// mode, warned about once per slot in the dev session ("Counter's
// UseState #2 holds a func(); it resets on reload"). The error is
// encoding/gob's, which the checks make unlikely.
func (t *Tree) CaptureDevState() ([]byte, DevStateReport, error) {
	dt := t.devStateMake()
	c := &devCapture{t: t, dt: dt, idents: map[string]uint32{}}
	var snap devSnapshot
	if t.root != nil {
		if n := c.node(t.root); n != nil {
			snap.Root = *n
		}
	}
	snap.Idents = c.table
	snap.Scheme, snap.HasScheme = uint8(dt.scheme), dt.schemeSet
	snap.Theme = dt.theme
	snap.FocusType, snap.FocusKey = uint8(t.foc.lastType), t.foc.hadKey
	snap.Warned = dt.warned
	snap.Extra = c.extras()
	data, err := encodeDevSnapshot(&snap)
	c.rep.Bytes = len(data)
	if err != nil {
		return nil, c.rep, err
	}
	return data, c.rep, nil
}

// CaptureAppDevState captures the state of the running app (the Tree that
// receives Post) on its UI thread, after the updates queued before the
// call, and calls done there with the result. Safe from any goroutine; it
// waits for an app that receives posts, like Post.
func CaptureAppDevState(done func(data []byte, rep DevStateReport, err error)) {
	if done == nil {
		panic("core: CaptureAppDevState(nil): pass the function that receives the state")
	}
	postUpdate(update{postTree: func(t *Tree) {
		data, rep, err := t.CaptureDevState()
		done(data, rep, err)
	}})
}

// devPending is the snapshot given to SetDevSeed, for the next NewHost.
var devPending struct {
	sync.Mutex
	data []byte
	ok   bool
}

// SetDevSeed gives the next window's tree (NewHost, and so Run) the
// snapshot data to mount with: one app per process, so the dev runtime
// calls it before Run. A nil or empty snapshot seeds nothing. Headless
// trees use Tree.SeedDevState.
func SetDevSeed(data []byte) {
	devPending.Lock()
	devPending.data, devPending.ok = data, true
	devPending.Unlock()
}

// devAdoptSeed seeds t, made by NewHost, with the snapshot of SetDevSeed.
func (t *Tree) devAdoptSeed() {
	devPending.Lock()
	data, ok := devPending.data, devPending.ok
	devPending.data, devPending.ok = nil, false
	devPending.Unlock()
	if ok {
		t.SeedDevState(data)
	}
}

// SeedDevState makes the tree mount with the state of a snapshot from
// CaptureDevState. Call it before Mount, on the UI thread. The state
// seeds the hook slots as the first builds create them and the engine
// state as the elements mount, during Mount and the first devSeedFrames
// frames; after that the snapshot is dropped. A snapshot that cannot be
// read (another format version, damaged data) starts the tree fresh with
// one warning; it never panics. A nil or empty snapshot seeds nothing.
func (t *Tree) SeedDevState(data []byte) {
	dt := t.devStateMake()
	if t.root != nil {
		t.warnf("tela: SeedDevState after Mount is ignored; seed a tree before it mounts")
		return
	}
	dt.seed, dt.seedRep = nil, DevSeedReport{}
	if len(data) == 0 {
		return
	}
	snap, err := decodeDevSnapshot(data)
	if err != nil {
		dt.seedRep.Err = err.Error()
		t.warnf("tela: the state saved by the previous build cannot be restored (%v); the app starts fresh", err)
		return
	}
	dt.warned = snap.Warned
	for _, k := range snap.Warned {
		t.once(k)
	}
	if snap.HasScheme && ColorScheme(snap.Scheme) <= ColorSchemeDark {
		t.SetColorScheme(ColorScheme(snap.Scheme))
	}
	if snap.Theme != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.warnf("tela: the theme saved by the previous build cannot be restored (%v); the app keeps its theme", r)
				}
			}()
			t.SetTheme(*snap.Theme)
		}()
	}
	t.restoreExtras(snap.Extra)
	dt.seed = newDevSeed(t, snap)
}

// ---------------------------------------------------------------------
// State outside the tree.

// devExtras are the functions given to RegisterDevState, by name.
var devExtras struct {
	sync.Mutex
	m map[string]devExtra
}

type devExtra struct {
	save    func() ([]byte, error)
	restore func([]byte) error
}

// RegisterDevState registers state that lives outside the tree, to keep
// across reloads in the same snapshot: save runs in CaptureDevState, on
// the UI thread, and restore in SeedDevState, before the tree mounts, with
// what save returned in the previous build. For the dev runtime (the app's
// App.DevState) and for Tela's packages with process-wide state (package
// i18n's locale), from files built only with the teladev tag. A later
// registration of a name replaces the earlier one; nil functions remove
// it. An error or a panic of either function is a warning: that state
// starts fresh.
func RegisterDevState(name string, save func() ([]byte, error), restore func([]byte) error) {
	devExtras.Lock()
	defer devExtras.Unlock()
	if save == nil || restore == nil {
		delete(devExtras.m, name)
		return
	}
	if devExtras.m == nil {
		devExtras.m = map[string]devExtra{}
	}
	devExtras.m[name] = devExtra{save, restore}
}

// devExtrasNow returns a copy of the registered functions.
func devExtrasNow() map[string]devExtra {
	devExtras.Lock()
	defer devExtras.Unlock()
	m := make(map[string]devExtra, len(devExtras.m))
	for k, v := range devExtras.m {
		m[k] = v
	}
	return m
}

// extras runs the registered save functions.
func (c *devCapture) extras() map[string][]byte {
	var out map[string][]byte
	for name, x := range devExtrasNow() {
		data, err := devCall(func() ([]byte, error) { return x.save() })
		if err != nil {
			c.rep.ExtraErrors = append(c.rep.ExtraErrors, name+": "+err.Error())
			c.t.warnf("tela: the dev state %q cannot be saved (%v); it starts fresh on reload", name, err)
			continue
		}
		if out == nil {
			out = map[string][]byte{}
		}
		out[name] = data
		c.rep.Extra = append(c.rep.Extra, name)
	}
	slices.Sort(c.rep.Extra)
	return out
}

// restoreExtras runs the registered restore functions with the saved
// state (names that nothing registered are ignored).
func (t *Tree) restoreExtras(saved map[string][]byte) {
	if len(saved) == 0 {
		return
	}
	for name, x := range devExtrasNow() {
		data, ok := saved[name]
		if !ok {
			continue
		}
		if _, err := devCall(func() ([]byte, error) { return nil, x.restore(data) }); err != nil {
			t.warnf("tela: the dev state %q cannot be restored (%v); it starts fresh", name, err)
		}
	}
}

// devCall calls f, turning a panic into an error.
func devCall(f func() ([]byte, error)) (data []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return f()
}

// DevSeedReport reports how the tree used the snapshot of SeedDevState
// (complete once the seed frames are over).
func (t *Tree) DevSeedReport() DevSeedReport {
	dt := t.devState()
	if dt == nil {
		return DevSeedReport{}
	}
	if dt.seed != nil {
		r := dt.seedRep
		r.Unused = dt.seed.unused()
		return r
	}
	return dt.seedRep
}

// ---------------------------------------------------------------------
// The dev state of a Tree.

// devTreeState is the dev-loop state of a Tree, kept in its locals (TreeLocal)
// so that the Tree has no field for it without the tag.
type devTreeState struct {
	// scheme and theme are what SetColorScheme and SetTheme set last.
	scheme    ColorScheme
	schemeSet bool
	theme     *Theme
	// warned are the keys of the warnings about slots that cannot be kept
	// given in this dev session (this process and the ones before).
	warned []string
	// seed is the snapshot being restored, nil when none (or over), and
	// seedRep what it restored.
	seed    *devRestore
	seedRep DevSeedReport
}

type devKey struct{}

func (t *Tree) devState() *devTreeState {
	d, _ := t.locals[devKey{}].(*devTreeState)
	return d
}

func (t *Tree) devStateMake() *devTreeState {
	if d := t.devState(); d != nil {
		return d
	}
	d := &devTreeState{}
	if t.locals == nil {
		t.locals = map[any]any{}
	}
	t.locals[devKey{}] = d
	return d
}

// devNoteScheme records a color scheme set at run time.
func (t *Tree) devNoteScheme(s ColorScheme) {
	d := t.devStateMake()
	d.scheme, d.schemeSet = s, true
}

// devNoteTheme records a theme set at run time.
func (t *Tree) devNoteTheme(th Theme) {
	th.Colors = cloneColors(th.Colors)
	t.devStateMake().theme = &th
}

func cloneColors(m map[string]SchemeColor) map[string]SchemeColor {
	if m == nil {
		return nil
	}
	out := make(map[string]SchemeColor, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// ---------------------------------------------------------------------
// Capture.

// devCapture is one capture of a tree.
type devCapture struct {
	t      *Tree
	dt     *devTreeState
	rep    DevStateReport
	idents map[string]uint32
	table  []string
}

// intern returns the index of s in the snapshot's string table.
func (c *devCapture) intern(s string) uint32 {
	if i, ok := c.idents[s]; ok {
		return i
	}
	i := uint32(len(c.table))
	c.table = append(c.table, s)
	c.idents[s] = i
	return i
}

// node returns the saved node of el, or nil when nothing in its subtree
// has state to keep.
func (c *devCapture) node(el *Element) *devNode {
	var n devNode
	for _, ch := range el.children {
		if ch.kind == elemText || ch.exiting() {
			continue // exiting Presence children are not kept
		}
		if k := c.node(ch); k != nil {
			n.Kids = append(n.Kids, *k)
		}
	}
	has := len(n.Kids) > 0
	switch el.kind {
	case elemPrimitive:
		if c.primitive(el, &n) {
			has = true
		}
	case elemComponent:
		if el.prov == nil && !el.comp.presence {
			c.rep.Components++
			if c.component(el, &n) {
				has = true
			}
			if has {
				n.Comp = true
				n.Sig = make([]uint32, len(el.hooks))
				for j := range el.hooks {
					n.Sig[j] = c.intern(devSlotSig(el, j))
				}
			}
		}
	}
	if !has {
		return nil
	}
	if el.parent != nil {
		ident, id := devStepParts(el)
		n.Ident, n.ID = c.intern(ident), id
	}
	return &n
}

// component records the slots of component el that it keeps, reporting
// whether it has any (or motion slots, which a reload starts at their
// target).
func (c *devCapture) component(el *Element, n *devNode) bool {
	any := false
	for j, h := range el.hooks {
		if _, ok := h.(devMotionSlot); ok {
			any = true
			continue
		}
		k, ok := h.(devKept)
		if !ok {
			continue
		}
		cr := devCreatorOf(el, j)
		if cr.transient {
			c.reset(el, j, "follows the pointer, which the next build reads again", true)
			continue
		}
		data, why := k.devEncode()
		if why.reason != "" {
			c.reset(el, j, why.reason, why.silent || cr.framework)
			continue
		}
		n.Slots = append(n.Slots, devSlotRec{Index: j, Data: data})
		c.rep.Kept++
		any = true
	}
	return any
}

// reset records slot j of el as not kept and warns once per slot in the
// dev session, unless silent.
func (c *devCapture) reset(el *Element, j int, reason string, silent bool) {
	kind := devSlotKind(el, j)
	r := DevSlotReset{
		Component: devCompName(el.comp), Hook: kind.String(), Index: j + 1,
		Type: devSlotTypeName(el.hooks[j]), Reason: reason, Silent: silent,
	}
	c.rep.Reset = append(c.rep.Reset, r)
	t := c.t
	if silent || !t.dev {
		return
	}
	key := "devstate:" + devPath(el) + "#" + fmt.Sprint(j)
	if !t.once(key) {
		return
	}
	c.dt.warned = append(c.dt.warned, key)
	where := ""
	if j < len(el.hookDev) {
		where = " (called at " + el.hookSite(j) + ")"
	}
	t.warnf("tela: %s %s; it resets on reload%s. tela dev keeps what encoding/gob encodes in full: booleans, numbers, strings, and slices, arrays, maps, pointers and structs of exported fields of those, and types with a GobEncode or MarshalBinary method",
		devSlotTitle(el, j, kind), reason, where)
}

// devSlotTitle names slot j of el for a warning: "Counter's UseState #2",
// "UseState #2 of a function literal in app.Main".
func devSlotTitle(el *Element, j int, kind hookKind) string {
	name := devCompName(el.comp)
	if strings.HasPrefix(name, "a ") {
		return fmt.Sprintf("%s #%d of %s", kind, j+1, name)
	}
	return fmt.Sprintf("%s's %s #%d", name, kind, j+1)
}

// primitive records the engine state of primitive el: a text field's
// editor, a scroll container's offset (where it is going, when it
// animates), the focus.
func (c *devCapture) primitive(el *Element, n *devNode) bool {
	bx := el.box
	has := false
	if f := bx.fld; f != nil && !f.pristine() {
		st := f.ed.DevState()
		n.Field = &st
		if !f.multi {
			n.FieldX = f.scrollX
		}
		c.rep.Fields++
		has = true
	}
	if s := bx.scr; s != nil {
		if p := s.target(); p.X != 0 || p.Y != 0 {
			n.Scroll = &devPoint{p.X, p.Y}
			c.rep.Scrolls++
			has = true
		}
	}
	if c.t.foc.el == el {
		n.Focus = true
		c.rep.Focus = true
		has = true
	}
	return has
}

// pristine reports whether the field shows what mounting it shows: its
// initial value, the caret at the start, no history.
func (f *field) pristine() bool {
	if f.ed.CanUndo() || f.ed.CanRedo() {
		return false
	}
	if s, e, _ := f.ed.SelectionRange(); s != 0 || e != 0 {
		return false
	}
	fp := f.props()
	switch {
	case fp.hasValue:
		return f.ed.Value() == fp.value
	case fp.bind != nil:
		return true // the bound Value holds the text
	}
	return f.ed.Value() == fp.defValue
}
