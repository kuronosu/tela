package tela_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// probe records what a component saw.
type probe struct {
	mu     sync.Mutex
	states []tela.Async[string]
}

func (p *probe) add(a tela.Async[string]) {
	p.mu.Lock()
	p.states = append(p.states, a)
	p.mu.Unlock()
}

func (p *probe) first() tela.Async[string] {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.states[0]
}

// show renders an Async[string] as a paragraph.
func show(a tela.Async[string]) tela.Node {
	switch {
	case a.Err != nil:
		return P(Textf("error: %v (data %q)", a.Err, a.Data))
	case a.Loading:
		return P(Textf("loading (data %q)", a.Data))
	}
	return P(Textf("data: %s", a.Data))
}

func waitDone(t *testing.T, ctx context.Context) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the context was not cancelled")
	}
}

func TestUseAsyncLoadingThenData(t *testing.T) {
	release := make(chan struct{})
	var p probe
	page := func() tela.Node {
		a := tela.UseAsync(func(ctx context.Context) (string, error) {
			<-release
			return "ready", nil
		})
		p.add(a)
		return show(a)
	}
	app := telatest.Mount(t, tela.C(page))
	if f := p.first(); !f.Loading || f.Data != "" || f.Err != nil {
		t.Fatalf("the first build saw %+v, want Loading with no data", f)
	}
	app.ExpectText(`loading (data "")`)
	close(release)
	app.ExpectText("data: ready")
	app.ExpectNoText("loading")
}

func TestUseAsyncError(t *testing.T) {
	boom := errors.New("boom")
	page := func() tela.Node {
		return show(tela.UseAsync(func(ctx context.Context) (string, error) { return "", boom }))
	}
	app := telatest.Mount(t, tela.C(page))
	app.ExpectText(`error: boom (data "")`)
}

func TestUseAsyncDepsChangeCancelsAndDropsLateResult(t *testing.T) {
	releaseOld := make(chan struct{})
	ctxs := make(chan context.Context, 4)
	page := func() tela.Node {
		id, setID := tela.UseState(1)
		a := tela.UseAsync(func(ctx context.Context) (string, error) {
			ctxs <- ctx
			if id == 1 {
				<-releaseOld // ignores ctx: returns late with a result
				return "old", nil
			}
			return "new", nil
		}, id)
		return Div(Button(OnClick(func(tela.MouseEvent) { setID.Set(2) }), Text("Next")), show(a))
	}
	app := telatest.Mount(t, tela.C(page))
	first := <-ctxs
	app.ExpectText("loading")
	app.Click(telatest.ByRole("button", "Next"))
	waitDone(t, first)
	app.ExpectText("data: new")
	close(releaseOld) // the old run returns now; its result must be dropped
	time.Sleep(20 * time.Millisecond)
	app.ExpectText("data: new")
	app.ExpectNoText("old")
}

func TestUseAsyncDepsChangeResetsData(t *testing.T) {
	hold := make(chan struct{})
	page := func() tela.Node {
		id, setID := tela.UseState(1)
		a := tela.UseAsync(func(ctx context.Context) (string, error) {
			if id == 2 {
				<-hold
			}
			return "v" + string(rune('0'+id)), nil
		}, id)
		return Div(Button(OnClick(func(tela.MouseEvent) { setID.Set(2) }), Text("Next")), show(a))
	}
	app := telatest.Mount(t, tela.C(page))
	app.ExpectText("data: v1")
	app.Click(telatest.ByRole("button", "Next"))
	app.ExpectText(`loading (data "")`) // reset, not stale
	close(hold)
	app.ExpectText("data: v2")
}

func TestUseAsyncReloadKeepsData(t *testing.T) {
	var runs int
	var mu sync.Mutex
	hold := make(chan struct{})
	var reload func()
	page := func() tela.Node {
		a := tela.UseAsync(func(ctx context.Context) (string, error) {
			mu.Lock()
			runs++
			n := runs
			mu.Unlock()
			if n == 2 {
				<-hold
			}
			return "run" + string(rune('0'+n)), nil
		})
		reload = a.Reload
		return show(a)
	}
	app := telatest.Mount(t, tela.C(page))
	app.ExpectText("data: run1")
	go reload() // safe from any goroutine
	app.ExpectText(`loading (data "run1")`)
	close(hold)
	app.ExpectText("data: run2")
}

func TestUseAsyncReloadErrorKeepsData(t *testing.T) {
	var n int
	var mu sync.Mutex
	var reload func()
	page := func() tela.Node {
		a := tela.UseAsync(func(ctx context.Context) (string, error) {
			mu.Lock()
			n++
			k := n
			mu.Unlock()
			if k == 2 {
				return "", errors.New("down")
			}
			return "fresh", nil
		})
		reload = a.Reload
		return show(a)
	}
	app := telatest.Mount(t, tela.C(page))
	app.ExpectText("data: fresh")
	reload()
	app.ExpectText(`error: down (data "fresh")`)
	reload()
	app.ExpectText("data: fresh")
}

func TestUseAsyncReloadDropsSupersededResult(t *testing.T) {
	var n int
	var mu sync.Mutex
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	var reload func()
	page := func() tela.Node {
		a := tela.UseAsync(func(ctx context.Context) (string, error) {
			mu.Lock()
			n++
			k := n
			mu.Unlock()
			if k == 1 {
				started <- struct{}{}
				<-release
				return "first", nil
			}
			return "second", nil
		})
		reload = a.Reload
		return show(a)
	}
	app := telatest.Mount(t, tela.C(page))
	app.ExpectText("loading")
	<-started // the first run is the one in flight
	reload()
	app.ExpectText("data: second")
	close(release)
	time.Sleep(20 * time.Millisecond)
	app.ExpectText("data: second")
}

