//go:build teladev

package core

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
)

// Changes between the build that captured the state and the one that
// restores it. Go cannot give two functions of one binary the same name,
// so an edited component is a second function whose identity devAlias
// maps to the first one's (the test-only seam devIdentAlias): to the dev
// state it is the same component, as an edited function is after a
// rebuild.

func devCounter() Node {
	n, _ := UseState(0)
	return div(Textf("counter %d", n))
}

// devCounterMore is devCounter with a hook added after the first.
func devCounterMore() Node {
	n, _ := UseState(0)
	s, _ := UseState("x")
	return div(Textf("counter %d %s", n, s))
}

// devCounterString is devCounter with the state's type changed.
func devCounterString() Node {
	n, _ := UseState("")
	return div(Text("counter " + n))
}

// devCounterEffect is devCounter with an effect added before the state.
func devCounterEffect() Node {
	UseEffect(func() func() { return nil }, Once)
	n, _ := UseState(0)
	return div(Textf("counter %d", n))
}

func devSibling() Node {
	n, _ := UseState(0)
	return div(Textf("sibling %d", n))
}

func devOther() Node {
	n, _ := UseState(0)
	return div(Textf("other %d", n))
}

// devParent holds a stateful child; devParentMore adds a hook to it.
func devParent() Node {
	n, _ := UseState(0)
	return div(Textf("parent %d", n), Component(devChild))
}

func devParentMore() Node {
	n, _ := UseState(0)
	UseMemo(func() int { return 1 }, Once)
	return div(Textf("parent %d", n), Component(devChild))
}

func devChild() Node {
	n, _ := UseState(0)
	return div(Textf("child %d", n))
}

func TestDevStateAddedHook(t *testing.T) {
	a := devTestTree(t, div(Component(devCounter, Key("c")), Component(devSibling, Key("s"))), nil, nil)
	devSet(t, a, devComp(t, a, "devCounter"), 0, 5)
	devSet(t, a, devComp(t, a, "devSibling"), 0, 6)
	data, _ := devSnap(t, a)

	devAlias(t, map[string]string{"devCounterMore": "devCounter"})
	b := devTestTree(t, div(Component(devCounterMore, Key("c")), Component(devSibling, Key("s"))), data, nil)
	if got := devState[int](t, devComp(t, b, "devCounterMore"), 0); got != 0 {
		t.Errorf("the counter with a new hook kept %d; it must reset", got)
	}
	if got := devState[int](t, devComp(t, b, "devSibling"), 0); got != 6 {
		t.Errorf("the sibling has %d, want 6", got)
	}
	r := b.DevSeedReport()
	if len(r.Remounted) != 1 || !strings.Contains(r.Remounted[0], "devCounterMore: hook 2, UseState[string], is new") {
		t.Errorf("remounted %q", r.Remounted)
	}
	if r.Restored != 1 {
		t.Errorf("restored %d slots, want 1 (the sibling's)", r.Restored)
	}
}

func TestDevStateChangedType(t *testing.T) {
	a := devTestTree(t, div(Component(devCounter)), nil, nil)
	devSet(t, a, devComp(t, a, "devCounter"), 0, 5)
	data, _ := devSnap(t, a)

	devAlias(t, map[string]string{"devCounterString": "devCounter"})
	b := devTestTree(t, div(Component(devCounterString)), data, nil)
	if got := devState[string](t, devComp(t, b, "devCounterString"), 0); got != "" {
		t.Errorf("state %q, want a fresh one", got)
	}
	if r := b.DevSeedReport(); len(r.Remounted) != 1 || !strings.Contains(r.Remounted[0], "hook 1 was UseState[int], is UseState[string]") {
		t.Errorf("remounted %q", r.Remounted)
	}
}

func TestDevStateHookBefore(t *testing.T) {
	// An effect added before the state: the state is not seeded (its
	// index's entry differs), and nothing has to be built again.
	a := devTestTree(t, div(Component(devCounter)), nil, nil)
	devSet(t, a, devComp(t, a, "devCounter"), 0, 5)
	data, _ := devSnap(t, a)

	devAlias(t, map[string]string{"devCounterEffect": "devCounter"})
	b := devTestTree(t, div(Component(devCounterEffect)), data, nil)
	if got := devState[int](t, devComp(t, b, "devCounterEffect"), 1); got != 0 {
		t.Errorf("state %d, want a fresh one", got)
	}
}

