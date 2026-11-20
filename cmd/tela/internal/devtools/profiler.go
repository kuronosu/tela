package devtools

import (
	"fmt"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/kit"
)

// budget is the time of a frame at 60 Hz.
const budget = 16667 * time.Microsecond

// chartHeight is the height of the plot of the frame chart, in pixels.
const chartHeight = 150

// phaseColors are the background classes of the phases, in the order of
// core.Phase: one hue each, readable on both schemes.
var phaseColors = [core.NumPhases]string{
	core.PhaseInput:     "bg-slate-400",
	core.PhaseAnimate:   "bg-cyan-400",
	core.PhaseBuild:     "bg-blue-500",
	core.PhaseStyle:     "bg-violet-400",
	core.PhaseLayout:    "bg-emerald-500",
	core.PhasePaint:     "bg-amber-400",
	core.PhaseSemantics: "bg-pink-400",
	core.PhaseRaster:    "bg-orange-500",
	core.PhasePresent:   "bg-zinc-500",
	core.PhaseEffects:   "bg-lime-400",
}

// ProfilerPanel is the Profiler tab: record frames, see where each one
// spent its time and which elements it built.
type ProfilerPanel struct{ panelProps }

// Summary is what a recording adds up to.
type Summary struct {
	Frames, OverBudget int
	// Slowest is the phase with the most time over the recording, and
	// SlowestShare its part of the frame time.
	Slowest      core.Phase
	SlowestShare float64
	Average, Max time.Duration
}

// Summarize adds up frames.
func Summarize(frames []*core.FrameProfile) Summary {
	var s Summary
	var total time.Duration
	var phases [core.NumPhases]time.Duration
	for _, f := range frames {
		s.Frames++
		total += f.Duration
		if f.Duration > budget {
			s.OverBudget++
		}
		s.Max = max(s.Max, f.Duration)
		for i, d := range f.Phases {
			if core.Phase(i) != core.PhaseInput {
				phases[i] += d
			}
		}
	}
	if s.Frames == 0 {
		return s
	}
	s.Average = total / time.Duration(s.Frames)
	for i, d := range phases {
		if d > phases[s.Slowest] {
			s.Slowest = core.Phase(i)
		}
	}
	if total > 0 {
		s.SlowestShare = float64(phases[s.Slowest]) / float64(total)
	}
	return s
}

// Build returns the panel.
func (p ProfilerPanel) Build() tela.Node {
	v, m := p.V, p.M
	sum := tela.UseMemo(func() Summary { return Summarize(v.Frames) }, len(v.Frames), v.Recording)
	var sel *core.FrameProfile
	for _, f := range v.Frames {
		if f.Frame == v.Frame {
			sel = f
		}
	}

	recordLabel, recordIcon, recordVariant, recordMark := "Record", iconRecord, kit.Outline, "text-red-500"
	if v.Recording {
		recordLabel, recordIcon, recordVariant, recordMark = "Stop", iconStop, kit.Destructive, ""
	}
	var main tela.Child
	switch {
	case len(v.Frames) == 0:
		title, hint := "Nothing recorded", "Press Record, then use the app. Each frame it draws is a bar here."
		if v.Recording {
			title, hint = "Recording", "Waiting for the app to draw. An idle app draws nothing: move the pointer or click in it."
		}
		main = Div(Class("min-h-0 flex-1"), empty(iconRecord, title, hint))
	default:
		main = Fragment(
			FrameChart{Frames: v.Frames, Selected: v.Frame, M: m},
			Div(Class("min-h-0 flex-1 overflow-y-auto border-t border-border"),
				If(sel == nil, Div(Class("h-full"), empty(iconCrosshair, "Select a frame", "Click a bar to see where the frame spent its time and what it built."))),
				If(sel != nil, FrameDetail{P: sel, Els: v.FrameEls, More: v.FrameMore, Loading: sel != nil && len(sel.BuiltIDs) > 0 && v.FrameEls == nil, Open: p.Open}),
			))
	}
	return Div(Class("flex min-h-0 flex-1 flex-col"),
		Div(Class("flex h-10 shrink-0 items-center gap-2 border-b border-border bg-card px-2"),
			kit.Button{Variant: recordVariant, Size: kit.Sm, Icon: recordIcon, IconClass: recordMark, OnClick: func() { m.SetRecording(!v.Recording) },
				Class: "h-7 gap-1.5 px-2.5 text-xs", AriaLabel: recordLabel + " frames", Children: Text(recordLabel)},
			kit.Button{Variant: kit.Ghost, Size: kit.Sm, Icon: iconTrash, Disabled: len(v.Frames) == 0, OnClick: m.ClearFrames,
				Class: "h-7 gap-1.5 px-2.5 text-xs", Children: Text("Clear")},
			Div(Class("flex-1")),
			SummaryBar{S: sum},
		),
		main,
	)
}

