//go:build teladev

package core

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
)

// Tests of the dev-loop state (devstate.go, ADR-113). Run with
// go test -tags teladev ./internal/core/.

// ---------------------------------------------------------------------
// Helpers.

// devTestTree mounts root in a dev-mode tree seeded with data (nil: none)
// and runs frames until it is idle; warnings go to *warns.
func devTestTree(t *testing.T, root Node, data []byte, warns *[]string) *Tree {
	t.Helper()
	cfg := Config{Fonts: testFonts, Dev: true}
	cfg.Warn = func(m string) {
		if warns != nil {
			*warns = append(*warns, m)
		}
	}
	cfg.Log = func(m string) { t.Logf("log: %s", m) }
	tr := NewTree(cfg)
	tr.SetViewport(gfx.Size{W: 400, H: 300})
	if data != nil {
		tr.SeedDevState(data)
	}
	tr.Mount(root)
	devFrames(tr)
	return tr
}

// devFrames runs frames and effects until the tree is idle (a few).
func devFrames(tr *Tree) {
	for i := 0; i < 10; i++ {
		tr.Frame()
		tr.FlushEffects()
		if !tr.Dirty() {
			return
		}
	}
}

// devSnap captures tr's state.
func devSnap(t *testing.T, tr *Tree) ([]byte, DevStateReport) {
	t.Helper()
	data, rep, err := tr.CaptureDevState()
	if err != nil {
		t.Fatalf("CaptureDevState: %v", err)
	}
	return data, rep
}

// devComp returns the mounted component named name (devCompName), with
// key k when given.
func devComp(t *testing.T, tr *Tree, name string, k ...any) *Element {
	t.Helper()
	var found *Element
	var walk func(el *Element)
	walk = func(el *Element) {
		if found != nil {
			return
		}
		if el.kind == elemComponent && el.prov == nil && !el.comp.presence && devCompName(el.comp) == name &&
			(len(k) == 0 || el.keyed && safeEqual(el.key, k[0])) {
			found = el
			return
		}
		for _, c := range el.children {
			walk(c)
		}
	}
	walk(tr.root)
	if found == nil {
		t.Fatalf("no component %s %v", name, k)
	}
	return found
}

// devState returns state slot i of component el.
func devState[T any](t *testing.T, el *Element, i int) T {
	t.Helper()
	s, ok := el.hooks[i].(*stateSlot[T])
	if !ok {
		t.Fatalf("hook %d of %s is a %T", i, devCompName(el.comp), el.hooks[i])
	}
	return s.value
}

// devSet sets state slot i of component el and runs frames.
func devSet[T any](t *testing.T, tr *Tree, el *Element, i int, v T) {
	t.Helper()
	Setter[T]{el.hooks[i].(*stateSlot[T])}.Set(v)
	devFrames(tr)
}

// devAlias makes the component identities of the keys those of the
// values, for the test: a function of this test binary standing for an
// edited version of another.
func devAlias(t *testing.T, alias map[string]string) {
	t.Helper()
	m := map[string]string{}
	for k, v := range alias {
		m["F:"+corePkg+"."+k] = "F:" + corePkg + "." + v
	}
	devIdentAlias.Store(&m)
	t.Cleanup(func() { devIdentAlias.Store(nil) })
}

// ---------------------------------------------------------------------
// Round trip through a window.

type devPageData struct {
	Name string
	Tags []string
	N    *int
	At   time.Time
}

func devAppend(s, a string) string { return s + a }

func devPage() Node {
	count, setCount := UseState(0)
	mode, dispatch := UseReducer(devAppend, "a")
	val := UseValue(1.5)
	ref := UseRef(devPageData{Name: "x"})
	rows := make([]int, 30)
	for i := range rows {
		rows[i] = i
	}
	return div(Class("flex flex-col gap-1 p-2"),
		button(Key("inc"), On("click", func(MouseEvent) { setCount.Update(func(n int) int { return n + 1 }) }),
			Textf("count %d", count)),
		button(Key("mode"), On("click", func(MouseEvent) {
			dispatch.Dispatch("b")
			val.Set(val.Latest() * 2)
			n := 7
			ref.Current.Name += "y"
			ref.Current.Tags = append(ref.Current.Tags, "t")
			ref.Current.N = &n
			ref.Current.At = time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
		}), Textf("mode %s val %g", mode, val.Get())),
		input(Key("in"), DefaultValue("hello")),
		textarea(Key("ta"), DefaultValue("one")),
		div(Key("list"), Class("h-[60px] overflow-auto"),
			Map(rows, func(i int) Node { return div(Key(i), Class("h-[20px]"), Textf("row %d", i)) })),
	)
}

// devRest lets a harness's window go idle: animations end, the overlay
// scrollbars fade and the caret stops blinking (steady after 10 s).
func devRest(x *inputHarness) {
	for range 30 {
		x.b.Advance(500 * time.Millisecond)
		x.b.Wake()
		x.b.Drain(x.h)
		x.settle()
	}
}

