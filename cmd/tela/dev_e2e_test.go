package main

import (
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/cmd/tela/internal/watch"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/platform/remote"
	"github.com/kuronosu/tela/internal/testutil/wine"
)

// e2eWait bounds each wait of the end-to-end test; the first build of an
// app compiles Tela with the teladev tag, which takes a while with a cold
// build cache.
const e2eWait = 3 * time.Minute

// e2e is a real tela dev (real go build, real watcher, real children) on a
// headless window, running testdata/devapp copied into a module of its own.
type e2e struct {
	t    *testing.T
	hb   *headless.Backend
	s    *supervisor
	out  *syncBuffer // the supervisor's lines and the children's output
	view *recordView
	done chan int
	src  string // the app's main.go in the temporary module
}

// devAppModule creates a module outside the repository that requires tela
// through a replace directive, with testdata/devapp as its package main
// (the outsideModule of outside_test.go, with an app).
func devAppModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gomod := fmt.Sprintf("module example.com/devapp\n\ngo 1.27\n\nrequire github.com/kuronosu/tela v0.0.0\n\nreplace github.com/kuronosu/tela => %s\n", filepath.ToSlash(repoRoot(t)))
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"main.go", "devstate_dev.go"} {
		b, err := os.ReadFile(filepath.Join("testdata", "devapp", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Resolve the requirements from the module cache, without the network.
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	tidy.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=-mod=mod", "GOPROXY=off", "GOSUMDB=off")
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Skipf("cannot resolve the dependencies of a module outside the repository offline (go mod tidy: %v):\n%s", err, out)
	}
	return dir
}

func startE2E(t *testing.T) *e2e {
	t.Helper()
	skipShort(t)
	wine.Skip(t, "no AF_UNIX sockets (WSAEAFNOSUPPORT), so the remote host cannot listen")
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("the go command is not in PATH")
	}
	dir := devAppModule(t)
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	opts := devOptions{Pkg: ".", Verbose: true, noTerminal: true}
	target, err := resolveDevTarget(goBin, opts)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	b, err := newGoBuilder(goBin, target, tmp)
	if err != nil {
		t.Fatal(err)
	}
	set := newWatchSet(tmp)
	deps, err := listDeps(goBin, target)
	if err != nil {
		t.Fatal(err)
	}
	set.setDeps(deps)
	w, err := watch.New(watch.Options{Filter: set.relevant, SkipDir: set.skipDir}, set.roots()...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })

	e := &e2e{
		t:    t,
		hb:   headless.New(headless.Options{RealTime: true}),
		out:  &syncBuffer{},
		view: &recordView{},
		done: make(chan int, 1),
		src:  filepath.Join(dir, "main.go"),
	}
	e.s = newSupervisor(supervisorConfig{
		opts:     opts,
		builder:  b,
		log:      e.out,
		stdout:   e.out,
		stderr:   e.out,
		env:      os.Environ(),
		watchSet: set,
		setRoots: func(roots []string) { w.SetRoots(roots) },
		refreshDeps: func() (depSet, error) {
			return listDeps(goBin, target)
		},
		view:              e.view,
		firstFrameTimeout: e2eWait,
	})
	e.view.term = terminalView{e.s.log}
	host, err := remote.NewHost(e.hb, e.s.hostOptions())
	if err != nil {
		t.Fatal(err)
	}
	e.s.attach(host, e.hb.Quit)
	go e.s.forward(w)
	go func() {
		err := host.Run()
		e.s.push(evHostDone{err: err})
	}()
	go func() { e.done <- e.s.run() }()
	t.Cleanup(func() {
		e.s.push(evKey{'q'})
		select {
		case <-e.done:
		case <-time.After(30 * time.Second):
			t.Error("tela dev did not quit")
		}
		host.Close()
		if t.Failed() || testing.Verbose() {
			t.Logf("output:\n%s", e.out.String())
		}
	})
	return e
}

