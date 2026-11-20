package core

import (
	"image"
	"time"
)

// The frame profile of an instrumented tree (ADR-299): the last
// ProfileFrames frames, each with the time of every phase of the pipeline
// (ARCHITECTURE section 8), what it built, styled, laid out and recorded,
// and the area it repainted.
//
// Times are wall-clock spans of the UI thread, read from the tree's debug
// clock (Tree.DebugNow: monotonic unless Config.DebugClock replaced it).
// Each instant is charged to one phase: the profile keeps a cursor, which
// Frame moves from phase to phase (Tree.phase), and the work that runs
// apart from Frame (effects, semantics, rasterization, presenting) pushes
// a span of its own phase over it and pops it at its end, so nested work
// is charged once, to the innermost phase. A frame is recorded when Frame
// runs the pipeline (Stats.Frames counts it): a Frame with nothing to do
// records nothing. Its Duration runs from its Animate phase (a Host's
// FrameEvent) or the start of Frame, to the end of Frame, plus the work
// charged to it after (its rasterization, its present, its passive
// effects, the semantics tree the accessibility bridge built after it).
// The input handled since the previous frame is in Phases[PhaseInput] and
// not in Duration: it ran between the frames. Rasterization runs in
// worker goroutines; PhaseRaster is the time the UI thread waited for
// them.
//
// The profile measures the app, not the tools that read it: a semantics
// tree built for a consumer (Tree.Semantics: a tools request, telatest,
// tela mcp, the debug overlays) runs in a span of phaseNone, which charges
// nothing, so it does not land in the frame shown last. The tools' other
// requests (Elements, Inspect, FrameProfiles...) open no span and are not
// charged either, unless they run inside one: a function queued with a
// tree's updates (tela.Post, Tree.enqueue) runs in the input span of the
// Host's WakeEvent, so tools work queued that way (tela dev's push of the
// profiles) is charged to the next frame's input unless it opens a
// phaseNone span of its own.
//
// Where the phases are: Input is the Host's input events and wakes (the
// handlers and the state updates they queue); Animate the Host's tick of
// gestures and Tree.Animate; Build the build phase with the state updates
// and scroll events applied before it; Style the style phase and the work
// that restyles (presence exits, focus); Layout the box structure and
// paragraphs, the layout phase and the scroll phase; Paint the paint
// phase, the float pass and the end of the frame; Semantics building the
// semantics tree; Raster RasterizeFrame; Present the Host's present;
// Effects the layout effects and the passive effects.

// ProfileFrames is the number of frames FrameProfiles keeps.
const ProfileFrames = 600

// maxFrameBuilt bounds the IDs a FrameProfile lists of the elements it
// built.
const maxFrameBuilt = 128

// maxSpanDepth bounds the nesting of profile spans.
const maxSpanDepth = 16

// Phase is a phase of the frame pipeline, as the profile charges it.
type Phase uint8

const (
	PhaseInput Phase = iota
	PhaseAnimate
	PhaseBuild
	PhaseStyle
	PhaseLayout
	PhasePaint
	PhaseSemantics
	PhaseRaster
	PhasePresent
	PhaseEffects
	// NumPhases is the number of phases.
	NumPhases
)

// phaseNone is the phase of the work the profile does not charge: the
// DevTools' own (see the comment at the top of instrument_prof.go).
const phaseNone = NumPhases

var phaseNames = [NumPhases]string{"input", "animate", "build", "style", "layout", "paint", "semantics", "raster", "present", "effects"}

// String names the phase: "input", "build"...
func (p Phase) String() string {
	if p < NumPhases {
		return phaseNames[p]
	}
	return "?"
}

