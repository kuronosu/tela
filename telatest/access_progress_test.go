package telatest

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/kit"
)

// TestAccessProgressValueFn: the value of a kit.Progress with ValueFn
// (ADR-250) reaches the OS accessibility bridge when the Value changes:
// the progress bar node of the next update has the new aria-valuenow and
// aria-valuetext, and the component does not build.
func TestAccessProgressValueFn(t *testing.T) {
	var done *tela.Value[float64]
	builds := 0
	a := Mount(t, tela.C(func() tela.Node {
		builds++
		done = tela.UseValue(10.0)
		return h.Div(h.Class("w-[200px]"), kit.Progress{ValueFn: done.Get, AriaLabel: "Download"})
	}))
	a.accessOn()
	bar := func(from int) (platform.AccessNode, bool) {
		var last platform.AccessNode
		found := false
		for _, u := range a.win.AccessUpdates()[from:] {
			for _, n := range u.Nodes {
				if n.Kind == platform.AccessElement && n.Role == "progressbar" {
					last, found = n, true
				}
			}
		}
		return last, found
	}
	if n, ok := bar(0); !ok || n.Name != "Download" || n.Range.Now != 10 || !n.Range.HasNow || n.Value != "10%" {
		t.Fatalf("at the activation: progress bar %+v (found %v), want Download at 10, \"10%%\"", n, ok)
	}
	from, b0 := len(a.win.AccessUpdates()), builds
	done.Set(60)
	a.WaitFor("an update of the progress bar", func() bool { _, ok := bar(from); return ok })
	if n, _ := bar(from); n.Range.Now != 60 || n.Value != "60%" {
		t.Errorf("after the change: progress bar %+v, want 60 and \"60%%\"", n)
	}
	if builds != b0 {
		t.Errorf("the change built the screen %d times", builds-b0)
	}
}
