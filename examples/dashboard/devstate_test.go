//go:build teladev

package main

import (
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
)

// The cost of the dev state of `tela dev` (ADR-113) on this page, after a
// few interactions (period, tab, task order), at 1280 x 800:
//
//	go test -tags teladev -run DevState -v ./examples/dashboard

func TestDevStateDashboard(t *testing.T) {
	r := newRig(t, 1280, 800, 1, false)
	r.settle(600)
	for _, c := range [][]func(){r.click("tab", "90D"), r.click("tab", "Orders"), r.click("button", "Due")} {
		r.tick(nil, c...)
		r.settle(600)
	}
	elements := r.tree.Stats().Mounts - r.tree.Stats().Unmounts

	const runs = 7
	var capture, plain, seeded []time.Duration
	var data []byte
	var rep core.DevStateReport
	for range runs {
		t0 := time.Now()
		d, rp, err := r.tree.CaptureDevState()
		capture = append(capture, time.Since(t0))
		if err != nil {
			t.Fatal(err)
		}
		data, rep = d, rp
	}
	mount := func(seed []byte) (time.Duration, *core.Tree) {
		tr := core.NewTree(core.Config{RootClass: rootClass})
		tr.SetViewport(gfx.Size{W: 1280, H: 800})
		tr.Animate(0)
		t0 := time.Now()
		if seed != nil {
			tr.SeedDevState(seed)
		}
		tr.Mount(Dashboard())
		return time.Since(t0), tr
	}
	var last *core.Tree
	for range runs {
		d, tr := mount(nil)
		plain = append(plain, d)
		tr.Unmount()
		d, last = mount(data)
		seeded = append(seeded, d)
		last.Unmount()
	}
	sr := last.DevSeedReport()
	t.Logf("%d elements, %d components; %v", elements, rep.Components, rep)
	t.Logf("capture and encode: median %v; mount: median %v, with the snapshot %v (decode and seed: %v)",
		median(capture), median(plain), median(seeded), median(seeded)-median(plain))
	t.Logf("seed report: %+v", sr)
	if sr.Err != "" || sr.Restored == 0 || len(sr.Remounted) != 0 {
		t.Errorf("seed report %+v", sr)
	}
	for _, r := range rep.Reset {
		if !r.Silent {
			t.Errorf("the dashboard's %s #%d of %s resets: %s", r.Hook, r.Index, r.Component, r.Reason)
		}
	}
}

func median(ds []time.Duration) time.Duration {
	s := slices.Clone(ds)
	slices.Sort(s)
	return s[len(s)/2]
}
