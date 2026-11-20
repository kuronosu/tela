package telatest

import (
	"strings"
	"time"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/semantics"
)

// DevTools data about the app (ADR-299): where each element was made, why
// each component rendered, what its hooks hold, and how long the frames
// took. It is off unless the app is mounted with [Instrument]: it costs
// a little per build and, in dev mode, a call stack per node made. The
// methods below return nothing (or the plain snapshot) without it.

// Instrument turns the DevTools instrumentation on before the root
// mounts, so every element built, the first builds included, has its
// creation site and its build records. [App.AriaSnapshotSources],
// [App.LayoutJSON], [App.Components], [App.ComponentOf] and [App.Frames]
// read it; the output of [App.AriaSnapshot] does not change. Creation
// sites need dev mode, which tests have unless [NoDev] turns it off.
// `tela inspect` and `tela mcp` mount their apps with it.
func Instrument() Option { return func(c *config) { c.instrument = true } }

// AriaSnapshotSources is [App.AriaSnapshotNodes] with the line that made
// each element: its key ends with " [src=file:line]", where the path is
// relative to the working directory when the file is inside it (the
// package's directory, for tests and for `tela inspect`), else the file's
// module path and its path in the module ("github.com/kuronosu/tela/kit/
// button.go:88"). An element a library component built is shown at the
// line that wrote the component, as that is the line to edit. Elements
// without a known site (the app is not mounted with [Instrument] or runs
// outside dev mode) keep their plain line. `tela inspect` prints this
// snapshot.
//
//   - button "Save" [src=settings.go:42]
//   - textbox "Email" [disabled] [src=settings.go:40]: ana@example.com
func (a *App) AriaSnapshotSources() (yaml string, nodes []uint64) {
	yaml, nodes = a.AriaSnapshotNodes()
	if !a.tree.Instrumented() {
		return yaml, nodes
	}
	lines := strings.Split(yaml, "\n")
	if len(lines) != len(nodes) {
		return yaml, nodes
	}
	for i, id := range nodes {
		if id == 0 {
			continue
		}
		src := a.tree.AppSite(a.tree.ElementOfNode(semantics.NodeID(id))).Rel()
		if src == "" {
			continue
		}
		if l, ok := semantics.ParseSnapshotLine(lines[i]); ok {
			l.Src = src
			lines[i] = l.WithSuffix("")
		}
	}
	return strings.Join(lines, "\n"), nodes
}

// Component is a component of an app mounted with [Instrument]: what it
// was given, what its hooks hold and why it rendered.
type Component struct {
	// Name is the component as a Go user reads it: "main.Settings",
	// "kit.Button", "Context[app.Theme].Provider".
	Name string
	// Key is its key formatted like %v, "" without one.
	Key string
	// Source is where the app made it, in the form of
	// [App.AriaSnapshotSources] ("" outside dev mode).
	Source string
	// Props are the fields of a struct component, in shortened form.
	Props []Prop
	// Hooks are its hook slots in call order.
	Hooks []Hook
	// Builds counts its builds since the app mounted.
	Builds int
	// Renders are its last builds (at most 8), oldest first.
	Renders []Rendering
}

// Prop is a prop of a [Component].
type Prop struct{ Name, Value string }

// Hook is a hook slot of a [Component].
type Hook struct {
	// Index is its position, 0 for the first hook called.
	Index int
	// Kind is the hook: "UseState", "UseMemo", "UseEffect"...
	Kind string
	// Source is where it was called (dev mode), like [Component.Source].
	Source string
	// Value is what it holds, shortened like %v: the state, a memo's
	// value, an effect's dependencies; "" for hooks without one.
	Value string
}

// Rendering is one build of a [Component] and the reasons for it.
type Rendering struct {
	// Frame is the frame number it built for ([Frame.Number]).
	Frame int
	// Reasons say why it built, each a sentence: "mounted", "UseState #1
	// at counter.go:12 was set", "props Title differ", "its parent built
	// (tela.C never skips)".
	Reasons []string
}