// SummaryBar is the line of numbers over the chart.
type SummaryBar struct{ S Summary }

// Build returns the bar.
func (s SummaryBar) Build() tela.Node {
	x := s.S
	if x.Frames == 0 {
		return nil
	}
	stat := func(label, value, class string) tela.Node {
		return Div(Key(label), Class("flex items-baseline gap-1.5"),
			Span(Class("text-[11px] text-muted-foreground"), Text(label)),
			Span(Class("font-mono text-xs tabular-nums", class), Text(value)))
	}
	over := ""
	if x.OverBudget > 0 {
		over = "text-destructive"
	}
	return Div(Class("flex items-center gap-4"), Role("status"), AriaLabel("Recording summary"),
		stat("frames", fmt.Sprint(x.Frames), ""),
		stat("over 16.7 ms", fmt.Sprint(x.OverBudget), over),
		stat("average", fmtMs(x.Average), ""),
		stat("worst", fmtMs(x.Max), ifs(x.Max > budget, "text-destructive", "")),
		stat("slowest phase", fmt.Sprintf("%s (%.0f%%)", x.Slowest, x.SlowestShare*100), ""),
	)
}

// FrameChart is the timeline: a bar per frame, stacked by phase, with the
// frame budget as a line.
type FrameChart struct {
	Frames   []*core.FrameProfile
	Selected int
	M        *Model
}

// Build returns the chart.
func (c FrameChart) Build() tela.Node {
	hover, setHover := tela.UseState(0)
	var worst time.Duration
	for _, f := range c.Frames {
		worst = max(worst, f.Duration)
	}
	// The scale is the budget plus room, or the worst frame rounded up to
	// 10 ms.
	scale := max(2*budget, (worst/(10*time.Millisecond)+1)*10*time.Millisecond)
	y := func(d time.Duration) float32 { return float32(chartHeight) * float32(d) / float32(scale) }
	// A frame is read from the bar under the pointer, else the selected one.
	shown := c.Selected
	if hover != 0 {
		shown = hover
	}
	var readout string
	for _, f := range c.Frames {
		if f.Frame == shown {
			readout = fmt.Sprintf("frame %d  %s", f.Frame, fmtMs(f.Duration))
		}
	}
	bars := Map(c.Frames, func(f *core.FrameProfile) tela.Node {
		return tela.WithKey(FrameBar{P: f, Scale: scale, Selected: f.Frame == c.Selected, M: c.M, Hover: setHover.Set}, f.Frame)
	})
	return Div(Class("shrink-0 bg-background"),
		Div(Class("flex h-8 items-center gap-3 px-3"),
			Span(Class("w-40 font-mono text-xs text-muted-foreground tabular-nums"), Text(ifs(readout == "", "hover a bar", readout))),
			Div(Class("flex flex-1 flex-wrap items-center justify-end gap-x-3 gap-y-1"),
				Map(phaseOrder(), func(ph core.Phase) tela.Node {
					return Span(Key(int(ph)), Class("flex items-center gap-1.5 text-[11px] text-muted-foreground"),
						Span(Class("size-2 rounded-[2px]", phaseColors[ph])), Text(ph.String()))
				}))),
		Div(Class("flex px-3 pb-3"),
			Div(Class("relative w-12 shrink-0"), Style(tela.Height(chartHeight)),
				AxisLabel{Text: fmtAxis(scale), Bottom: y(scale) - 12, Class: "text-muted-foreground"},
				AxisLabel{Text: "16.7 ms", Bottom: y(budget) - 6, Class: "text-amber-700 dark:text-amber-400"},
				AxisLabel{Text: "0", Bottom: 0, Class: "text-muted-foreground"},
			),
			Div(Class("relative min-w-0 flex-1"), Style(tela.Height(chartHeight)),
				Div(Class("pointer-events-none absolute inset-x-0 h-px border-t border-dashed border-amber-500/70", fmt.Sprintf("bottom-[%dpx]", int(y(budget))))),
				Div(Class("pointer-events-none absolute inset-x-0 bottom-0 border-t border-border")),
				Div(Class("absolute inset-0 flex items-end gap-px"), OnMouseLeave(func() { setHover.Set(0) }), bars),
			),
		),
	)
}