func TestDevStateSubtreeResets(t *testing.T) {
	// A component whose hooks changed resets with its subtree, even though
	// the child is unchanged; the discarded first build's state slot is not
	// kept, and the rebuild starts from the initial value.
	a := devTestTree(t, div(Component(devParent)), nil, nil)
	devSet(t, a, devComp(t, a, "devParent"), 0, 3)
	devSet(t, a, devComp(t, a, "devChild"), 0, 4)
	data, _ := devSnap(t, a)

	devAlias(t, map[string]string{"devParentMore": "devParent"})
	b := devTestTree(t, div(Component(devParentMore)), data, nil)
	if got := devState[int](t, devComp(t, b, "devParentMore"), 0); got != 0 {
		t.Errorf("parent %d, want a fresh one", got)
	}
	if got := devState[int](t, devComp(t, b, "devChild"), 0); got != 0 {
		t.Errorf("child %d, want a fresh one: its parent reset", got)
	}
	if r := b.DevSeedReport(); r.Restored != 0 || len(r.Remounted) != 1 {
		t.Errorf("report %+v", r)
	}
	// Unchanged, both keep their state.
	c := devTestTree(t, div(Component(devParent)), data, nil)
	if got := devState[int](t, devComp(t, c, "devChild"), 0); got != 4 {
		t.Errorf("child %d, want 4", got)
	}
}

// devTrackV is the Value of the last devTrackHost built.
var devTrackV *Value[int]

// devTrackHost passes a Value of its own to a child that reads it only
// while the child's state is true; devTrackHostMore's child has a hook
// added.
func devTrackHost() Node {
	v := UseValue(0)
	devTrackV = v
	return div(ComponentProps(devTrackChild, v))
}

func devTrackHostMore() Node {
	v := UseValue(0)
	devTrackV = v
	return div(ComponentProps(devTrackChildMore, v))
}

func devTrackChild(v *Value[int]) Node {
	show, _ := UseState(false)
	if show {
		_ = v.Get()
	}
	return div(Textf("show %v", show))
}

func devTrackChildMore(v *Value[int]) Node {
	show, _ := UseState(false)
	UseMemo(func() int { return 1 }, Once)
	if show {
		_ = v.Get()
	}
	return div(Textf("show %v", show))
}

func TestDevStateDiscardedBuildReads(t *testing.T) {
	// A component whose hooks changed after a slot was seeded builds again
	// from scratch: the Values only the discarded build read are not its
	// subscriptions (a change would rebuild it for nothing, and keep the
	// Value referenced).
	a := devTestTree(t, Component(devTrackHost), nil, nil)
	devSet(t, a, devComp(t, a, "devTrackChild"), 0, true)
	if el := devComp(t, a, "devTrackChild"); el.vobs == nil || len(el.vobs.subs) != 1 {
		t.Fatal("setup: the child does not read the Value")
	}
	data, _ := devSnap(t, a)

	devAlias(t, map[string]string{"devTrackHostMore": "devTrackHost", "devTrackChildMore": "devTrackChild"})
	b := devTestTree(t, Component(devTrackHostMore), data, nil)
	el := devComp(t, b, "devTrackChildMore")
	if devState[bool](t, el, 0) {
		t.Fatal("the child whose hooks changed kept its state")
	}
	if r := b.DevSeedReport(); len(r.Remounted) != 1 {
		t.Fatalf("remounted %q, want the child", r.Remounted)
	}
	if el.vobs != nil && len(el.vobs.subs) != 0 {
		t.Errorf("the child is subscribed to %d Values read only by its discarded build", len(el.vobs.subs))
	}
	builds := el.builds
	devTrackV.Set(1)
	devFrames(b)
	if el.builds != builds {
		t.Errorf("a Value only the discarded build read rebuilt the child (%d builds, want %d)", el.builds, builds)
	}
}

// devRow is a list item with state.
func devRow(id int) Node {
	n, _ := UseState(0)
	return div(Textf("row %d: %d", id, n))
}

func devRows(ids []int) Node {
	return div(Map(ids, func(id int) Node { return ComponentProps(devRow, id, Key(id)) }))
}

