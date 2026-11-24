package core

import (
	"context"
	"errors"
	"testing"
	"time"
)

// FrameTime.Refresh is the display's as of each frame (ADR-317): it changes
// when the window moves to another display, and may be unknown.
func TestTextureNextFrameRefresh(t *testing.T) {
	var a texApp
	x := newWinHarness(t, Options{Root: a.root()})
	w := x.windows()[0]
	next := func() FrameTime {
		t.Helper()
		l := startTexLoop(a.tex, 1, nil)
		waitingFrame(t, a.tex)
		x.drain()
		ft := <-l.frames
		if err := <-l.errs; err != nil {
			t.Fatal(err)
		}
		return ft
	}
	if got, want := next().Refresh, x.b.FrameInterval(); got != want {
		t.Errorf("Refresh = %v, want headless's %v", got, want)
	}
	for _, want := range []time.Duration{time.Second / 144, 0, time.Second / 60} {
		w.SetRefresh(want)
		if got := next().Refresh; got != want {
			t.Errorf("Refresh = %v, want %v", got, want)
		}
	}
	x.expectIdle("once nothing waits for a frame")
}

// NextFrame does not return while a frame waits for its fence or is ready,
// since one more could replace it: it returns once the frame was taken, and
// when its fence failed, though no frame became ready then.
func TestTextureNextFrameWaitsForPending(t *testing.T) {
	for _, fails := range []bool{false, true} {
		var a texApp
		x := newWinHarness(t, Options{Root: a.root()})
		x.h.setTextureImporter(fakeImporter{
			dev: func() TextureDevice { return TextureDevice{} },
			imp: func(TextureMemory) (func() []byte, func(), error) {
				return func() []byte { return solid(4, 4, texRed).Pix }, nil, nil
			},
		})
		buf, err := a.tex.Import(Dmabuf{Width: 4, Height: 4, Fourcc: fourccAR24, Planes: []DmabufPlane{{Stride: 16}}})
		if err != nil {
			t.Fatal(err)
		}
		f := &pendingFence{done: make(chan error, 1)}
		if err := a.tex.Submit(buf, TextureFrame{Fence: f}); err != nil {
			t.Fatal(err)
		}
		l := startTexLoop(a.tex, 1, nil)
		waitingFrame(t, a.tex)
		x.drain()
		if len(l.frames) != 0 {
			t.Fatal("NextFrame returned while a frame waits for its fence")
		}
		if fails {
			f.done <- errors.New("the work failed")
		} else {
			f.done <- nil
		}
		waitFor(t, "NextFrame to return", func() bool { x.drain(); return len(l.frames) == 1 })
		if st := a.tex.Stats(); !fails && (st.Shown != 1 || st.Dropped != 0) || fails && st.Dropped != 1 {
			t.Errorf("fails %v: stats %+v", fails, st)
		}
		x.expectIdle("once nothing waits")
	}
}

// A loop of NextFrame drops nothing, however fast it is.
func TestTextureNextFrameDropsNothing(t *testing.T) {
	var a texApp
	x := newWinHarness(t, Options{Root: a.root()})
	const n = 200
	l := startTexLoop(a.tex, n, func() { _ = a.tex.SubmitImage(solid(2, 2, texRed), nil) })
	waitFor(t, "the loop to end", func() bool { x.drain(); return len(l.errs) == 1 })
	x.drain()
	if st := a.tex.Stats(); st.Submitted != n || st.Shown != n || st.Dropped != 0 {
		t.Errorf("stats %+v, want %d shown and none dropped", st, n)
	}
}

// pendingFence waits until it is given a result.
type pendingFence struct{ done chan error }

func (f *pendingFence) Wait(ctx context.Context) error {
	select {
	case err := <-f.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