func phaseOrder() []core.Phase {
	out := make([]core.Phase, core.NumPhases)
	for i := range out {
		out[i] = core.Phase(i)
	}
	return out
}

// FrameBar is the column of one frame.
type FrameBar struct {
	P        *core.FrameProfile
	Scale    time.Duration
	Selected bool
	M        *Model
	Hover    func(int)
}

// Build returns the column.
func (b FrameBar) Build() tela.Node {
	f := b.P
	over := f.Duration > budget
	var segs []tela.Child
	for i, d := range f.Phases {
		hgt := float32(chartHeight) * float32(d) / float32(b.Scale)
		if hgt < 0.4 {
			continue
		}
		segs = append(segs, Div(Key(i), Class("w-full shrink-0", phaseColors[i]), Style(tela.Height(hgt))))
	}
	n := f.Frame
	return Div(Role("button"), AriaLabel(fmt.Sprintf("Frame %d, %s", f.Frame, fmtMs(f.Duration))), AriaPressed(b.Selected),
		Class("flex h-full min-w-px max-w-3 flex-1 flex-col-reverse justify-start",
			ifs(b.Selected, "bg-primary/35", ifs(over, "bg-destructive/10 hover:bg-destructive/20", "hover:bg-accent"))),
		OnClick(func() { b.M.SelectFrame(n) }),
		OnMouseEnter(func() { b.Hover(n) }),
		Fragment(segs...))
}

// FrameDetail is the breakdown of a frame: its phases, its counts and the
// elements it built with why.
type FrameDetail struct {
	P       *core.FrameProfile
	Els     []core.ElementInfo
	More    int
	Loading bool
	Open    func(core.ElementID)
}

// Build returns the detail.
func (d FrameDetail) Build() tela.Node {
	f := d.P
	over := f.Duration > budget
	var peak time.Duration
	for _, p := range f.Phases {
		peak = max(peak, p)
	}
	phases := make([]core.Phase, 0, core.NumPhases)
	for i := range f.Phases {
		phases = append(phases, core.Phase(i))
	}
	return Div(Class("flex min-h-full"),
		Div(Class("w-[420px] shrink-0 border-r border-border"),
			Div(Class("flex items-center gap-2 px-3 py-3"),
				Span(Class("text-[13px] font-semibold"), Textf("Frame %d", f.Frame)),
				Span(Class("font-mono text-[13px] tabular-nums", ifs(over, "text-destructive", "")), Text(fmtMs(f.Duration))),
				If(over, kit.Badge{Variant: kit.Destructive, Class: "text-[10px]", Children: Text("over budget")})),
			Div(Class("pb-2"),
				Map(phases, func(ph core.Phase) tela.Node {
					dur := f.Phases[ph]
					return Div(Key(int(ph)), Class("flex h-6 items-center gap-2 px-3 text-xs"),
						Span(Class("size-2 shrink-0 rounded-[2px]", phaseColors[ph])),
						Span(Class("w-20 shrink-0"), Text(ph.String())),
						Div(Class("h-1.5 min-w-0 flex-1 rounded-full bg-muted"),
							Div(Class("h-full rounded-full", phaseColors[ph], fmt.Sprintf("w-[%d%%]", pct(dur, peak))))),
						Span(Class("w-16 shrink-0 text-right font-mono tabular-nums", ifs(dur == 0, "text-muted-foreground", "")), Text(fmtMs(dur))))
				})),
			Div(Class("grid grid-cols-2 gap-x-6 gap-y-1 border-t border-border px-3 py-3"),
				count("elements built", fmt.Sprint(f.Built)),
				count("boxes styled", fmt.Sprint(f.Styled)),
				count("boxes laid out", fmt.Sprint(f.LaidOut)),
				count("layers painted", fmt.Sprint(f.Layers)),
				count("damage", fmtCount(f.Damage)+" px"),
			),
		),
		Div(Class("min-w-0 flex-1"),
			Div(Class("flex h-10 items-center justify-between px-3"),
				Span(Class("text-[11px] font-semibold uppercase tracking-wider text-muted-foreground"), Text("Elements built")),
				Span(Class("text-[11px] text-muted-foreground"), Textf("%d in this frame", f.Built))),
			If(d.Loading, Div(Class("px-3 pb-3 text-xs text-muted-foreground"), Text("Loading..."))),
			If(!d.Loading && len(d.Els) == 0, Div(Class("px-3 pb-3 text-xs text-muted-foreground"),
				Text("None of them is mounted any more, so their reasons are gone."))),
			Map(d.Els, func(el core.ElementInfo) tela.Node {
				return tela.WithKey(BuiltRow{El: el, Frame: f.Frame, Open: d.Open}, el.ID)
			}),
			If(d.More > 0 && len(d.Els) > 0, Div(Class("px-3 py-2 text-[11px] text-muted-foreground"), Textf("and %d more", d.More))),
		),
	)
}