// until polls cond until it holds or e2eWait passes.
func (e *e2e) until(what string, cond func() bool) {
	e.t.Helper()
	deadline := time.Now().Add(e2eWait)
	for !cond() {
		if time.Now().After(deadline) {
			e.t.Fatalf("timeout waiting for %s; output:\n%s", what, e.out.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// last returns the last line of the output after offset that starts with
// prefix ("" if none).
func (e *e2e) last(offset int, prefix string) string {
	out := e.out.String()
	if offset > len(out) {
		return ""
	}
	last := ""
	for _, l := range strings.Split(out[offset:], "\n") {
		if strings.HasPrefix(l, prefix) {
			last = l
		}
	}
	return last
}

// expect waits until the last line with prefix after offset is want.
func (e *e2e) expect(offset int, prefix, want string) {
	e.t.Helper()
	e.until(fmt.Sprintf("%q", want), func() bool { return e.last(offset, prefix) == want })
}

func (e *e2e) window() *headless.Window {
	e.t.Helper()
	var w *headless.Window
	e.until("the window", func() bool {
		if ws := e.hb.Windows(); len(ws) > 0 {
			w = ws[0]
		}
		return w != nil
	})
	return w
}

func (e *e2e) pixel(x, y int) color.RGBA {
	e.t.Helper()
	var c color.RGBA
	e.until("a frame", func() bool {
		f := e.window().Frame()
		if f == nil {
			return false
		}
		c = f.RGBAAt(x, y)
		return true
	})
	return c
}

func (e *e2e) click(x, y float32) {
	e.t.Helper()
	w := e.window()
	w.SetFocused(true)
	p := platform.Point{X: x, Y: y}
	w.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: p})
	w.Inject(platform.PointerEvent{Action: platform.PointerDown, Pos: p, Button: platform.ButtonPrimary, Buttons: 1})
	w.Inject(platform.PointerEvent{Action: platform.PointerUp, Pos: p, Button: platform.ButtonPrimary})
}

func (e *e2e) typeText(text string) {
	e.t.Helper()
	w := e.window()
	for _, r := range text {
		s := string(r)
		w.Inject(platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeUnidentified, Key: platform.Key(s), Text: s})
		w.Inject(platform.KeyEvent{Action: platform.KeyRelease, Code: platform.CodeUnidentified, Key: platform.Key(s)})
	}
}

func (e *e2e) scroll(x, y, dy float32) {
	e.t.Helper()
	w := e.window()
	p := platform.Point{X: x, Y: y}
	w.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: p})
	w.Inject(platform.ScrollEvent{Pos: p, Delta: platform.Point{Y: dy}})
}

// edit rewrites the app's source with f and waits for the reload it causes.
// It returns the time from the write to the window showing the new build,
// and the offset of the output where the new build's lines start.
func (e *e2e) edit(f func(src string) string) (time.Duration, int) {
	e.t.Helper()
	b, err := os.ReadFile(e.src)
	if err != nil {
		e.t.Fatal(err)
	}
	next := f(string(b))
	if next == string(b) {
		e.t.Fatal("the edit changed nothing")
	}
	reloads := strings.Count(e.out.String(), "reloaded in")
	offset := len(e.out.String())
	t0 := time.Now()
	if err := os.WriteFile(e.src, []byte(next), 0o644); err != nil {
		e.t.Fatal(err)
	}
	e.until("the reload", func() bool { return strings.Count(e.out.String(), "reloaded in") > reloads })
	return time.Since(t0), offset
}

func within(a, b color.RGBA, d int) bool {
	abs := func(x int) int {
		if x < 0 {
			return -x
		}
		return x
	}
	return abs(int(a.R)-int(b.R)) <= d && abs(int(a.G)-int(b.G)) <= d && abs(int(a.B)-int(b.B)) <= d
}

