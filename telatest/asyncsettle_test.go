package telatest_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// slowLoad is a screen whose data takes `d` of real time to load.
func slowLoad(d time.Duration) func() tela.Node {
	return func() tela.Node {
		a := tela.UseAsync(func(ctx context.Context) (string, error) {
			select {
			case <-time.After(d):
				return "ready", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		})
		if a.Loading {
			return P(Text("loading"))
		}
		return P(Textf("data: %s", a.Data))
	}
}

// Settling waits for the UseAsync runs in flight, so a plain query, a
// click or a snapshot never captures the loading state (ADR-307).
func TestSettleWaitsForUseAsync(t *testing.T) {
	app := telatest.MountFunc(t, slowLoad(40*time.Millisecond), telatest.Size(300, 100))
	if n := app.Count(telatest.ByText("data: ready")); n != 1 {
		t.Errorf("a query right after Mount counted %d loaded texts, want 1: it saw the loading state", n)
	}
	if got := app.AriaSnapshot(); !strings.Contains(got, "data: ready") {
		t.Errorf("snapshot:\n%s", got)
	}
}

// The headless entry points (tela snapshot, inspect) wait too.
func TestHeadlessEntryPointsWaitForUseAsync(t *testing.T) {
	root := tela.C(slowLoad(40 * time.Millisecond))
	got, err := telatest.AriaSnapshot(root, telatest.Size(300, 100))
	if err != nil || !strings.Contains(got, "data: ready") {
		t.Errorf("AriaSnapshot = %q, %v", got, err)
	}
	js, err := telatest.LayoutJSON(root, telatest.Size(300, 100))
	if err != nil || !strings.Contains(string(js), "data: ready") {
		t.Errorf("LayoutJSON = %.200s, %v", js, err)
	}
	img, err := telatest.Render(root, telatest.Size(300, 100))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := telatest.Render(tela.C(func() tela.Node { return P(Text("data: ready")) }), telatest.Size(300, 100))
	if err != nil {
		t.Fatal(err)
	}
	if string(img.Pix) != string(loaded.Pix) {
		t.Errorf("the image of the screen that loads differs from the loaded one: it was drawn loading")
	}
}

// A run that never finishes fails the settle after the timeout, naming the
// run, rather than hanging or capturing the loading state.
func TestSettleNamesAUseAsyncThatNeverFinishes(t *testing.T) {
	mustFail(t, func(tb testing.TB) {
		telatest.MountFunc(tb, func() tela.Node {
			a := tela.UseAsync(func(ctx context.Context) (string, error) {
				<-ctx.Done()
				return "", ctx.Err()
			})
			return P(Textf("loading=%v", a.Loading))
		}, telatest.Size(300, 100), telatest.Timeout(60*time.Millisecond))
	}, "UseAsync run", "still in flight", "NoSettle")
}

// NoSettle is how a test sees the loading state: nothing waits for the run,
// Pending names it, and an expectation finds the result once it ends.
func TestNoSettleLeavesUseAsyncAlone(t *testing.T) {
	release := make(chan struct{})
	app := telatest.MountFunc(t, func() tela.Node {
		a := tela.UseAsync(func(ctx context.Context) (string, error) {
			select {
			case <-release:
				return "ready", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		})
		if a.Loading {
			return Div(P(Text("loading")), Button(Text("Cancel")))
		}
		return P(Textf("data: %s", a.Data))
	}, telatest.Size(300, 100), telatest.NoSettle())
	app.ExpectText("loading")
	app.Click(telatest.ByRole("button", "Cancel")) // acts while it loads
	var found bool
	for _, p := range app.Pending() {
		found = found || p.Kind == telatest.PendingAsync && strings.Contains(p.What, "UseAsync run")
	}
	if !found {
		t.Errorf("Pending = %v, want the UseAsync run named", app.Pending())
	}
	close(release)
	app.ExpectText("data: ready")
}
