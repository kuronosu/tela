package core

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/internal/style"
)

// kit.Progress with ValueFn slides its indicator with a TranslateFn, which
// makes the indicator a deferred layer for as long as the prop is there
// (ADR-263). The layer is cheap: a handful of commands and a marker, and a
// new value records only it and composites, never the page's list.
// Dropping it when the value stops changing would save that list and mean
// a build to bring it back; an app that wants the plain bar gives Value
// instead of ValueFn (the hooks are the same), so the layer stays.

// progressLikePage is a long page with a bar at its top, whose indicator is
// slid by a TranslateFn.
func progressLikePage(pct *Value[float32]) Node {
	rows := make([]Child, 0, 60)
	rows = append(rows, div(Class("h-2 w-[200px] overflow-hidden rounded-full bg-primary/20"),
		div(Key("ind"), Class("h-full w-full bg-primary"),
			Style(style.TranslateFn(func() (x, y float32) { return -(100 - pct.Get()) * 2, 0 })))))
	for i := range 50 {
		rows = append(rows, p(Key(fmt.Sprint("row", i)), Class("w-60"), Text(fmt.Sprint("A row of the page, number ", i))))
	}
	return div(rows...)
}

func TestProgressValueFnLayerCostsLittle(t *testing.T) {
	var pct *Value[float32]
	tr := newTestTree(t, 300, 600, Component(func() Node {
		pct = UseValue[float32](10)
		return progressLikePage(pct)
	}), nil)
	tr.Frame()
	if n := countLayers(tr, "deferred"); n != 1 {
		t.Fatalf("%d deferred layers, want the indicator's", n)
	}
	var cmds int
	for _, l := range tr.Layers() {
		if l.Kind == "deferred" {
			cmds = l.Commands
		}
	}
	if cmds == 0 || cmds > 4 {
		t.Errorf("the indicator's layer has %d commands, want a handful", cmds)
	}
	// A new value records the indicator's layer and nothing else.
	s0 := tr.Stats()
	pct.Set(60)
	tr.ApplyUpdates()
	tr.Frame()
	if d := work(s0, tr.Stats()); d != paintOnly {
		t.Errorf("work %+v, want a paint only", d)
	}
	for _, l := range tr.Layers() {
		if l.Recorded && l.Kind != "deferred" && l.Kind != "scrollbars" {
			t.Errorf("a %s layer was recorded again for a new value", l.Kind)
		}
	}
	// A frame in which the value did not change does no work.
	s0 = tr.Stats()
	pct.Set(60)
	if tr.ApplyUpdates() && tr.Dirty() {
		tr.Frame()
		if d := work(s0, tr.Stats()); d.Paints != 0 {
			t.Errorf("an equal value repainted: %+v", d)
		}
	}
}
