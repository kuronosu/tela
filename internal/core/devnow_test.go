package core

import (
	"strings"
	"sync"
	"testing"
)

// TestDevChecksFollowTheTreeBeingBuilt: the dev checks of node
// constructors (Role, Dir, Type) follow the mode of the
// Tree whose build calls them, and the process default outside builds
// (telatest.NoDev must test what a release build does).
func TestDevChecksFollowTheTreeBeingBuilt(t *testing.T) {
	defer devAny.Store(devAny.Load())
	bad := func() Node {
		return div(Role("buton"), Dir("sideways"), input(FieldType("emial")))
	}
	for _, def := range []bool{true, false} {
		devAny.Store(def)
		if msg := panicMessage(func() { newTestTree(t, 100, 100, Component(bad), nil) }); msg != "" {
			t.Errorf("default %v: a tree out of dev mode panicked: %s", def, msg)
		}
		if msg := panicMessage(func() { devTree(t, Component(bad), nil) }); !strings.Contains(msg, "buton") {
			t.Errorf("default %v: dev tree: panic %q, want the role's", def, msg)
		}
		// A dev tree turns the process default on (ADR-085).
		devAny.Store(def)
		// Updates are builds too.
		tr := newTestTree(t, 100, 100, Component(func() Node { return Text("ok") }), nil)
		if msg := panicMessage(func() { tr.Mount(Component(bad)); tr.Frame() }); msg != "" {
			t.Errorf("default %v: an update out of dev mode panicked: %s", def, msg)
		}
	}
	// Outside any build, the process default decides.
	devAny.Store(true)
	if msg := panicMessage(func() { Role("buton") }); !strings.Contains(msg, `unknown ARIA role "buton"`) {
		t.Errorf("outside a build, default on: panic %q", msg)
	}
	devAny.Store(false)
	if msg := panicMessage(func() { Role("buton") }); msg != "" {
		t.Errorf("outside a build, default off: panicked: %s", msg)
	}
}

// TestDevNowConcurrentTrees: Trees of both modes building at once on two
// goroutines each see their own mode.
func TestDevNowConcurrentTrees(t *testing.T) {
	defer devAny.Store(devAny.Load())
	devAny.Store(true)
	var wg sync.WaitGroup
	errs := make(chan string, 2)
	for _, dev := range []bool{true, false} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wrong := 0
			probe := func() Node {
				if devNow() != dev {
					wrong++
				}
				return Text("x")
			}
			tr := NewTree(Config{Fonts: testFonts, Dev: dev, Log: func(string) {}})
			for range 300 {
				tr.Mount(Component(probe))
			}
			if wrong > 0 {
				errs <- "a tree saw the other mode"
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

// TestSetThemeKeepsWarningDedup: a new theme reports the class errors
// again (they may differ under it), but not the warnings given once per
// call site (one mechanism, Tree.once, with class errors on their own).
func TestSetThemeKeepsWarningDedup(t *testing.T) {
	var logs []string
	f := func() {}
	comp := func() Node {
		UseEffect(func() func() { return nil }, f) // a func dependency: warned once
		return div(Class("flex-colum"), Text("x"))
	}
	count := func(sub string) int {
		n := 0
		for _, l := range logs {
			if strings.Contains(l, sub) {
				n++
			}
		}
		return n
	}
	tr := devTree(t, Component(comp), &logs)
	for range 3 {
		tr.Frame()
		tr.FlushEffects()
		tr.Mount(Component(comp))
	}
	tr.Frame()
	if d, c := count("cannot be compared"), count("flex-colum"); d != 1 || c != 1 {
		t.Fatalf("%d dependency warnings and %d class errors, want 1 and 1:\n%s", d, c, strings.Join(logs, "\n"))
	}
	tr.SetTheme(Theme{})
	tr.Mount(Component(comp))
	tr.Frame()
	tr.FlushEffects()
	if d, c := count("cannot be compared"), count("flex-colum"); d != 1 || c != 2 {
		t.Errorf("after SetTheme: %d dependency warnings and %d class errors, want 1 (not again) and 2:\n%s", d, c, strings.Join(logs, "\n"))
	}
}
