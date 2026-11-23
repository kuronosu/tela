package core

import (
	"fmt"
	"sort"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
)

// The runs of UseAsync in flight (ADR-298, ADR-307). A run is on its own
// goroutine, so nothing in the tree says it is coming: a headless capture
// (tela snapshot, telatest) that settles without looking for it shows the
// loading state. The tree keeps the slots that have a run in flight, to
// name them (Pending, AsyncRuns) and for the entry points that wait for
// them (AwaitAsync, telatest's settle).

// asyncRun is a UseAsync slot with a run in flight.
type asyncRun interface {
	// describeAsync names the slot: its component and, in dev mode, where
	// UseAsync is called. UI thread only.
	describeAsync() string
}

// asyncStarted records that r has a run in flight.
func (t *Tree) asyncStarted(r asyncRun) {
	t.asyncMu.Lock()
	if t.asyncRuns == nil {
		t.asyncRuns = map[asyncRun]struct{}{}
	}
	t.asyncRuns[r] = struct{}{}
	t.asyncMu.Unlock()
}

// asyncEnded records that r has no run in flight any more: it ended, was
// cancelled or superseded, or the component unmounted.
func (t *Tree) asyncEnded(r asyncRun) {
	t.asyncMu.Lock()
	delete(t.asyncRuns, r)
	t.asyncMu.Unlock()
}

// AsyncInFlight reports whether a UseAsync run is in flight in the tree.
// Safe from any goroutine.
func (t *Tree) AsyncInFlight() bool {
	t.asyncMu.Lock()
	defer t.asyncMu.Unlock()
	return len(t.asyncRuns) > 0
}

// AsyncRuns names the UseAsync runs in flight, sorted, one per slot: "a
// UseAsync run in <Notes> (called at library.go:41)". UI thread only.
func (t *Tree) AsyncRuns() []string {
	t.asyncMu.Lock()
	runs := make([]asyncRun, 0, len(t.asyncRuns))
	for r := range t.asyncRuns {
		runs = append(runs, r)
	}
	t.asyncMu.Unlock()
	names := make([]string, len(runs))
	for i, r := range runs {
		names[i] = r.describeAsync()
	}
	sort.Strings(names)
	return names
}

// AsyncPollInterval is how often the entry points that wait for UseAsync
// runs look again (a run's result also wakes them).
const AsyncPollInterval = 2 * time.Millisecond

// AwaitAsync is the settle of a headless capture without an event loop
// (Render, telatest's Render, AriaSnapshot and LayoutJSON): it starts the
// UseAsync runs the last frame left to start (the passive effects) and, if
// there are any, runs frames, waiting in real time for the runs in flight
// and for what their results change, until none is in flight and the tree
// is clean, or limit passes, when a dev warning names the runs left (the
// capture shows their loading state). It returns the display list of the
// last frame it ran, nil when it ran none.
func (t *Tree) AwaitAsync(limit time.Duration) (dl *gfx.DisplayList) {
	deadline := time.Now().Add(limit)
	woke := make(chan struct{}, 1)
	t.SetWake(func() {
		select {
		case woke <- struct{}{}:
		default:
		}
	})
	defer t.SetWake(nil)
	t.FlushEffects() // starts the runs of the last frame's UseAsync
	if !t.AsyncInFlight() {
		// Nothing to wait for: the render is what its frames made it (the
		// state that passive effects set is rendered only when the tree
		// needs further frames, as Render documents).
		return nil
	}
	for frames := 0; ; {
		if t.Dirty() && frames < 1000 {
			dl, _ = t.Frame()
			frames++
			t.FlushEffects() // runs started by what the frame mounted
			continue
		}
		if !t.AsyncInFlight() {
			return dl
		}
		left := time.Until(deadline)
		if left <= 0 {
			for _, r := range t.AsyncRuns() {
				t.warnOnce("async-render:"+r, func() string { return asyncRenderWarning(r, limit) })
			}
			return dl
		}
		select {
		case <-woke:
		case <-time.After(min(left, AsyncPollInterval)):
		}
	}
}

// asyncRenderWarning is the dev warning of a capture that gave up on a run.
func asyncRenderWarning(run string, limit time.Duration) string {
	return fmt.Sprintf("%s was still in flight after %v: the capture shows its loading state. "+
		"A function that never returns, or waits on the network, keeps the screen loading; "+
		"make it return when its context is done, or give the preview data that does not load", run, limit)
}