func TestDevStateKeyedReorder(t *testing.T) {
	a := devTestTree(t, devRows([]int{1, 2, 3}), nil, nil)
	for _, id := range []int{1, 2, 3} {
		devSet(t, a, devComp(t, a, "devRow", id), 0, id*10)
	}
	data, _ := devSnap(t, a)
	b := devTestTree(t, devRows([]int{3, 1, 4, 2}), data, nil)
	for _, c := range []struct{ id, want int }{{1, 10}, {2, 20}, {3, 30}, {4, 0}} {
		if got := devState[int](t, devComp(t, b, "devRow", c.id), 0); got != c.want {
			t.Errorf("row %d has %d, want %d", c.id, got, c.want)
		}
	}
}

func TestDevStatePositions(t *testing.T) {
	a := devTestTree(t, div(Component(devCounter), Component(devSibling)), nil, nil)
	devSet(t, a, devComp(t, a, "devCounter"), 0, 1)
	devSet(t, a, devComp(t, a, "devSibling"), 0, 2)
	data, _ := devSnap(t, a)

	// A component inserted before them: without keys the identity is the
	// position, so both move to positions that held something else, and
	// mount fresh (as reconcile would remount them).
	b := devTestTree(t, div(Component(devOther), Component(devCounter), Component(devSibling)), data, nil)
	for _, name := range []string{"devOther", "devCounter", "devSibling"} {
		if got := devState[int](t, devComp(t, b, name), 0); got != 0 {
			t.Errorf("%s has %d, want a fresh state", name, got)
		}
	}
	if r := b.DevSeedReport(); r.Unused != 2 {
		t.Errorf("unused %d, want 2", r.Unused)
	}
	// Appended after them, they keep their positions and their state.
	d := devTestTree(t, div(Component(devCounter), Component(devSibling), Component(devOther)), data, nil)
	if got := devState[int](t, devComp(t, d, "devSibling"), 0); got != 2 {
		t.Errorf("appended after: sibling %d, want 2", got)
	}
}

func TestDevStateRemoved(t *testing.T) {
	a := devTestTree(t, div(Component(devCounter), Component(devSibling)), nil, nil)
	devSet(t, a, devComp(t, a, "devCounter"), 0, 1)
	devSet(t, a, devComp(t, a, "devSibling"), 0, 2)
	data, _ := devSnap(t, a)
	b := devTestTree(t, div(Component(devCounter)), data, nil)
	if got := devState[int](t, devComp(t, b, "devCounter"), 0); got != 1 {
		t.Errorf("counter %d, want 1", got)
	}
	if r := b.DevSeedReport(); r.Unused != 1 || r.Restored != 1 {
		t.Errorf("report %+v, want 1 restored and 1 unused", r)
	}
}

// devCard is a struct component with state.
type devCard struct{ Title string }

func (c devCard) Build() Node {
	n, _ := UseState(0)
	return div(Textf("%s %d", c.Title, n))
}

type devPanel struct{ Title string }

func (c devPanel) Build() Node {
	n, _ := UseState(0)
	return div(Textf("%s %d", c.Title, n))
}

func TestDevStateStructComponents(t *testing.T) {
	a := devTestTree(t, div(devCard{Title: "a"}, devPanel{Title: "b"}), nil, nil)
	devSet(t, a, devComp(t, a, "devCard"), 0, 1)
	devSet(t, a, devComp(t, a, "devPanel"), 0, 2)
	data, _ := devSnap(t, a)
	// New props keep the state (the type is the identity); another type at
	// a position resets.
	b := devTestTree(t, div(devCard{Title: "changed"}, devCard{Title: "c"}), data, nil)
	if got := devState[int](t, b.root.children[0].children[0], 0); got != 1 {
		t.Errorf("card %d, want 1", got)
	}
	if got := devState[int](t, b.root.children[0].children[1], 0); got != 0 {
		t.Errorf("a card where a panel was: %d, want 0", got)
	}
}

// devHandles holds values that cannot be kept: a func (a warning), and
// Tela's handles (silent).
func devHandles() Node {
	f, _ := UseState(func() int { return 1 })
	ref := UseElementRef()
	r := UseRef(ref)
	n, set := UseState(0)
	h, _ := UseState(set)
	m, _ := UseState(map[string]struct{}{"a": {}})
	s := UseRef(strings.Builder{}) // a type with unexported fields

	_, _, _, _, _ = f, r, h, m, s
	return div(AttachRef(ref), Textf("%d", n))
}

