package core

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/style"
)

// The ring keeps the last ProfileFrames frames, each with what it built.
func TestInstrumentProfileRing(t *testing.T) {
	var set Setter[int]
	tr := newInsTree(t, func() Node {
		return Component(func() Node {
			n, s := UseState(0)
			set = s
			return Primitive(style.KindDiv, Textf("%d", n))
		})
	})
	comp := insOne(t, tr, "core.TestInstrumentProfileRing.func1.1")
	first := tr.FrameProfiles(0)
	if len(first) != 1 || first[0].Frame != 1 || first[0].Built != 1 || len(first[0].BuiltIDs) != 1 || first[0].BuiltIDs[0] != comp.ID {
		t.Fatalf("the mount's frame: %+v", first)
	}
	// An idle Frame is not recorded.
	tr.Frame()
	if n := len(tr.FrameProfiles(0)); n != 1 {
		t.Errorf("an idle frame was recorded: %d frames", n)
	}
	const n = ProfileFrames + 50
	for i := range n {
		set.Set(i + 1)
		tr.Frame()
	}
	ps := tr.FrameProfiles(0)
	if len(ps) != ProfileFrames {
		t.Fatalf("%d frames kept, want %d", len(ps), ProfileFrames)
	}
	last := tr.Stats().Frames
	for i, p := range ps {
		if want := last - ProfileFrames + 1 + i; p.Frame != want {
			t.Fatalf("frame %d of the ring is %d, want %d", i, p.Frame, want)
		}
		if p.Built != 1 || len(p.BuiltIDs) != 1 || p.BuiltIDs[0] != comp.ID || p.LaidOut == 0 || p.Layers == 0 {
			t.Fatalf("frame %d: %+v", p.Frame, p)
		}
		if p.Duration <= 0 || p.Phases[PhaseBuild] <= 0 {
			t.Fatalf("frame %d took %v (build %v)", p.Frame, p.Duration, p.Phases[PhaseBuild])
		}
	}
	if since := tr.FrameProfiles(last - 3); len(since) != 3 || since[0].Frame != last-2 {
		t.Errorf("FrameProfiles(%d): %d frames", last-3, len(since))
	}
	// The builds of a Mount count in the next frame.
	tr.Mount(Component(func() Node { return Text("other") }))
	tr.Frame()
	if p := tr.FrameProfiles(last); len(p) != 1 || p[0].Built != 1 {
		t.Errorf("after a Mount: %+v", p)
	}
}

// With a clock that moves only when read, every tick of a frame is
// charged to one phase: the phases but the input sum to the frame's
// Duration, up to the reading between its Animate and its Frame; the
// input before it is in its Phases, not its Duration.
func TestInstrumentProfilePhases(t *testing.T) {
	var clock time.Duration
	tick := func() time.Duration {
		clock += time.Microsecond
		return clock
	}
	var set Setter[string]
	effects := 0
	root := Component(func() Node {
		label, s := UseState("start")
		set = s
		UseEffect(func() func() { effects++; return nil }, label)
		return Primitive(style.KindDiv, Class("p-4"),
			Primitive(style.KindP, Text(label)),
			Primitive(style.KindButton, Class("px-2 bg-sky-500"), Text("go")),
		)
	})
	x := newScrollHost(t, root, 300, 200, Config{Dev: true, DebugClock: tick})
	tr := x.tree()
	tr.SetInstrument(true)
	t.Cleanup(func() { tr.SetInstrument(false) })
	x.send(move(10, 10), move(20, 20))
	set.Set("a longer label that lays out again")
	x.frame()
	ps := tr.FrameProfiles(0)
	if len(ps) != 1 {
		t.Fatalf("%d frames recorded", len(ps))
	}
	p := ps[0]
	t.Logf("%+v", p)
	var sum time.Duration
	for ph := PhaseAnimate; ph < NumPhases; ph++ {
		sum += p.Phases[ph]
	}
	if d := p.Duration - sum; d < 0 || d > 3*time.Microsecond {
		t.Errorf("the phases sum to %v of a %v frame: %v", sum, p.Duration, p.Phases)
	}
	for _, ph := range []Phase{PhaseInput, PhaseAnimate, PhaseBuild, PhaseStyle, PhaseLayout, PhasePaint, PhaseRaster, PhasePresent, PhaseEffects} {
		if p.Phases[ph] <= 0 {
			t.Errorf("%s took %v: %v", ph, p.Phases[ph], p.Phases)
		}
	}
	if p.Built != 1 || p.Damage <= 0 || p.Layers == 0 || p.Start != x.b.Now() {
		t.Errorf("frame %+v (clock %v)", p, x.b.Now())
	}
	if effects != 2 {
		t.Errorf("%d effects ran", effects)
	}
	// A semantics tree built after the frame is charged to it.
	tr.Semantics()
	q := tr.FrameProfiles(0)[0]
	if q.Phases[PhaseSemantics] <= 0 || q.Duration-p.Duration != q.Phases[PhaseSemantics]-p.Phases[PhaseSemantics] {
		t.Errorf("after Semantics: %v (was %v), duration %v (was %v)", q.Phases[PhaseSemantics], p.Phases[PhaseSemantics], q.Duration, p.Duration)
	}
}