// FrameProfile is one frame of the profile.
type FrameProfile struct {
	// Frame is its number (Stats.Frames after it).
	Frame int `json:"frame"`
	// Start is the frame clock at its start (the FrameEvent's time, the
	// clock of telatest).
	Start time.Duration `json:"start"`
	// Duration is the UI thread's time on it: every phase but
	// PhaseInput (see the comment at the top of instrument_prof.go).
	Duration time.Duration `json:"duration"`
	// Phases are the time of each phase, by Phase.
	Phases [NumPhases]time.Duration `json:"phases"`
	// Built counts the component builds since the previous frame
	// recorded (those of a Mount before it included), Styled the style
	// computations, LaidOut the boxes laid out (Stats.LaidOut) and
	// Layers the layers recorded by its paint passes.
	Built   int `json:"built"`
	Styled  int `json:"styled"`
	LaidOut int `json:"laidOut"`
	Layers  int `json:"layers"`
	// Damage is the area the application repainted in device pixels (its
	// whole framebuffer for a full frame), 0 until it is rasterized.
	Damage int64 `json:"damage"`
	// BuiltIDs are the elements built, in the order they built (the first
	// maxFrameBuilt), and BuiltMore counts the builds past them.
	BuiltIDs  []ElementID `json:"builtIDs,omitempty"`
	BuiltMore int         `json:"builtMore,omitempty"`
}

// profiler is the frame profile of an instrumented tree (UI thread).
type profiler struct {
	t *Tree
	// The cursor: the phase charged now (active), since mark; stack and
	// was hold the phases under the open spans.
	active bool
	phase  Phase
	mark   time.Duration
	stack  [maxSpanDepth]Phase
	was    [maxSpanDepth]bool
	depth  int
	// open: a Frame runs and is recorded in rec, which started at wall0.
	open  bool
	rec   FrameProfile
	wall0 time.Duration
	// live: the newest frame of ring takes the work charged after it.
	live bool
	ring []FrameProfile
	next int // the oldest record once ring is full
	// pre is what comes before the next frame.
	pre struct {
		on             bool // a Host's FrameEvent started it, at start
		start          time.Duration
		input, animate time.Duration
		built          []ElementID
		more           int
	}
	statsAt Stats // the counters at the end of the last frame recorded
	layers  int
}

func (p *profiler) init(t *Tree) {
	p.t = t
	p.ring = make([]FrameProfile, 0, ProfileFrames)
	p.statsAt = t.stats
}

func (p *profiler) now() time.Duration { return p.t.DebugNow() }

// charge adds d to phase ph: of the running frame, of the next frame
// (input and animation before it) or of the frame shown last; phaseNone
// adds it to none.
func (p *profiler) charge(ph Phase, d time.Duration) {
	switch {
	case ph >= NumPhases:
	case p.open:
		p.rec.Phases[ph] += d
	case ph == PhaseInput:
		p.pre.input += d
	case ph == PhaseAnimate:
		p.pre.animate += d
	case p.live:
		r := p.newest()
		r.Phases[ph] += d
		r.Duration += d
	}
}

// switchTo moves the cursor of a running frame to ph.
func (p *profiler) switchTo(ph Phase) {
	if !p.active || p.phase == ph {
		return
	}
	now := p.now()
	p.charge(p.phase, now-p.mark)
	p.phase, p.mark = ph, now
}

// spanBegin starts charging ph until the matching spanEnd, which it
// returns the depth for.
func (p *profiler) spanBegin(ph Phase) int {
	now := p.now()
	d := p.depth
	if d < maxSpanDepth {
		if p.active {
			p.charge(p.phase, now-p.mark)
		}
		p.stack[d], p.was[d] = p.phase, p.active
		p.depth++
	}
	p.active, p.phase, p.mark = true, ph, now
	return d
}

// spanEnd ends the span spanBegin returned depth d for.
func (p *profiler) spanEnd(d int) {
	if d >= maxSpanDepth || p.depth != d+1 {
		return // a Frame started over the open spans (begin)
	}
	now := p.now()
	if p.active {
		p.charge(p.phase, now-p.mark)
	}
	p.depth--
	p.phase, p.active, p.mark = p.stack[d], p.was[d], now
}

// frameEvent starts the work before a frame: a Host's FrameEvent, whose
// Animate phase opens the frame's Duration.
func (p *profiler) frameEvent() {
	p.live = false
	p.pre.on, p.pre.start = true, p.now()
}

