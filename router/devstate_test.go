//go:build teladev

package router_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/stdfonts"
	"github.com/kuronosu/tela/router"
)

// The history of a Router is kept by `tela dev` across reloads (ADR-113):
// it is hook state of exported plain fields. Run with
// go test -tags teladev ./router/.

func frames(tr *core.Tree) {
	for i := 0; i < 10; i++ {
		tr.Frame()
		tr.FlushEffects()
		if !tr.Dirty() {
			return
		}
	}
}

func TestDevStateKeepsTheHistory(t *testing.T) {
	var nav router.Navigator
	var loc router.Location
	root := tela.C(func() tela.Node {
		return router.Router{Children: tela.C(func() tela.Node {
			nav, loc = router.UseNavigate(), router.UseLocation()
			return nil
		})}
	})
	mount := func(seed []byte) *core.Tree {
		tr := core.NewTree(core.Config{Fonts: stdfonts.NewCollection(), Dev: true})
		tr.SetViewport(gfx.Size{W: 200, H: 100})
		if seed != nil {
			tr.SeedDevState(seed)
		}
		tr.Mount(root)
		frames(tr)
		return tr
	}

	tr := mount(nil)
	nav.To("/notes/3?x=1")
	frames(tr)
	nav.To("/about")
	frames(tr)
	nav.Back()
	frames(tr)
	if loc.String() != "/notes/3?x=1" {
		t.Fatalf("location %q before the reload", loc)
	}
	data, rep, err := tr.CaptureDevState()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rep.Reset {
		if !r.Silent {
			t.Errorf("a slot resets: %s #%d of %s: %s", r.Hook, r.Index, r.Component, r.Reason)
		}
	}
	tr.Unmount()

	tr2 := mount(data)
	defer tr2.Unmount()
	if sr := tr2.DevSeedReport(); sr.Err != "" || sr.Restored == 0 {
		t.Fatalf("seed report %+v", sr)
	}
	if loc.String() != "/notes/3?x=1" || !nav.CanGoBack() || !nav.CanGoForward() {
		t.Errorf("after the reload: location %q, back %v, forward %v", loc, nav.CanGoBack(), nav.CanGoForward())
	}
	nav.Forward()
	frames(tr2)
	if loc.Path != "/about" {
		t.Errorf("forward goes to %q, not to /about: the stack was not kept", loc)
	}
}