// TestDevEndToEnd runs tela dev on a real Tela app (testdata/devapp) on
// the headless backend: it clicks, types and scrolls through the host,
// edits the source and checks what each reload kept and reset (ADR-111,
// ADR-113, ADR-114, ADR-115).
func TestDevEndToEnd(t *testing.T) {
	e := startE2E(t)
	started := time.Now()
	e.expect(0, "devapp: counter", "devapp: counter n=0 pkg=0")
	e.until("the first frame", func() bool { return e.window().Frame() != nil })
	t.Logf("first build and start: %v", time.Since(started).Round(time.Millisecond))

	blue := e.pixel(5, 5)
	row := func(i int) color.RGBA { return color.RGBA{uint8(40 + i*7), uint8(200 - i*5), uint8(100 + i*3), 255} }
	if got := e.pixel(10, 212); !within(got, row(0), 1) {
		t.Fatalf("the scroll container shows %v at its top, want row 0 %v (is the layout as testdata/devapp says?)", got, row(0))
	}

	// Build up state: the counter and the package variable, a kept and an
	// unkeepable slot, a sidebar, typed text with focus, a scroll offset.
	e.click(60, 68)
	e.expect(0, "devapp: counter", "devapp: counter n=1 pkg=1")
	e.click(60, 68)
	e.expect(0, "devapp: counter", "devapp: counter n=2 pkg=2")
	e.click(60, 108)
	e.expect(0, "devapp: mixed", "devapp: mixed n=1 f=7")
	e.click(60, 148)
	e.expect(0, "devapp: sidebar", "devapp: sidebar n=1")
	e.click(60, 188)
	e.typeText("hi")
	e.expect(0, "devapp: input", `devapp: input "hi"`)
	e.scroll(60, 250, 64)
	e.until("the scrolled container", func() bool { return within(e.pixel(10, 212), row(2), 1) })

	// 1. Edit a class and a text: the new code shows, the state stays.
	took, mark := e.edit(func(s string) string {
		s = strings.Replace(s, "bg-blue-500", "bg-green-500", 1)
		return strings.Replace(s, `Text("Version A")`, `Text("Version B")`, 1)
	})
	t.Logf("save to swapped (class and text): %v", took.Round(time.Millisecond))
	e.until("the new header", func() bool { return e.pixel(5, 5) != blue })
	e.expect(mark, "devapp: counter", "devapp: counter n=2 pkg=2")
	e.expect(mark, "devapp: restore", "devapp: restore 2")
	e.expect(mark, "devapp: mixed", "devapp: mixed n=1 f=7")
	e.expect(mark, "devapp: sidebar", "devapp: sidebar n=1")
	if got := e.pixel(10, 212); !within(got, row(2), 1) {
		t.Errorf("the scroll offset was not kept: the container shows %v at its top, want row 2 %v", got, row(2))
	}
	e.expect(mark, "devapp: focus", "devapp: focus") // the window gained the focus the field kept
	e.typeText("!")                                  // the focus is kept, so the key goes to the field, whose text is kept
	e.expect(mark, "devapp: input", `devapp: input "hi!"`)
	e.until("the state report line", func() bool {
		return strings.Contains(e.out.String()[mark:], "(state: ") && strings.Contains(e.out.String()[mark:], " kept, 0 reset)")
	})
	if out := e.out.String()[mark:]; !strings.Contains(out, "holds a func") {
		t.Errorf("no warning that Mixed's func slot resets:\n%s", out)
	}

	// 2. Add a hook to Sidebar: it resets, the other components keep state.
	took, mark = e.edit(func(s string) string {
		return strings.Replace(s, "// HOOK", `tela.UseState("x")`, 1)
	})
	t.Logf("save to swapped (a hook added): %v", took.Round(time.Millisecond))
	e.expect(mark, "devapp: sidebar", "devapp: sidebar n=0")
	e.expect(mark, "devapp: counter", "devapp: counter n=2 pkg=2")
	e.expect(mark, "devapp: mixed", "devapp: mixed n=1 f=7")
	e.until("the report of the reset", func() bool { return strings.Contains(e.out.String()[mark:], " kept, 1 reset)") })
	if out := e.out.String()[mark:]; !strings.Contains(out, "state reset:") || !strings.Contains(out, "Sidebar") {
		t.Errorf("the child did not say which component reset:\n%s", out)
	}

	// 3. A compile error: the old build stays, the problem is reported.
	broken := func(s string) string { return s + "\nfunc broken( {\n" }
	b, _ := os.ReadFile(e.src)
	good := string(b)
	if err := os.WriteFile(e.src, []byte(broken(good)), 0o644); err != nil {
		t.Fatal(err)
	}
	e.until("the build error", func() bool {
		for _, l := range e.view.entries() {
			if strings.HasPrefix(l, "show 1: build failed") {
				return true
			}
		}
		return false
	})
	if last := e.view.lastProblem(); last.kind != problemBuild || !strings.Contains(last.text, "main.go") {
		t.Errorf("problem %+v, want the compiler output", last)
	}
	mark = len(e.out.String())
	e.click(60, 68) // the previous build still works
	e.expect(mark, "devapp: counter", "devapp: counter n=3 pkg=3")

	// 4. Fix it: the problem clears and the new build keeps the state.
	if err := os.WriteFile(e.src, []byte(good+"\n// fixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mark = len(e.out.String())
	e.until("the problem to clear", func() bool {
		es := e.view.entries()
		return len(es) > 0 && es[len(es)-1] == "clear"
	})
	e.expect(mark, "devapp: counter", "devapp: counter n=3 pkg=3")
	e.expect(mark, "devapp: restore", "devapp: restore 3")

	// 5. The directory of tela.Assets(os.DirFS(...)) is watched: the child
	// reported it, and a file under it reloads the app.
	assets := filepath.Join(filepath.Dir(e.src), "assets")
	e.until("the assets directory", func() bool { return strings.Contains(e.out.String(), "tela dev: assets: "+assets) })
	reloads := strings.Count(e.out.String(), "reloaded in")
	if err := os.WriteFile(filepath.Join(assets, "logo.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.until("the reload after an asset changed", func() bool { return strings.Count(e.out.String(), "reloaded in") > reloads })
}
