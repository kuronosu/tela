package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/text"
)

// dirReads records what UseDirection returned to each named reader, one
// entry per build.
type dirReads map[string][]string

// reader returns a component that reads the direction and records it
// under name. It takes props (0) so that a parent's build skips it.
func (r dirReads) reader(name string, kids ...Child) func(int) Node {
	return func(int) Node {
		r[name] = append(r[name], UseDirection())
		return div(kids...)
	}
}

// expect checks how many times name built and what its last build read.
func (r dirReads) expect(t *testing.T, when, name string, builds int, dir string) {
	t.Helper()
	got := r[name]
	if len(got) != builds {
		t.Errorf("%s: %s built %d times (%v), want %d", when, name, len(got), got, builds)
	}
	if len(got) > 0 && got[len(got)-1] != dir {
		t.Errorf("%s: %s read %q, want %q", when, name, got[len(got)-1], dir)
	}
}

// TestUseDirectionNesting: the direction at a component's position is the
// Dir of its nearest primitive ancestor that has one (ltr without one),
// through components without a box, and Portal content reads the
// direction where the Portal is declared.
func TestUseDirectionNesting(t *testing.T) {
	r := dirReads{}
	app := func() Node {
		return div(
			ComponentProps(r.reader("root"), 0),
			div(Dir("RTL"),
				div(ComponentProps(r.reader("rtl"), 0)),
				Component(func() Node { return ComponentProps(r.reader("through"), 0) }),
				div(Dir("ltr"), ComponentProps(r.reader("nested"), 0)),
				div(Dir(""), ComponentProps(r.reader("empty"), 0)),
				Portal(ComponentProps(r.reader("portal"), 0)),
			),
		)
	}
	tr := newTestTree(t, 200, 200, Component(app), nil)
	tr.Frame()
	r.expect(t, "mount", "root", 1, "ltr")
	r.expect(t, "mount", "rtl", 1, "rtl")
	r.expect(t, "mount", "through", 1, "rtl")
	r.expect(t, "mount", "nested", 1, "ltr")
	r.expect(t, "mount", "empty", 1, "rtl")
	r.expect(t, "mount", "portal", 1, "rtl")
}

// TestUseDirectionRebuildsOnlyReaders: a change of an ancestor's Dir
// builds again the readers below it that read another direction, and
// nothing else: not the readers outside it, not those below a Dir of
// their own, not a component that does not read the direction, not a
// reader whose direction a new Dir leaves as it was.
func TestUseDirectionRebuildsOnlyReaders(t *testing.T) {
	r := dirReads{}
	others := 0
	other := func(int) Node { others++; return div() }
	var setDir Setter[string]
	app := func() Node {
		d, set := UseState("")
		setDir = set
		return div(
			ComponentProps(r.reader("outside"), 0),
			div(Dir(d),
				ComponentProps(r.reader("inside"), 0),
				div(ComponentProps(r.reader("deep", Key("deep")), 0)),
				div(Dir("ltr"), ComponentProps(r.reader("own"), 0)),
				ComponentProps(other, 0),
			),
		)
	}
	tr := newTestTree(t, 200, 200, Component(app), nil)
	tr.Frame()
	for _, n := range []string{"outside", "inside", "deep", "own"} {
		r.expect(t, "mount", n, 1, "ltr")
	}

	setDir.Set("rtl")
	tr.Frame()
	r.expect(t, "rtl", "outside", 1, "ltr")
	r.expect(t, "rtl", "inside", 2, "rtl")
	r.expect(t, "rtl", "deep", 2, "rtl")
	r.expect(t, "rtl", "own", 1, "ltr")
	if others != 1 {
		t.Errorf("rtl: the component that reads no direction built %d times, want 1", others)
	}
	if d := keyed(tr, "deep").box.cs.Text.Direction; d != text.DirectionRTL {
		t.Errorf("rtl: the subtree's style direction is %v", d)
	}

	// Removing the Dir: the readers follow the parent again (ltr).
	setDir.Set("")
	tr.Frame()
	r.expect(t, "removed", "inside", 3, "ltr")
	r.expect(t, "removed", "deep", 3, "ltr")

	// Dir("ltr") where the parent already is ltr changes no reader.
	setDir.Set("ltr")
	tr.Frame()
	r.expect(t, "ltr", "inside", 3, "ltr")
	r.expect(t, "ltr", "deep", 3, "ltr")
	r.expect(t, "ltr", "outside", 1, "ltr")
	r.expect(t, "ltr", "own", 1, "ltr")
}

// TestUseDirectionAuto: under Dir("auto"), the hook gives the direction
// the element resolves from its text. A reader first built before the
// element was styled reads ltr and builds again in the same frame when
// it resolves to rtl, also when the reader's own output is the text; a
// change of the text that flips the direction builds the readers again
// in the frame of the change, and one that does not flip it does not.
func TestUseDirectionAuto(t *testing.T) {
	r := dirReads{}
	var setText Setter[string]
	app := func() Node {
		s, set := UseState("مرحبا")
		setText = set
		return div(
			div(Dir("auto"), Key("auto"),
				span(Text(s)),
				ComponentProps(r.reader("reader"), 0),
			),
			div(Dir("auto"), Key("self"),
				ComponentProps(r.reader("self", span(Text("שלום"))), 0),
			),
			div(Dir("auto"), Key("latin"),
				ComponentProps(r.reader("latin", span(Text("hello"))), 0),
			),
		)
	}
	tr := newTestTree(t, 300, 200, Component(app), nil)
	tr.Frame()
	r.expect(t, "mount", "reader", 2, "rtl")
	r.expect(t, "mount", "self", 2, "rtl")
	r.expect(t, "mount", "latin", 1, "ltr")
	if tr.Dirty() {
		t.Error("mount: the frame left work for the next one")
	}
	if d := keyed(tr, "auto").box.cs.Text.Direction; d != text.DirectionRTL {
		t.Errorf("mount: auto element direction %v, want rtl", d)
	}

	setText.Set("hello")
	tr.Frame()
	r.expect(t, "latin text", "reader", 3, "ltr")
	setText.Set("hello again")
	tr.Frame()
	r.expect(t, "still latin", "reader", 3, "ltr")
	setText.Set("مرحبا")
	tr.Frame()
	r.expect(t, "arabic again", "reader", 4, "rtl")
	r.expect(t, "arabic again", "self", 2, "rtl")
	r.expect(t, "arabic again", "latin", 1, "ltr")
}

// TestUseDirectionAutoInsideRTL: a reader below an auto element inside
// an rtl one reads the auto element's result, and an rtl Dir changed to
// auto (Latin text) moves its readers to ltr.
func TestUseDirectionAutoInsideRTL(t *testing.T) {
	r := dirReads{}
	var setDir Setter[string]
	app := func() Node {
		d, set := UseState("rtl")
		setDir = set
		return div(Dir("rtl"),
			div(Dir(d), span(Text("abc")), ComponentProps(r.reader("reader"), 0)),
		)
	}
	tr := newTestTree(t, 300, 200, Component(app), nil)
	tr.Frame()
	r.expect(t, "mount", "reader", 1, "rtl")
	setDir.Set("auto")
	tr.Frame()
	r.expect(t, "auto", "reader", 2, "ltr")
	setDir.Set("")
	tr.Frame()
	r.expect(t, "inherit", "reader", 3, "rtl")
}
