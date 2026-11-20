package router_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/router"
	"github.com/kuronosu/tela/telatest"
)

func panicText(f func()) (msg string) {
	defer func() { msg, _ = recover().(string) }()
	f()
	return ""
}

func TestZeroNavigatorPanicsWithTheFix(t *testing.T) {
	var n router.Navigator
	for name, f := range map[string]func(){
		"To": func() { n.To("/") }, "Replace": func() { n.Replace("/") },
		"Back": n.Back, "Forward": n.Forward,
	} {
		if msg := panicText(f); !strings.Contains(msg, "router.UseNavigate()") {
			t.Errorf("%s on the zero Navigator: %q", name, msg)
		}
	}
	if n.CanGoBack() || n.CanGoForward() {
		t.Error("the zero Navigator can go somewhere")
	}
}

func TestOutletOutsideARouteElementPanicsWithTheFix(t *testing.T) {
	page := func() tela.Node { return router.Router{Children: router.Outlet()} }
	var msg string
	func() {
		defer func() { msg, _ = recover().(string) }()
		telatest.Mount(t, tela.C(page), telatest.AllowDev("panic"))
	}()
	if !strings.Contains(msg, "Element of a router.Route") {
		t.Errorf("panic %q", msg)
	}
}

func TestEmptyRoutesPanicWithTheFix(t *testing.T) {
	if msg := panicText(func() { router.Routes() }); !strings.Contains(msg, "at least one router.Route") {
		t.Errorf("panic %q", msg)
	}
}

func TestParamsOutsideARouteAreEmpty(t *testing.T) {
	var got router.Params
	page := func() tela.Node {
		return router.Router{Children: tela.C(func() tela.Node { got = router.UseParams(); return nil })}
	}
	telatest.Mount(t, tela.C(page))
	if got == nil || len(got) != 0 {
		t.Errorf("params %v", got)
	}
}