func TestDevStateRoundTrip(t *testing.T) {
	root := Component(devPage)
	a := newKeys(t, root, 400, 300)
	a.frame()
	// State: two clicks, a reducer action, a Value, a ref.
	a.click(a.center("inc"))
	a.click(a.center("inc"))
	a.click(a.center("mode"))
	a.settle()
	// Scroll the list: one notch, animated.
	lx, ly := a.center("list")
	a.wheel(notch(lx, ly, 1))
	a.settle()
	if a.offset("list") == 0 {
		t.Fatal("the list did not scroll")
	}
	// Focus the input, type, move the caret.
	a.click(a.center("in"))
	a.typeText(" world")
	a.key(platform.KeyArrowLeft, 0)
	a.key(platform.KeyArrowLeft, 0)
	devRest(a)

	data, rep := devSnap(t, a.tree())
	t.Logf("capture: %v", rep)
	if rep.Kept != 4 || rep.Fields != 1 || rep.Scrolls != 1 || !rep.Focus || len(rep.Reset) != 0 {
		t.Errorf("report %+v, want 4 slots, 1 field, 1 scroll offset and the focus", rep)
	}

	SetDevSeed(data)
	b := newKeys(t, root, 400, 300)
	b.frame()
	devRest(b)
	sr := b.tree().DevSeedReport()
	if sr.Err != "" || sr.Restored != 4 || sr.Fields != 1 || sr.Scrolls != 1 || !sr.Focus || len(sr.Remounted) != 0 || sr.Unused != 0 {
		t.Errorf("seed report %+v", sr)
	}

	pa, pb := devComp(t, a.tree(), "devPage"), devComp(t, b.tree(), "devPage")
	if got := devState[int](t, pb, 0); got != 2 {
		t.Errorf("count %d, want 2", got)
	}
	if got := pb.hooks[1].(*reducerSlot[string, string]).value; got != "ab" {
		t.Errorf("reducer %q, want ab", got)
	}
	if got := pb.hooks[2].(*Value[float64]).Get(); got != 3 {
		t.Errorf("value %g, want 3", got)
	}
	ra, rb := pa.hooks[3].(*Ref[devPageData]).Current, pb.hooks[3].(*Ref[devPageData]).Current
	if rb.Name != ra.Name || !slices.Equal(rb.Tags, ra.Tags) || rb.N == nil || *rb.N != 7 || !rb.At.Equal(ra.At) {
		t.Errorf("ref %+v, want %+v", rb, ra)
	}
	if got, want := focusedName(b.tree()), "in"; got != want {
		t.Errorf("focus %s, want %s", got, want)
	}
	if b.tree().foc.visible != a.tree().foc.visible {
		t.Errorf("focus-visible %v, want %v", b.tree().foc.visible, a.tree().foc.visible)
	}
	if got, want := b.offset("list"), a.offset("list"); got != want {
		t.Errorf("list offset %g, want %g", got, want)
	}
	if got, want := b.value("in"), "hello world"; got != want {
		t.Errorf("input %q, want %q", got, want)
	}
	if got, want := fieldOf(t, b.tree(), "in").ed.DevState(), fieldOf(t, a.tree(), "in").ed.DevState(); !reflect.DeepEqual(got, want) {
		t.Errorf("editor %+v, want %+v", got, want)
	}
	// Same pixels.
	ia, ib := a.w.Frame(), b.w.Frame()
	if ia == nil || ib == nil || !bytes.Equal(ia.Pix, ib.Pix) {
		t.Errorf("the restored window's pixels differ from the original's")
		if dir := os.Getenv("TELA_DEVSTATE_DUMP"); dir != "" && ia != nil && ib != nil {
			for name, img := range map[string]*image.RGBA{"a.png": ia, "b.png": ib} {
				f, err := os.Create(filepath.Join(dir, name))
				if err == nil {
					png.Encode(f, img)
					f.Close()
				}
			}
		}
	}
	// The undo history came along: Ctrl+Z undoes the same step in both.
	a.ctrl("z", 0)
	b.ctrl("z", 0)
	if va, vb := a.value("in"), b.value("in"); va != vb || va == "hello world" {
		t.Errorf("after undo %q and %q", va, vb)
	}
	// The next capture of the restored tree is the same state.
	data2, rep2 := devSnap(t, b.tree())
	if rep2.Kept != rep.Kept || len(data2) == 0 {
		t.Errorf("second capture %+v", rep2)
	}
}

// devDirReader reads the direction between two states (UseDirection's
// slot is part of the hook signature, as UseColorScheme's is).
func devDirReader() Node {
	n, _ := UseState(0)
	dir := UseDirection()
	s, _ := UseState("")
	return div(Text(dir + s + string(rune('0'+n))))
}

// TestDevStateUseDirection: a component that calls UseDirection keeps the
// state around it across a reload, and its slot is neither kept nor
// reported as reset (it holds no state: the new build reads the direction
// again).
func TestDevStateUseDirection(t *testing.T) {
	root := Component(func() Node { return div(Dir("rtl"), Component(devDirReader)) })
	a := devTestTree(t, root, nil, nil)
	el := devComp(t, a, "devDirReader")
	devSet(t, a, el, 0, 3)
	devSet(t, a, el, 2, "x")
	data, rep := devSnap(t, a)
	if rep.Kept != 2 || len(rep.Reset) != 0 {
		t.Errorf("capture report %+v, want 2 kept slots and none reset", rep)
	}
	b := devTestTree(t, root, data, nil)
	sr := b.DevSeedReport()
	if sr.Err != "" || sr.Restored != 2 || len(sr.Remounted) != 0 {
		t.Errorf("seed report %+v", sr)
	}
	pb := devComp(t, b, "devDirReader")
	if devState[int](t, pb, 0) != 3 || devState[string](t, pb, 2) != "x" {
		t.Errorf("restored %d %q, want 3 x", devState[int](t, pb, 0), devState[string](t, pb, 2))
	}
	if !pb.hooks[1].(*dirSlot).rtl {
		t.Error("the restored reader does not read rtl")
	}
}
