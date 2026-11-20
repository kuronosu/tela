//go:build teladev

package tela_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
)

// Run with go test -tags teladev: a UseAsync whose Data the dev state can
// keep shows it again, loading, after a reload of the code (ADR-113).

type devUser struct {
	Name string
	Age  int
}

// devAsyncApp mounts a component that loads a devUser and records what it
// saw, in a tree seeded with data (nil: none).
func devAsyncApp(t *testing.T, data []byte, f func(ctx context.Context) (devUser, error), saw *[]tela.Async[devUser], warns *[]string) *core.Tree {
	t.Helper()
	tr := core.NewTree(core.Config{Dev: true, Warn: func(m string) { *warns = append(*warns, m) }, Log: func(string) {}})
	tr.SetViewport(gfx.Size{W: 200, H: 100})
	if data != nil {
		tr.SeedDevState(data)
	}
	page := func() tela.Node {
		a := tela.UseAsync(f)
		*saw = append(*saw, a)
		return tela.C(func() tela.Node { return nil })
	}
	tr.Mount(tela.C(page))
	tr.Frame()
	tr.FlushEffects()
	return tr
}

// settle applies the updates of the goroutines until the tree has nothing
// left to build, or fails.
func settleTree(t *testing.T, tr *core.Tree, until func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !until() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the run's result")
		}
		tr.ApplyUpdates()
		tr.Frame()
		tr.FlushEffects()
		time.Sleep(time.Millisecond)
	}
}

func TestUseAsyncDevStateKeepsData(t *testing.T) {
	var warns []string
	var saw1 []tela.Async[devUser]
	tr1 := devAsyncApp(t, nil, func(ctx context.Context) (devUser, error) { return devUser{"Ada", 36}, nil }, &saw1, &warns)
	settleTree(t, tr1, func() bool { return !saw1[len(saw1)-1].Loading })
	if got := saw1[len(saw1)-1].Data; got != (devUser{"Ada", 36}) {
		t.Fatalf("the first process loaded %+v", got)
	}
	data, rep, err := tr1.CaptureDevState()
	if err != nil || rep.Kept == 0 {
		t.Fatalf("CaptureDevState = %v, kept %d (%s)", err, rep.Kept, rep.String())
	}

	// The second process: the run takes its time; the seeded Data shows,
	// loading, from the first build.
	release := make(chan struct{})
	var saw2 []tela.Async[devUser]
	tr2 := devAsyncApp(t, data, func(ctx context.Context) (devUser, error) {
		<-release
		return devUser{"Grace", 45}, nil
	}, &saw2, &warns)
	first := saw2[0]
	if !first.Loading || first.Data != (devUser{"Ada", 36}) || first.Err != nil {
		t.Fatalf("the first build after the reload saw %+v, want Loading with the kept Data", first)
	}
	close(release)
	settleTree(t, tr2, func() bool { return !saw2[len(saw2)-1].Loading })
	if got := saw2[len(saw2)-1].Data; got != (devUser{"Grace", 45}) {
		t.Errorf("the reloaded run left %+v", got)
	}
	if len(warns) != 0 {
		t.Errorf("warnings: %s", strings.Join(warns, "; "))
	}
}

func TestUseAsyncDevStateDoesNotKeepErr(t *testing.T) {
	var warns []string
	var saw []tela.Async[devUser]
	tr := devAsyncApp(t, nil, func(ctx context.Context) (devUser, error) { return devUser{}, errors.New("nope") }, &saw, &warns)
	settleTree(t, tr, func() bool { return !saw[len(saw)-1].Loading })
	if saw[len(saw)-1].Err == nil {
		t.Fatal("no error")
	}
	if _, rep, err := tr.CaptureDevState(); err != nil {
		t.Fatalf("CaptureDevState: %v (%s)", err, rep.String())
	}
	if len(warns) != 0 {
		t.Errorf("an error left a warning: %s", strings.Join(warns, "; "))
	}
}