func componentOf(a *App, e core.ElementInfo) Component {
	c := Component{Name: e.Kind, Key: e.Key, Source: a.tree.AppSite(e.ID).Rel(), Builds: e.Builds}
	for _, p := range e.Props {
		c.Props = append(c.Props, Prop{p.Name, p.Value})
	}
	for _, h := range e.Hooks {
		c.Hooks = append(c.Hooks, Hook{Index: h.Index, Kind: h.Kind, Source: h.Site.Rel(), Value: h.Value})
	}
	for _, r := range e.Recent {
		rr := Rendering{Frame: r.Frame}
		for _, why := range r.Reasons {
			rr.Reasons = append(rr.Reasons, why.String())
		}
		c.Renders = append(c.Renders, rr)
	}
	return c
}

// Components returns the components of the app in tree order, each with
// its props, hooks and last builds. It returns nil unless the app was
// mounted with [Instrument]. Hook values are formatted for every
// component: for a large tree ask [App.ComponentOf] for the one that
// matters.
func (a *App) Components() []Component {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleQuery()
	var out []Component
	for _, e := range a.tree.Elements(true) {
		if e.Component {
			out = append(out, componentOf(a, e))
		}
	}
	return out
}

// ComponentOf returns the component that rendered the element of node id
// (the identity [App.AriaSnapshotNodes] gives a snapshot line): the
// nearest component at or above it. ok is false when the app was not
// mounted with [Instrument] or the element is no longer mounted.
func (a *App) ComponentOf(id uint64) (c Component, ok bool) {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleQuery()
	if !a.tree.Instrumented() {
		return Component{}, false
	}
	a.tree.Semantics() // the node identities of the latest frame
	el := a.tree.ElementOfNode(semantics.NodeID(id))
	if el == 0 {
		return Component{}, false
	}
	parent := map[core.ElementID]core.ElementID{}
	comp := map[core.ElementID]bool{}
	for _, e := range a.tree.Elements(false) {
		parent[e.ID], comp[e.ID] = e.Parent, e.Component
	}
	for ; el != 0; el = parent[el] {
		if comp[el] {
			info, found := a.tree.Element(el)
			if !found {
				return Component{}, false
			}
			return componentOf(a, info), true
		}
	}
	return Component{}, false
}

// Frame is the profile of a frame of an app mounted with [Instrument]:
// the time of each phase of the pipeline and what it did.
type Frame struct {
	// Number is the frame's number ([Stats.Frames] after it).
	Number int
	// Duration is the time of the UI thread on the frame, the input that
	// came before it excluded.
	Duration time.Duration
	// Phases are the wall-clock time of each phase that took any: "input",
	// "animate", "build", "style", "layout", "paint", "semantics",
	// "raster", "present" and "effects".
	Phases map[string]time.Duration
	// Built counts the component builds, Styled the style computations
	// and LaidOut the boxes laid out in the frame.
	Built, Styled, LaidOut int
	// Damage is the area repainted, in device pixels.
	Damage int64
}

// Frames returns the profile of the frames whose number is above since (0
// for all), oldest first; the app keeps the last 600. The times come from
// the clock of [DebugClock], which reads 1 ms longer at every call unless
// the test gives a real one (a function of time.Since), so by default they
// count clock readings, not time: assert on counts ([Frame.Built],
// [Frame.LaidOut]) and pass a real clock to measure. It returns nil unless
// the app was mounted with [Instrument].
func (a *App) Frames(since int) []Frame {
	a.tb.Helper()
	a.mustBeOpen()
	var out []Frame
	for _, p := range a.tree.FrameProfiles(since) {
		f := Frame{Number: p.Frame, Duration: p.Duration, Phases: map[string]time.Duration{},
			Built: p.Built, Styled: p.Styled, LaidOut: p.LaidOut, Damage: p.Damage}
		for ph, d := range p.Phases {
			if d > 0 {
				f.Phases[core.Phase(ph).String()] = d
			}
		}
		out = append(out, f)
	}
	return out
}