// With the real clock, the phases of a heavy frame cover it.
func TestInstrumentProfileWallClock(t *testing.T) {
	var set Setter[int]
	root := Component(func() Node {
		n, s := UseState(0)
		set = s
		rows := make([]Node, 400)
		for i := range rows {
			rows[i] = Primitive(style.KindDiv, Key(i), Class("flex gap-2 px-2"), Textf("row %d of %d", i, n))
		}
		return Primitive(style.KindDiv, Class("flex flex-col"), List(rows))
	})
	x := newScrollHost(t, root, 400, 600, Config{Dev: true})
	tr := x.tree()
	tr.SetInstrument(true)
	t.Cleanup(func() { tr.SetInstrument(false) })
	for i := range 3 {
		set.Set(i + 1)
		x.frame()
	}
	for _, p := range tr.FrameProfiles(0) {
		t.Logf("frame %d: %v of %v", p.Frame, p.Phases, p.Duration)
		var sum time.Duration
		for ph := PhaseAnimate; ph < NumPhases; ph++ {
			sum += p.Phases[ph]
		}
		if sum > p.Duration || float64(sum) < 0.8*float64(p.Duration) {
			t.Errorf("frame %d: the phases sum to %v of %v: %v", p.Frame, sum, p.Duration, p.Phases)
		}
	}
}

// The log keeps the tree's warnings and errors, and the Host's.
func TestInstrumentLogs(t *testing.T) {
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Warn: func(string) {}, Log: func(string) {}})
	tr.warnf("before %d", 0)
	if tr.Logs(0) != nil {
		t.Error("a log while off")
	}
	tr.SetInstrument(true)
	t.Cleanup(func() { tr.SetInstrument(false) })
	tr.warnf("careful %d", 1)
	tr.warnOnce("k", func() string { return "once" })
	tr.logf("broken %s", "x")
	es := tr.Logs(0)
	if len(es) != 3 || es[0].Msg != "careful 1" || es[0].Level != slog.LevelWarn || es[1].Msg != "once" || es[2].Level != slog.LevelError || es[2].Seq != 3 {
		t.Fatalf("log %+v", es)
	}
	if es[0].At.IsZero() || es[2].At.Before(es[0].At) {
		t.Errorf("times %v, %v", es[0].At, es[2].At)
	}
	if got := tr.Logs(2); len(got) != 1 || got[0].Msg != "broken x" {
		t.Errorf("Logs(2) = %+v", got)
	}
	for i := range maxLogs + 10 {
		tr.warnf("w%d", i)
	}
	es = tr.Logs(0)
	if len(es) != maxLogs || es[len(es)-1].Msg != fmt.Sprintf("w%d", maxLogs+9) || es[0].Seq != uint64(3+10+1) {
		t.Errorf("%d entries, first %+v, last %+v", len(es), es[0], es[len(es)-1])
	}

	// The Host's errors land in its main tree's log.
	var host strings.Builder
	x := newScrollHost(t, Primitive(style.KindDiv), 100, 100, Config{Dev: true})
	x.h.log = insHostLog{w: &host, t: x.tree()}
	x.tree().SetInstrument(true)
	t.Cleanup(func() { x.tree().SetInstrument(false) })
	fmt.Fprintln(x.h.log, "tela: present: lost")
	if got := x.tree().Logs(0); len(got) != 1 || got[0].Msg != "tela: present: lost" || host.String() != "tela: present: lost\n" {
		t.Errorf("host log %+v, writer %q", got, host.String())
	}
}