func TestUseAsyncUnmountCancels(t *testing.T) {
	ctxs := make(chan context.Context, 1)
	var reload func()
	child := func() tela.Node {
		a := tela.UseAsync(func(ctx context.Context) (string, error) {
			ctxs <- ctx
			<-ctx.Done()
			return "late", nil
		})
		reload = a.Reload
		return show(a)
	}
	page := func() tela.Node {
		shown, setShown := tela.UseState(true)
		return Div(Button(OnClick(func(tela.MouseEvent) { setShown.Set(false) }), Text("Hide")),
			If(shown, tela.C(child)))
	}
	app := telatest.Mount(t, tela.C(page))
	ctx := <-ctxs
	app.Click(telatest.ByRole("button", "Hide"))
	waitDone(t, ctx)
	reload() // after unmount: nothing runs, nothing panics
	app.ExpectNoText("loading")
}

// AsyncCard holds an Async: it skips its build while the value is equal.
type AsyncCard struct {
	Data   tela.Async[string]
	builds *int
}

func (c AsyncCard) Build() tela.Node {
	*c.builds++
	return show(c.Data)
}

func TestUseAsyncSkipsStructComponents(t *testing.T) {
	var builds int
	page := func() tela.Node {
		clicks, setClicks := tela.UseState(0)
		a := tela.UseAsync(func(ctx context.Context) (string, error) { return "ok", nil })
		return Div(Button(OnClick(func(tela.MouseEvent) { setClicks.Set(clicks + 1) }), Textf("Clicked %d", clicks)),
			AsyncCard{Data: a, builds: &builds})
	}
	app := telatest.Mount(t, tela.C(page))
	app.ExpectText("data: ok")
	before := builds
	app.Click(telatest.ByRole("button", "Clicked 0"))
	app.Click(telatest.ByRole("button", "Clicked 1"))
	app.ExpectText("Clicked 2")
	if builds != before {
		t.Errorf("the card built %d times after two unrelated clicks, want it skipped", builds-before)
	}
}

func TestUseAsyncNoDepsRunsOnce(t *testing.T) {
	var runs int
	var mu sync.Mutex
	page := func() tela.Node {
		clicks, setClicks := tela.UseState(0)
		a := tela.UseAsync(func(ctx context.Context) (string, error) {
			mu.Lock()
			runs++
			mu.Unlock()
			return "x", nil
		})
		return Div(Button(OnClick(func(tela.MouseEvent) { setClicks.Set(clicks + 1) }), Textf("Clicked %d", clicks)), show(a))
	}
	app := telatest.Mount(t, tela.C(page))
	app.ExpectText("data: x")
	app.Click(telatest.ByRole("button", "Clicked 0"))
	app.Click(telatest.ByRole("button", "Clicked 1"))
	app.ExpectText("Clicked 2")
	mu.Lock()
	defer mu.Unlock()
	if runs != 1 {
		t.Errorf("f ran %d times, want 1", runs)
	}
}

func TestUseAsyncNilAndZero(t *testing.T) {
	nilF := func() tela.Node { tela.UseAsync[int](nil); return nil }
	if msg := panicOf(func() { mount(t, tela.C(nilF), nil) }); !strings.Contains(msg, "UseAsync(nil") {
		t.Errorf("nil f: %q", msg)
	}
	if msg := panicOf(func() { tela.Async[int]{}.Reload() }); !strings.Contains(msg, "zero Async") {
		t.Errorf("zero Reload: %q", msg)
	}
}

func TestUseDebounced(t *testing.T) {
	page := func() tela.Node {
		q, setQ := tela.UseState("")
		settled := tela.UseDebounced(q, 300*time.Millisecond)
		return Div(
			Button(OnClick(func(tela.MouseEvent) { setQ.Set(q + "a") }), Text("Type")),
			P(Textf("typed %d settled %d", len(q), len(settled))))
	}
	app := telatest.Mount(t, tela.C(page))
	app.ExpectText("typed 0 settled 0")
	app.ExpectIdle()
	app.Click(telatest.ByRole("button", "Type"))
	app.Advance(200 * time.Millisecond)
	app.Click(telatest.ByRole("button", "Type")) // restarts the wait
	app.Advance(200 * time.Millisecond)
	app.ExpectText("typed 2 settled 0")
	if p := strings.Join(app.Pending(), ";"); !strings.Contains(p, "UseTimeout") {
		t.Errorf("Pending = %q, want it to name the UseTimeout of the wait", p)
	}
	app.Advance(150 * time.Millisecond)
	app.ExpectText("typed 2 settled 2")
	app.ExpectIdle()
}

func TestUseDebouncedBackToSettled(t *testing.T) {
	page := func() tela.Node {
		on, setOn := tela.UseState(false)
		settled := tela.UseDebounced(on, 300*time.Millisecond)
		return Div(Button(OnClick(func(tela.MouseEvent) { setOn.Set(!on) }), Text("Toggle")),
			P(Textf("on %v settled %v", on, settled)))
	}
	app := telatest.Mount(t, tela.C(page))
	app.Click(telatest.ByRole("button", "Toggle"))
	app.Click(telatest.ByRole("button", "Toggle")) // back to the settled value
	app.ExpectText("on false settled false")
	app.ExpectIdle()
}

func TestUseDebouncedZeroDuration(t *testing.T) {
	page := func() tela.Node {
		n, setN := tela.UseState(0)
		return Div(Button(OnClick(func(tela.MouseEvent) { setN.Set(n + 1) }), Text("Inc")),
			P(Textf("n %d", tela.UseDebounced(n, 0))))
	}
	app := telatest.Mount(t, tela.C(page))
	app.Click(telatest.ByRole("button", "Inc"))
	app.ExpectText("n 1")
	app.ExpectIdle()
}
