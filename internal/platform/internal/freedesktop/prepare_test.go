//go:build linux

package freedesktop

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// Portal requests whose identifiers come later (Options.Prepare, the
// Wayland backend's xdg-foreign export and activation token; ADR-233).

// fakePrepare is an Options.Prepare that answers only when the test
// releases it, like a compositor round trip: "wayland:h1" for a window,
// "tok1" when a token is asked.
type fakePrepare struct {
	asked   []bool // the token argument of each call
	pending []func()
}

func (f *fakePrepare) prepare(w platform.Window, token bool, done func(parent, token string)) {
	f.asked = append(f.asked, token)
	f.pending = append(f.pending, func() {
		parent, tok := "", ""
		if w != nil {
			parent = "wayland:h1"
		}
		if token {
			tok = "tok1"
		}
		done(parent, tok)
	})
}

// release answers every waiting call (on the test goroutine, the
// harness's UI thread).
func (f *fakePrepare) release() {
	p := f.pending
	f.pending = nil
	for _, fn := range p {
		fn()
	}
}

func optionString(c portalCall, key string) string {
	s, _ := c.Options[key].Value().(string)
	return s
}

func TestPreparedRequests(t *testing.T) {
	addr := privateBus(t)
	p := startPortal(t, addr, 4)
	h := newHarness(t, addr)
	f := &fakePrepare{}
	h.d.opts.Prepare = f.prepare

	// OpenURI waits for the parent and the token, then passes both.
	if err := h.d.OpenURI(fakeWindow{}, "https://example.com/"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if n := len(p.callsSoFar()); n != 0 {
		t.Fatalf("%d portal calls before the identifiers came", n)
	}
	if !slices.Equal(f.asked, []bool{true}) {
		t.Errorf("Prepare token arguments = %v, want [true]", f.asked)
	}
	f.release()
	eventually(t, "OpenURI", func() bool { return len(p.callsSoFar()) == 1 })
	if c := p.callsSoFar()[0]; c.Method != "OpenURI" || c.Parent != "wayland:h1" || optionString(c, "activation_token") != "tok1" {
		t.Errorf("OpenURI call %+v, want parent wayland:h1 and activation_token tok1", c)
	}

	// OpenPath without a window: no parent, still a token.
	file := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := h.d.OpenPath(nil, file); err != nil {
		t.Fatal(err)
	}
	f.release()
	eventually(t, "OpenFile", func() bool { return len(p.callsSoFar()) == 2 })
	if c := p.callsSoFar()[1]; c.Method != "OpenFile" || c.Parent != "" || optionString(c, "activation_token") != "tok1" {
		t.Errorf("OpenFile call %+v, want no parent and activation_token tok1", c)
	}

	// A file dialog asks for no token, waits for its parent and is modal
	// to it.
	p.setResponder(false, func(portalCall) (uint32, []string) { return 0, []string{pathURI("/tmp/x.png")} })
	var got []string
	answered := false
	err := h.d.OpenFiles(platform.FileDialogOptions{Parent: fakeWindow{}, Title: "Pick"}, func(paths []string, err error) {
		if err != nil {
			t.Errorf("OpenFiles: %v", err)
		}
		got, answered = paths, true
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.asked, []bool{true, true, false}) {
		t.Errorf("Prepare token arguments = %v, want [true true false]", f.asked)
	}
	f.release()
	h.pump("the OpenFiles answer", func() bool { return answered })
	if !slices.Equal(got, []string{"/tmp/x.png"}) {
		t.Errorf("paths %q", got)
	}
	c := p.callsSoFar()[2]
	if modal, _ := c.Options["modal"].Value().(bool); c.Method != "OpenFile" || c.Parent != "wayland:h1" || !modal {
		t.Errorf("OpenFiles call %+v, want parent wayland:h1, modal", c)
	}
	if _, ok := c.Options["activation_token"]; ok {
		t.Errorf("OpenFiles call has an activation_token: %+v", c.Options)
	}

	// A parent window without an identifier (not on screen) is not modal.
	h.d.opts.Prepare = func(_ platform.Window, _ bool, done func(string, string)) { done("", "") }
	answered = false
	if err := h.d.SaveFile(platform.FileDialogOptions{Parent: fakeWindow{}}, func(string, error) { answered = true }); err != nil {
		t.Fatal(err)
	}
	h.pump("the SaveFile answer", func() bool { return answered })
	if c := p.callsSoFar()[3]; c.Parent != "" || c.Options["modal"].Value() != false {
		t.Errorf("SaveFile call %+v, want no parent, not modal", c)
	}

	// A request still waiting when the Desktop closes is never sent.
	h.d.opts.Prepare = f.prepare
	if err := h.d.OpenPath(fakeWindow{}, file); err != nil {
		t.Fatal(err)
	}
	h.d.Close()
	f.release()
	time.Sleep(30 * time.Millisecond)
	if n := len(p.callsSoFar()); n != 4 {
		t.Errorf("%d portal calls, want 4: a request was sent after Close", n)
	}
}

// The xdg-open fallback gets the token in its environment, never the
// token this process was started with.
func TestOpenerToken(t *testing.T) {
	addr := privateBus(t) // no portal
	h := newHarness(t, addr)
	t.Setenv("XDG_ACTIVATION_TOKEN", "stale")
	t.Setenv("DESKTOP_STARTUP_ID", "stale")
	dir := t.TempDir()
	out := filepath.Join(dir, "env")
	script := filepath.Join(dir, "env-opener")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s|%s|%s' \"$1\" \"$XDG_ACTIVATION_TOKEN\" \"$DESKTOP_STARTUP_ID\" > "+out+".tmp && mv "+out+".tmp "+out+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h.d.opts.Opener = script
	read := func(want string) {
		t.Helper()
		var got []byte
		eventually(t, "the opener", func() bool {
			var err error
			got, err = os.ReadFile(out)
			return err == nil
		})
		if string(got) != want {
			t.Errorf("the opener got %q, want %q", got, want)
		}
		os.Remove(out)
	}

	h.d.opts.Prepare = func(_ platform.Window, token bool, done func(string, string)) {
		if !token {
			t.Error("the opener's Prepare asked no token")
		}
		done("", "tok2")
	}
	if err := h.d.OpenURI(nil, "https://example.com/"); err != nil {
		t.Fatal(err)
	}
	read("https://example.com/|tok2|tok2")

	h.d.opts.Prepare = nil // X11: no token
	if err := h.d.OpenURI(nil, "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	read("https://example.com/b||")

	// Prepare answering later: the opener starts then.
	f := &fakePrepare{}
	h.d.opts.Prepare = f.prepare
	if err := h.d.OpenURI(nil, "https://example.com/c"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := os.Stat(out); err == nil {
		t.Error("the opener ran before the token came")
	}
	f.release()
	read("https://example.com/c|tok1|tok1")
}

func TestOpenerEnv(t *testing.T) {
	env := []string{"A=1", "XDG_ACTIVATION_TOKEN=old", "B=2", "DESKTOP_STARTUP_ID=old"}
	if got := openerEnv(env, ""); !slices.Equal(got, []string{"A=1", "B=2"}) {
		t.Errorf("openerEnv without a token = %q", got)
	}
	if got := openerEnv(env, "t"); !slices.Equal(got, []string{"A=1", "B=2", "XDG_ACTIVATION_TOKEN=t", "DESKTOP_STARTUP_ID=t"}) {
		t.Errorf("openerEnv with a token = %q", got)
	}
}

func TestActivationTokenLifetime(t *testing.T) {
	d := New(Options{Wake: func() {}, Post: func(func()) {}, Event: func(platform.Event) {}})
	d.setActivationToken("a")
	if got := d.TakeActivationToken(); got != "a" {
		t.Errorf("TakeActivationToken = %q, want a", got)
	}
	if got := d.TakeActivationToken(); got != "" {
		t.Errorf("TakeActivationToken again = %q, want none", got)
	}
	d.setActivationToken("b")
	d.tokenAt = time.Now().Add(-activationTokenLifetime - time.Second)
	if got := d.TakeActivationToken(); got != "" {
		t.Errorf("TakeActivationToken of an old token = %q, want none", got)
	}
}

// BindShortcuts is parented to the focused window, once its identifier
// is known; requests made meanwhile join the batch.
func TestShortcutsParent(t *testing.T) {
	addr := privateBus(t)
	p := startPortal13(t, addr, portal13Options{shortcuts: true, registry: true, dataDirs: []string{appEntries(t, "tela-test")}})
	h := newHarness(t, addr)
	f := &fakePrepare{}
	h.d.opts.Prepare = f.prepare
	h.d.opts.Focused = func() platform.Window { return fakeWindow{} }

	if err := h.d.RegisterShortcut(shortcut("show", platform.ModCtrl, "k")); err != nil {
		t.Fatal(err)
	}
	h.pump("the parent request", func() bool { return len(f.pending) > 0 })
	if err := h.d.RegisterShortcut(shortcut("hide", platform.ModCtrl, "j")); err != nil {
		t.Fatal(err)
	}
	h.d.Dispatch() // no second request while the first waits
	if len(f.pending) != 1 || f.asked[0] {
		t.Fatalf("Prepare calls %v (pending %d), want one without a token", f.asked, len(f.pending))
	}
	f.release()
	for range 2 {
		if se := h.status("bound"); se.Status != platform.ShortcutBound {
			t.Errorf("status %+v", se)
		}
	}
	p.mu.Lock()
	parents, binds := slices.Clone(p.parents), len(p.binds)
	p.mu.Unlock()
	if binds != 1 || !slices.Equal(parents, []string{"wayland:h1"}) {
		t.Errorf("BindShortcuts parents = %q in %d calls, want one with wayland:h1", parents, binds)
	}
}

// An opener that cannot be started after OpenURI returned (a Prepare that
// answers later, or a portal that refused the URI) is reported, since the
// contract has no error path then.
func TestOpenerFailsLater(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad-opener")
	if err := os.WriteFile(bad, []byte{0x7f, 'E', 'L', 'F', 0, 1}, 0o755); err != nil { // not a program: exec fails
		t.Fatal(err)
	}

	// No portal; the token comes later.
	h := newHarness(t, privateBus(t))
	h.d.opts.Opener = bad
	f := &fakePrepare{}
	h.d.opts.Prepare = f.prepare
	if err := h.d.OpenURI(nil, "https://example.com/"); err != nil {
		t.Fatalf("OpenURI = %v, want nil (the opener starts later)", err)
	}
	if err := h.d.asyncErr(); err != nil {
		t.Fatalf("reported %v before the token came", err)
	}
	f.release()
	h.pump("the opener's failure", func() bool { return h.d.asyncErr() != nil })
	t.Logf("reported: %v", h.d.asyncErr())

	// At once: returned, not reported.
	h = newHarness(t, privateBus(t))
	h.d.opts.Opener = bad
	if err := h.d.OpenURI(nil, "https://example.com/"); err == nil {
		t.Error("OpenURI with an opener that cannot start at once = nil, want its error")
	}
	if err := h.d.asyncErr(); err != nil {
		t.Errorf("reported %v; it was returned", err)
	}

	// A portal that refuses: the fallback runs on a godbus goroutine.
	addr := privateBus(t)
	p := startPortal(t, addr, 4)
	p.mu.Lock()
	p.refuseOpens = true
	p.mu.Unlock()
	h = newHarness(t, addr)
	h.d.opts.Opener = bad
	if err := h.d.OpenURI(nil, "https://example.com/"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the fallback's failure", func() bool { return h.d.asyncErr() != nil })
}
