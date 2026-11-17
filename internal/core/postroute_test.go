package core

import (
	"context"
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// uiThread is a goroutine that runs the functions it is given, one at a
// time: the UI thread of a Tree in a test, as each parallel test's
// goroutine is its app's.
type uiThread struct {
	do   chan func()
	done chan struct{}
}

func newUIThread(t *testing.T) *uiThread {
	u := &uiThread{do: make(chan func()), done: make(chan struct{})}
	go func() {
		for f := range u.do {
			f()
			u.done <- struct{}{}
		}
	}()
	t.Cleanup(func() { close(u.do) })
	return u
}

// run runs f on the goroutine and waits for it.
func (u *uiThread) run(f func()) {
	u.do <- f
	<-u.done
}

// TestPostGoesToTheCallersApp: while two Trees receive posts, each on its
// own UI thread (two tests in parallel), a post made on a Tree's UI thread
// goes to that Tree, a post of a UseGo goroutine to the app of its
// component, and a post from any other goroutine to the newest receiver,
// as before (ADR-292). The calls that look up the running app
// (RunningAppID here) are routed the same way.
func TestPostGoesToTheCallersApp(t *testing.T) {
	ga, gb := newUIThread(t), newUIThread(t)
	var a, b *Host
	var stopA, stopB func()
	posted := make(chan struct{})
	var ran []string // appended on the UI threads, which take turns (run)
	note := func(s string) func() { return func() { ran = append(ran, s) } }
	mk := func(id string, root Node) *Host {
		h, err := NewHost(headless.New(headless.Options{Size: platform.Size{W: 200, H: 100}}),
			Options{Config: Config{Fonts: testFonts}, AppID: id, Root: root})
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	ga.run(func() {
		a = mk("app.a", Component(func() Node {
			UseGo(func(ctx context.Context) {
				Post(note("A's goroutine"))
				close(posted)
			}, Once)
			return Text("a")
		}))
		stopA = a.tree.ReceivePosts()
	})
	gb.run(func() {
		b = mk("app.b", Text("b"))
		stopB = b.tree.ReceivePosts()
	})
	defer func() { ga.run(stopA); gb.run(stopB) }()

	ids := map[string]string{}
	ga.run(func() { Post(note("A's UI thread")); ids["a"], _ = RunningAppID() })
	gb.run(func() { Post(note("B's UI thread")); ids["b"], _ = RunningAppID() })
	Post(note("elsewhere"))
	ids["elsewhere"], _ = RunningAppID()
	ga.run(func() { a.tree.Frame(); a.tree.FlushEffects() }) // starts A's UseGo
	<-posted

	var gotA, gotB []string
	ga.run(func() { a.tree.ApplyUpdates(); gotA = slices.Clone(ran); ran = nil })
	gb.run(func() { b.tree.ApplyUpdates(); gotB = slices.Clone(ran); ran = nil })
	if want := []string{"A's UI thread", "A's goroutine"}; !slices.Equal(gotA, want) {
		t.Errorf("tree A ran %q, want %q", gotA, want)
	}
	if want := []string{"B's UI thread", "elsewhere"}; !slices.Equal(gotB, want) {
		t.Errorf("tree B ran %q, want %q", gotB, want)
	}
	if ids["a"] != "app.a" || ids["b"] != "app.b" || ids["elsewhere"] != "app.b" {
		t.Errorf("RunningAppID: %v, want app.a on A's UI thread and app.b on B's and elsewhere", ids)
	}

	// Once B stops, every post goes to A again.
	gb.run(stopB)
	Post(note("after B"))
	ga.run(func() { a.tree.ApplyUpdates(); gotA = slices.Clone(ran); ran = nil })
	if want := []string{"after B"}; !slices.Equal(gotA, want) {
		t.Errorf("after B stopped, tree A ran %q, want %q", gotA, want)
	}
}
