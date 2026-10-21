package core

import (
	"strings"
	"sync"
	"testing"
)

// zeroCard is a struct component for the check of h.If (CheckIf).
type zeroCard struct{}

func (zeroCard) Build() Node { return Text("zero") }

// TestDevChecksFollowTheTreeBeingBuilt: the dev checks of node
// constructors (Role, Dir, Type, h.If's CheckIf) follow the mode of the
// Tree whose build calls them, and the process default outside builds
// (telatest.NoDev must test what a release build does).
func TestDevChecksFollowTheTreeBeingBuilt(t *testing.T) {
	defer devAny.Store(devAny.Load())
	bad := func() Node {
		CheckIf[zeroCard]()
		return div(Role("buton"), Dir("sideways"), input(FieldType("emial")))
	}
	for _, def := range []bool{true, false} {
		devAny.Store(def)
		if msg := panicMessage(func() { newTestTree(t, 100, 100, Component(bad), nil) }); msg != "" {
			t.Errorf("default %v: a tree out of dev mode panicked: %s", def, msg)
		}
		if msg := panicMessage(func() { devTree(t, Component(bad), nil) }); !strings.Contains(msg, "If(cond, zeroCard{...})") {
			t.Errorf("default %v: dev tree: panic %q, want CheckIf's", def, msg)
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