func TestDevStateNotKept(t *testing.T) {
	var warns []string
	a := devTestTree(t, div(Component(devHandles)), nil, &warns)
	devSet(t, a, devComp(t, a, "devHandles"), 3, 9)
	warns = nil
	data, rep := devSnap(t, a)
	if rep.Kept != 1 || len(rep.Reset) != 5 {
		t.Fatalf("report %v (%+v), want 1 kept and 5 reset", rep, rep.Reset)
	}
	var silent int
	for _, r := range rep.Reset {
		if r.Silent {
			silent++
		}
	}
	if silent != 2 {
		t.Errorf("%d silent resets, want 2 (the ElementRef and the Setter): %+v", silent, rep.Reset)
	}
	if len(warns) != 3 {
		t.Fatalf("warnings %q, want 3 (the func, the empty struct, the unexported field)", warns)
	}
	if !strings.Contains(warns[0], "devHandles's UseState #1 holds a func() int; it resets on reload") {
		t.Errorf("warning %q", warns[0])
	}
	if !strings.Contains(warns[1], "struct {}") || !strings.Contains(warns[2], "strings.Builder with the unexported field addr") {
		t.Errorf("warnings %q", warns[1:])
	}
	// Once per slot: neither a second capture nor the next build warns
	// again.
	devSnap(t, a)
	b := devTestTree(t, div(Component(devHandles)), data, &warns)
	if got := devState[int](t, devComp(t, b, "devHandles"), 3); got != 9 {
		t.Errorf("kept state %d, want 9", got)
	}
	devSnap(t, b)
	if len(warns) != 3 {
		t.Errorf("warnings after reload %q, want the 3 of the first capture only", warns)
	}
}

func TestDevStateBadSnapshot(t *testing.T) {
	a := devTestTree(t, div(Component(devCounter)), nil, nil)
	devSet(t, a, devComp(t, a, "devCounter"), 0, 5)
	data, _ := devSnap(t, a)

	old := append([]byte(nil), data...)
	old[len(devMagic)] = 99
	cut := data[:len(data)/2]
	flip := append([]byte(nil), data...)
	for i := len(devMagic) + 1; i < len(flip); i += 3 {
		flip[i] ^= 0x5a
	}
	// Well-formed gob with an identity index out of range on a node that is
	// not the root: an empty ID must not exempt it (only the root has no
	// identity).
	crafted := func(kid devNode) []byte {
		b, err := encodeDevSnapshot(&devSnapshot{Idents: []string{"P:div"}, Root: devNode{Kids: []devNode{kid}}})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, c := range []struct {
		name string
		data []byte
		want string
	}{
		{"version", old, "format version 99"},
		{"truncated", cut, "damaged"},
		{"garbage", []byte("hello"), "not a tela dev state"},
		{"flipped", flip, "cannot be restored"},
		{"identity", crafted(devNode{Ident: 7, ID: "0/0"}), "identity 7 of 1"},
		{"identity without ID", crafted(devNode{Ident: 7}), "identity 7 of 1"},
		{"nested identity without ID", crafted(devNode{Ident: 0, ID: "0/0", Kids: []devNode{{Ident: 9}}}), "identity 9 of 1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var warns []string
			b := devTestTree(t, div(Component(devCounter)), c.data, &warns)
			if got := devState[int](t, devComp(t, b, "devCounter"), 0); got != 0 && c.name != "flipped" {
				t.Errorf("state %d, want a fresh start", got)
			}
			if len(warns) != 1 || !strings.Contains(warns[0], c.want) {
				t.Errorf("warnings %q, want one with %q", warns, c.want)
			}
		})
	}
}

func devSpring() Node {
	v := UseSpring(float32(1), Initial(float32(0)))
	w := UseValue(float32(0))
	return div(Key("s"), Class("h-10"), On("click", func(MouseEvent) { Animate(w, 100, Duration(time.Second)) }),
		Textf("%g %g", v.Get(), w.Get()))
}