func count(label, value string) tela.Node {
	return Div(Key(label), Class("flex items-baseline justify-between gap-3 text-xs"),
		Span(Class("text-muted-foreground"), Text(label)),
		Span(Class("font-mono tabular-nums"), Text(value)))
}

// BuiltRow is an element a frame built and the reasons of that build.
type BuiltRow struct {
	El    core.ElementInfo
	Frame int
	Open  func(core.ElementID)
}

// Build returns the row.
func (b BuiltRow) Build() tela.Node {
	el := b.El
	var reasons []core.BuildReason
	for _, r := range el.Recent {
		if r.Frame == b.Frame {
			reasons = r.Reasons
		}
	}
	pkg, name := kindParts(el.Kind)
	return Div(Key(el.ID), Role("button"), AriaLabel("Show "+el.Kind+" in Elements"),
		Class("group flex cursor-default flex-col gap-0.5 border-t border-border/60 px-3 py-1.5 hover:bg-accent/60"),
		OnClick(func() { b.Open(el.ID) }),
		Div(Class("flex items-baseline gap-2 font-mono text-xs"),
			Span(Class("text-muted-foreground/70"), Text(pkg)),
			Span(Class("font-medium", ifs(el.Component, "text-violet-700 dark:text-violet-300", "text-sky-700 dark:text-sky-300")), Text(name)),
			If(el.Key != "", Span(Class("text-amber-700 dark:text-amber-300"), Textf("key=%q", el.Key))),
			If(!el.Site.IsZero(), Span(Class("ml-auto text-[11px] text-muted-foreground"), Text(el.Site.String()))),
			Icon(iconExternal, Class("size-3 shrink-0 self-center text-muted-foreground opacity-0 group-hover:opacity-100"))),
		Div(Class("flex flex-col gap-0.5 pl-0.5"),
			Map(reasons, func(r core.BuildReason) tela.Node { return tela.WithKey(Reason{R: r}, r.String()) }),
			If(len(reasons) == 0, Span(Class("text-xs text-muted-foreground"), Text("No reason recorded for this build.")))),
	)
}

// AxisLabel is a label of the chart's vertical axis, bottom pixels over its
// foot.
type AxisLabel struct {
	Text   string
	Bottom float32
	Class  string
}

// Build returns the label.
func (a AxisLabel) Build() tela.Node {
	return Span(Class("absolute right-2 text-[10px] leading-3 tabular-nums", a.Class, fmt.Sprintf("bottom-[%dpx]", int(a.Bottom))), Text(a.Text))
}

func fmtAxis(d time.Duration) string { return fmt.Sprintf("%d ms", d/time.Millisecond) }

// pct is d as a part of whole, 0 to 100.
func pct(d, whole time.Duration) int {
	if whole <= 0 {
		return 0
	}
	return int(100 * float64(d) / float64(whole))
}
