package headless

import (
	"fmt"

	"github.com/kuronosu/tela/internal/platform"
)

// Accessibility (platform contract v5, ADR-246).
//
// Each window has a fake accessibility bridge. A test starts and stops an
// assistive technology with Inject(platform.AccessibilityEvent{Active:
// ...}) (events alternate, starting with Active true) and sends its
// actions with Inject(platform.AccessActionEvent{...}) while one is
// active. SetAccessibility records every update the app sends while the
// bridge is active (AccessUpdates) and applies it to the window's
// platform.AccessTree (AccessTree), as a real bridge would; an update the
// tree cannot take (a child that is not known, the first update after
// activation without every node) panics with the reason. Updates sent
// while the bridge is not active are counted (AccessIgnored) and dropped,
// as every backend drops them. Activation starts from an empty tree.

// accessState is a window's fake bridge; guarded by b.mu.
type accessState struct {
	// queued is the state of the last AccessibilityEvent injected,
	// active that of the last one delivered.
	queued, active bool
	updates        []platform.AccessUpdate
	tree           *platform.AccessTree
	ignored        int
}

// injectAccess validates and queues an AccessibilityEvent or an
// AccessActionEvent; see Inject.
func (w *Window) injectAccess(e platform.Event) {
	b := w.b
	b.mu.Lock()
	if w.WindowState.Closed() {
		b.mu.Unlock()
		return
	}
	switch ev := e.(type) {
	case platform.AccessibilityEvent:
		if ev.Active == w.acc.queued {
			b.mu.Unlock()
			panic(fmt.Sprintf("headless: Inject(AccessibilityEvent{Active: %v}) while the bridge is already so; the events alternate, starting with Active true", ev.Active))
		}
		w.acc.queued = ev.Active
	case platform.AccessActionEvent:
		problem := platform.CheckAccessActionEvent(ev)
		if problem == "" && !w.acc.queued {
			problem = "no assistive technology is active; inject AccessibilityEvent{Active: true} first"
		}
		if problem != "" {
			b.mu.Unlock()
			panic(fmt.Sprintf("headless: Inject(AccessActionEvent): %s", problem))
		}
	}
	b.loop.Enqueue(w, e)
	b.mu.Unlock()
	b.signal()
}

// accessDelivered notes the delivery of an AccessibilityEvent: the bridge
// takes updates from now on, from an empty tree, or drops its tree.
func (w *Window) accessDelivered(active bool) {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	w.acc.active = active
	w.acc.tree = nil
}

// SetAccessibility implements platform.Window: see the top of access.go.
func (w *Window) SetAccessibility(update platform.AccessUpdate) {
	platform.CheckAccessUpdate(&update)
	b := w.b
	b.mu.Lock()
	if w.WindowState.Closed() {
		b.mu.Unlock()
		return
	}
	if !w.acc.active {
		w.acc.ignored++
		b.mu.Unlock()
		return
	}
	u := update.Clone()
	tree, _, err := w.acc.tree.Apply(&u)
	if err != nil {
		b.mu.Unlock()
		panic(fmt.Sprintf("headless: SetAccessibility: update %d after the activation: %v", len(w.acc.updates)+1, err))
	}
	w.acc.tree = tree
	w.acc.updates = append(w.acc.updates, u)
	b.mu.Unlock()
}

// Accessible reports whether an injected assistive technology is active
// on the window (the last AccessibilityEvent delivered had Active true).
func (w *Window) Accessible() bool {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.acc.active
}

// AccessUpdates returns copies of the accessibility updates the window
// received while its bridge was active, oldest first, across
// activations.
func (w *Window) AccessUpdates() []platform.AccessUpdate {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	out := make([]platform.AccessUpdate, len(w.acc.updates))
	for i, u := range w.acc.updates {
		out[i] = u.Clone()
	}
	return out
}

// AccessTree returns the window's tree as its bridge holds it: the
// updates since the latest activation applied; nil before the first one
// and while no assistive technology is active. The tree is immutable.
func (w *Window) AccessTree() *platform.AccessTree {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.acc.tree
}

// AccessIgnored returns the number of SetAccessibility calls made while
// the window's bridge was not active (dropped, as every backend does).
func (w *Window) AccessIgnored() int {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.acc.ignored
}