func TestDevStateAnimationsFromRest(t *testing.T) {
	a := newKeys(t, Component(devSpring), 200, 100)
	a.frame()
	a.click(a.center("s"))
	a.frame()
	a.frame() // Animate is under way, the spring too (from 0 to 1)
	el := devComp(t, a.tree(), "devSpring")
	if w := el.hooks[1].(*Value[float32]).Get(); w <= 0 || w >= 100 {
		t.Fatalf("Animate not under way: %g", w)
	}
	data, _ := devSnap(t, a.tree())

	SetDevSeed(data)
	b := newKeys(t, Component(devSpring), 200, 100)
	el = devComp(t, b.tree(), "devSpring")
	if v := el.hooks[0].(*motionSlot[float32]).v.Get(); v != 1 {
		t.Errorf("UseSpring starts at %g, want its target 1", v)
	}
	if w := el.hooks[1].(*Value[float32]).Get(); w != 100 {
		t.Errorf("an animated Value restarts at %g, want its target 100", w)
	}
	b.frame()
	if b.tree().anim.tk.Active() {
		t.Errorf("animations run after the reload; they start at rest")
	}
}

func TestDevStateSchemeAndTheme(t *testing.T) {
	a := devTestTree(t, div(Component(devCounter)), nil, nil)
	a.SetColorScheme(ColorSchemeDark)
	a.SetTheme(Theme{Radius: 4, Colors: map[string]SchemeColor{"brand": {Light: gfx.Black, Dark: gfx.White}}})
	devFrames(a)
	data, _ := devSnap(t, a)
	b := devTestTree(t, div(Component(devCounter)), data, nil)
	if b.ColorScheme() != ColorSchemeDark {
		t.Errorf("scheme %v, want Dark", b.ColorScheme())
	}
	if _, ok := b.parser.Theme().Tokens["brand"]; !ok {
		t.Errorf("the theme set at run time was not restored")
	}
	// Without a change at run time, the app's configuration stays.
	c := devTestTree(t, div(Component(devCounter)), nil, nil)
	data, _ = devSnap(t, c)
	d := devTestTree(t, div(Component(devCounter)), data, nil)
	if d.ColorScheme() != ColorSchemeLight {
		t.Errorf("scheme %v, want the configured Light", d.ColorScheme())
	}
}

func devVRow(i int) Node {
	n, _ := UseState(0)
	return div(Class("h-[40px]"), Textf("row %d %d", i, n))
}

func devVirtualPage() Node {
	return div(VirtualList(1000, func(int) float32 { return 40 }, func(i int) Node { return ComponentProps(devVRow, i) },
		Class("h-[200px]"), ItemKey(func(i int) int { return i })))
}

// devRowEl returns the mounted devVRow of item i, or nil.
func devRowEl(tr *Tree, i int) *Element {
	var found *Element
	var walk func(el *Element)
	walk = func(el *Element) {
		if found != nil {
			return
		}
		if el.kind == elemComponent && el.comp.typ != 0 && devCompName(el.comp) == "devVRow" && el.comp.props == any(i) {
			found = el
			return
		}
		for _, c := range el.children {
			walk(c)
		}
	}
	walk(tr.root)
	return found
}

// devListScroller returns the scroller of the tree's VirtualList.
func devListScroller(t *testing.T, tr *Tree) *scroller {
	t.Helper()
	for _, s := range tr.scr.list {
		if s != nil && s.box != tr.rootBox {
			return s
		}
	}
	t.Fatal("no list scroller")
	return nil
}

func TestDevStateVirtualList(t *testing.T) {
	// The items a VirtualList builds for the restored offset mount in the
	// first frame, and take their saved state.
	a := devTestTree(t, Component(devVirtualPage), nil, nil)
	a.scrollDirect(devListScroller(t, a), gfx.Point{Y: 4000})
	devFrames(a)
	el := devRowEl(a, 101)
	if el == nil {
		t.Fatal("row 101 is not built at offset 4000")
	}
	devSet(t, a, el, 0, 7)
	data, rep := devSnap(t, a)
	if rep.Scrolls != 1 || rep.Kept < 5 {
		t.Errorf("report %v, want the offset and the slots of the rows built", rep)
	}
	b := devTestTree(t, Component(devVirtualPage), data, nil)
	if got := devListScroller(t, b).pos.Y; got != 4000 {
		t.Errorf("list offset %g, want 4000", got)
	}
	el = devRowEl(b, 101)
	if el == nil {
		t.Fatal("row 101 is not built after the reload")
	}
	if got := devState[int](t, el, 0); got != 7 {
		t.Errorf("row 101 has %d, want 7", got)
	}
	if r := b.DevSeedReport(); r.Restored != rep.Kept || r.Scrolls != 1 || r.Unused != 0 {
		t.Errorf("seed report %+v, want %d slots restored", r, rep.Kept)
	}
}