// begin starts recording a Frame.
func (p *profiler) begin() {
	t := p.t
	now := p.now()
	ids := p.rec.BuiltIDs[:0]
	p.rec = FrameProfile{Frame: t.stats.Frames + 1, Start: t.scr.now}
	p.rec.Phases[PhaseInput], p.rec.Phases[PhaseAnimate] = p.pre.input, p.pre.animate
	p.rec.BuiltIDs, p.rec.BuiltMore = append(ids, p.pre.built...), p.pre.more
	p.wall0 = now
	if p.pre.on {
		p.wall0 = p.pre.start
	}
	p.pre.on, p.pre.input, p.pre.animate = false, 0, 0
	p.pre.built, p.pre.more = p.pre.built[:0], 0
	p.open, p.live = true, false
	p.layers = 0
	// A fresh cursor: spans a panic left open are dropped.
	p.depth = 0
	p.active, p.phase, p.mark = true, PhaseBuild, now
}

// end ends the Frame begin started: it is recorded when it ran the
// pipeline.
func (p *profiler) end() {
	if !p.open {
		return
	}
	t := p.t
	now := p.now()
	if p.active {
		p.charge(p.phase, now-p.mark)
	}
	p.open, p.active, p.depth = false, false, 0
	if t.stats.Frames != p.rec.Frame {
		return // nothing to do: not a frame of the profile
	}
	r := &p.rec
	r.Duration = now - p.wall0
	s := t.stats
	r.Built, r.Styled, r.LaidOut = s.Builds-p.statsAt.Builds, s.Styles-p.statsAt.Styles, s.LaidOut-p.statsAt.LaidOut
	r.Layers = p.layers
	p.statsAt = s
	var slot *FrameProfile
	if len(p.ring) < ProfileFrames {
		p.ring = append(p.ring, FrameProfile{})
		slot = &p.ring[len(p.ring)-1]
	} else {
		slot = &p.ring[p.next]
		p.next = (p.next + 1) % ProfileFrames
	}
	ids := slot.BuiltIDs[:0]
	*slot = *r
	slot.BuiltIDs = append(ids, r.BuiltIDs...)
	p.live = true
}

// newest returns the newest record (ring is not empty).
func (p *profiler) newest() *FrameProfile {
	if len(p.ring) < ProfileFrames {
		return &p.ring[len(p.ring)-1]
	}
	return &p.ring[(p.next+ProfileFrames-1)%ProfileFrames]
}

// builtID lists a build of element id in its frame.
func (p *profiler) builtID(id ElementID) {
	ids, more := &p.rec.BuiltIDs, &p.rec.BuiltMore
	if !p.open {
		ids, more = &p.pre.built, &p.pre.more
	}
	if len(*ids) < maxFrameBuilt {
		*ids = append(*ids, id)
	} else {
		*more++
	}
}

// damage records the damage of the frame rasterized last into fb (Tree.
// LastDamage), the newest frame's.
func (p *profiler) damage(d *Damage, fb image.Rectangle) {
	if !p.live || p.open {
		return
	}
	var area int64
	if d.Full {
		area = int64(fb.Dx()) * int64(fb.Dy())
	} else {
		for _, s := range d.Shifts {
			area += int64(s.Rect.Dx()) * int64(s.Rect.Dy())
		}
		for _, r := range d.Rects {
			area += int64(r.Dx()) * int64(r.Dy())
		}
	}
	p.newest().Damage = area
}

// FrameProfiles returns the recorded frames whose number is above since
// (0 for all), oldest first: at most the last ProfileFrames. Nil when the
// instrumentation is off. Call it on the UI thread.
func (t *Tree) FrameProfiles(since int) []FrameProfile {
	if t.ins == nil {
		return nil
	}
	p := &t.ins.prof
	var out []FrameProfile
	add := func(rs []FrameProfile) {
		for i := range rs {
			if rs[i].Frame > since {
				r := rs[i]
				r.BuiltIDs = append([]ElementID(nil), r.BuiltIDs...)
				out = append(out, r)
			}
		}
	}
	if len(p.ring) < ProfileFrames {
		add(p.ring)
	} else {
		add(p.ring[p.next:])
		add(p.ring[:p.next])
	}
	return out
}

// phase moves the frame profile's cursor to ph (a nil check when off).
func (t *Tree) phase(ph Phase) {
	if t.ins != nil {
		t.ins.prof.switchTo(ph)
	}
}
