package core

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// portalApp: a page whose middle component renders a modal through a
// Portal, under a context provider and a click handler of its own.
func portalApp(open *bool, clicks *[]string, ctxVal *string) Node {
	ctx := CreateContext("default")
	modal := func() Node {
		v := UseContext(ctx)
		*ctxVal = v
		return div(Key("overlay"), Class("fixed inset-0 bg-black/50"),
			div(Key("dialog"), Class("absolute top-10 left-10 h-20 w-40 bg-white"),
				button(Key("ok"), Class("h-8 w-16"), Text("OK"),
					On("click", func(MouseEvent) { *clicks = append(*clicks, "ok") }))))
	}
	comp := func() Node {
		var portal Node
		if *open {
			portal = Portal(Component(modal))
		}
		return div(Class("relative"),
			button(Key("a"), Class("h-8 w-16"), Text("A")),
			ctx.Provider("provided",
				div(Key("owner"), Class("relative z-50 h-10 overflow-hidden bg-sky-200"),
					On("click", func(e MouseEvent) { *clicks = append(*clicks, "owner") }),
					portal)),
			button(Key("b"), Class("relative z-40 h-8 w-16"), Text("B")),
		)
	}
	return Component(comp)
}

func TestPortal(t *testing.T) {
	open := true
	var clicks []string
	var ctxVal string
	x := newInput(t, portalApp(&open, &clicks, &ctxVal), 400, 300)
	tr := x.tree()
	if ctxVal != "provided" {
		t.Errorf("context in the portal = %q, want the provider's value", ctxVal)
	}
	// Laid out against the window, though the owner is positioned,
	// clipped and small.
	if r := mustFind(t, tr, "overlay").Rect; r != (gfx.Rect{X1: 400, Y1: 300}) {
		t.Errorf("overlay at %v, want the window", r)
	}
	if r := mustFind(t, tr, "dialog").Rect; r.X0 != 40 || r.Y0 != 40 {
		t.Errorf("dialog at %v, want (40, 40)", r)
	}
	// Painted above the z-50 owner: the overlay is hit over it, and the
	// click on OK bubbles through the component tree to the owner.
	if el := tr.HitTest(200, 20); el == nil || el.key != "overlay" {
		t.Errorf("hit over the owner: %v, want overlay", el)
	}
	x.click(50, 50)
	if strings.Join(clicks, ",") != "ok,owner" {
		t.Errorf("clicks %v, want ok then owner (bubbling through the Portal)", clicks)
	}
	// Tab order and the aria snapshot put the portal's content last.
	// (The click focused OK, the last element: Tab wraps around.)
	x.w.SetFocused(true)
	x.b.Drain(x.h)
	for _, want := range []string{"a", "b", "ok"} {
		x.tab()
		if f := tr.Focused(); f == nil || f.key != want {
			t.Errorf("Tab focuses %v, want %s", f, want)
		}
	}
	snap := tr.AriaSnapshot()
	if ia, ib, iok := strings.Index(snap, `"A"`), strings.Index(snap, `"B"`), strings.Index(snap, `"OK"`); !(ia < ib && ib < iok) {
		t.Errorf("aria snapshot order:\n%s", snap)
	}
	// Inspect shows the portal layer as the root's last child.
	root := tr.Inspect()
	if last := root.Children[len(root.Children)-1]; last.Kind != "portal" {
		t.Errorf("root's last child is %q, want portal", last.Kind)
	}
	// Unmounting removes the layer.
	open = false
	tr.markBuild(tr.root.children[0])
	x.frame()
	if len(tr.portals) != 0 || len(tr.rootBox.kids) != 1 {
		t.Errorf("after unmount: %d portals, %d root boxes", len(tr.portals), len(tr.rootBox.kids))
	}
	if el := tr.HitTest(50, 50); el != nil && el.key == "ok" {
		t.Errorf("unmounted portal still hit")
	}
}

// TestPortalsStackInMountOrder: two portals, the later on top; the layer
// itself lets the content under its empty parts be hit.
func TestPortalsStackInMountOrder(t *testing.T) {
	tr := newTestTree(t, 300, 200, div(
		div(Key("page"), Class("h-[200px] bg-white")),
		Portal(div(Key("first"), Class("fixed top-0 left-0 size-20 bg-red-500"))),
		Portal(div(Key("second"), Class("fixed top-10 left-10 size-20 bg-blue-500"))),
	), nil)
	tr.Frame()
	if el := tr.HitTest(50, 50); el == nil || el.key != "second" {
		t.Errorf("overlap hit %v, want second", el)
	}
	if el := tr.HitTest(5, 5); el == nil || el.key != "first" {
		t.Errorf("hit %v, want first", el)
	}
	if el := tr.HitTest(250, 150); el == nil || el.key != "page" {
		t.Errorf("hit %v through the portal layers, want page", el)
	}
}

func TestPortalRejectsProps(t *testing.T) {
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "Portal") {
			t.Errorf("Portal(Class(...)) did not panic with a hint: %v", r)
		}
	}()
	Portal(Class("p-4"))
}