func devFields() Node {
	return div(Class("flex flex-col gap-2 p-2"),
		input(Key("long"), Class("w-[60px]"), DefaultValue("a long value that overflows its field")),
		textarea(Key("ta"), Class("h-[40px] w-[120px]"), Rows(2), DefaultValue("1\n2\n3\n4\n5\n6\n7\n8")),
	)
}

func TestDevStateFields(t *testing.T) {
	a := newKeys(t, Component(devFields), 300, 200)
	a.frame()
	a.click(a.center("ta"))
	a.key(platform.KeyEnd, platform.ModCtrl) // to the end: the textarea scrolls
	a.typeText("9")
	a.click(a.center("long"))
	a.key(platform.KeyEnd, 0) // the input scrolls itself
	a.typeText("!")
	devRest(a)
	if fieldOf(t, a.tree(), "long").scrollX == 0 || a.offset("ta") == 0 {
		t.Fatalf("nothing scrolled: input %g, textarea %g", fieldOf(t, a.tree(), "long").scrollX, a.offset("ta"))
	}
	data, rep := devSnap(t, a.tree())
	if rep.Fields != 2 || rep.Scrolls != 1 {
		t.Errorf("report %v", rep)
	}
	SetDevSeed(data)
	b := newKeys(t, Component(devFields), 300, 200)
	b.frame()
	devRest(b)
	if got, want := fieldOf(t, b.tree(), "long").scrollX, fieldOf(t, a.tree(), "long").scrollX; got != want {
		t.Errorf("input scroll %g, want %g", got, want)
	}
	if got, want := b.offset("ta"), a.offset("ta"); got != want {
		t.Errorf("textarea offset %g, want %g", got, want)
	}
	if got, want := b.value("ta"), a.value("ta"); got != want {
		t.Errorf("textarea %q, want %q", got, want)
	}
	if ia, ib := a.w.Frame(), b.w.Frame(); ia == nil || ib == nil || !bytes.Equal(ia.Pix, ib.Pix) {
		t.Errorf("the restored window's pixels differ from the original's")
	}
	b.ctrl("z", 0)
	if got, want := b.value("long"), "a long value that overflows its field"; got != want {
		t.Errorf("undo after the reload: %q, want %q", got, want)
	}
}

func devPresencePage() Node {
	show, setShow := UseState(true)
	return div(
		button(Key("hide"), Class("h-6"), On("click", func(MouseEvent) { setShow.Set(false) }), Text("hide")),
		Presence(If(show, div(Key("c"), Class("transition-opacity duration-300"), Exit("opacity-0"), Component(devCounter)))),
	)
}

func TestDevStatePresenceExitDropped(t *testing.T) {
	a := newKeys(t, Component(devPresencePage), 200, 100)
	a.frame()
	Setter[int]{devComp(t, a.tree(), "devCounter").hooks[0].(*stateSlot[int])}.Set(5)
	a.settle()
	_, rep := devSnap(t, a.tree())
	if rep.Kept != 2 {
		t.Fatalf("kept %d, want 2 (show and the counter)", rep.Kept)
	}
	a.click(a.center("hide"))
	a.frame() // the child exits: its transition runs
	if !keyed(a.tree(), "c").inert {
		t.Fatal("the child does not exit")
	}
	data, rep := devSnap(t, a.tree())
	if rep.Kept != 1 {
		t.Errorf("kept %d slots while the child exits, want 1: an exiting child is not kept", rep.Kept)
	}
	b := devTestTree(t, Component(devPresencePage), data, nil)
	if got := devState[bool](t, devComp(t, b, "devPresencePage"), 0); got {
		t.Errorf("show %v, want false", got)
	}
}
