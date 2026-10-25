package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/player/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// playerStyle is kit's default style with two button sizes for the
// transport: "transport" (previous, next) and "play". Their icons are
// filled (Lucide draws outlines; fill-current makes the triangle and the
// bars solid, as players do). A recipe edit, not a component of its own:
// kit.StyleProvider applies it to the player.
var playerStyle = func() *kit.Style {
	s := kit.DefaultStyle()
	s.Button.Sizes["transport"] = "size-12 rounded-full"
	s.Button.IconSizes["transport"] = "size-5"
	s.Button.Sizes["play"] = "size-16 rounded-full"
	s.Button.IconSizes["play"] = "size-7"
	s.Button.Icon += " fill-current"
	return &s
}()

const (
	transport kit.Size = "transport"
	play      kit.Size = "play"
)

// NowPlaying is the player card: cover, title, progress and controls. It
// reads Track in its body, so a new track rebuilds it; the progress bar
// below it does not. *Transport is a pointer, comparable and stable, so
// the components that show or drive the player take it as a field and are
// skipped when nothing they read changed.
type NowPlaying struct{ Tr *Transport }

func (p NowPlaying) Build() tela.Node {
	tr := p.Tr
	track := tr.Current()
	return Fragment(kit.Card{
		Class: "w-[380px] shrink-0 gap-5 rounded-3xl bg-card/80 py-6 shadow-2xl",
		Children: Div(Class("flex flex-col gap-5"),
			Cover{Track: track, Class: "mx-auto size-[clamp(96px,calc(100vh-356px),332px)] rounded-2xl shadow-lg"},
			Div(Class("flex flex-col gap-1"),
				Div(Class("text-xs font-medium tracking-widest text-violet-300 uppercase"), Text("Now playing")),
				H2(Class("truncate text-2xl font-semibold tracking-tight"), Text(track.Title)),
				P(Class("truncate text-sm text-muted-foreground"), Text(track.Artist)),
			),
			SeekBar{Tr: tr},
			Controls{Tr: tr},
			VolumeRow{},
		),
	})
}

// Cover is the cover art: a gradient with a few shapes (only two in the
// small covers of the queue). Class sets its size.
type Cover struct {
	Track Track
	Small bool
	Class string
}

func (c Cover) Build() tela.Node {
	box := Class("relative aspect-square shrink-0 overflow-hidden bg-linear-to-br", c.Track.From, c.Track.To, c.Class)
	if c.Small {
		return Div(box,
			Div(Class("absolute -top-3 -right-3 size-8 rounded-full bg-white/25")),
			Div(Class("absolute -bottom-4 -left-2 size-9 rounded-full bg-white/10")),
		)
	}
	return Div(box,
		Div(Class("absolute -top-12 -right-10 size-48 rounded-full bg-white/20")),
		Div(Class("absolute right-10 bottom-10 size-24 rounded-full bg-black/20")),
		Div(Class("absolute -bottom-16 -left-8 size-56 rounded-full bg-white/10")),
		Div(Class("absolute bottom-6 left-6 size-12 rounded-full border-4 border-white/70")),
	)
}

// SeekBar is the seek bar with the elapsed and total time: a kit.Slider
// whose ValueSource is the transport's Elapsed. The knob and the range read
// it in paint-phase closures, so a tick of the clock repaints them and
// nothing builds, nor lays out; dragging stores each position in Elapsed
// through OnValueChange, which costs a repaint too. The elapsed time is a
// TextFn of Secs, which lays out its paragraph once per second.
//
// The component builds when the track changes (the slider's maximum) and
// once a second, on Beat, so that aria-valuenow follows the music; a drag
// does not move Beat.
type SeekBar struct{ Tr *Transport }

func (p SeekBar) Build() tela.Node {
	tr := p.Tr
	tr.Beat.Get() // subscribe: see the type's comment
	total := float64(tr.Current().Seconds)

	return Div(Class("flex flex-col gap-2"),
		kit.Slider{
			AriaLabel: "Seek", ValueSource: tr.Elapsed, Max: total, Step: 1,
			OnValueChange: tr.SeekTo,
			FormatValue:   func(v float64) string { return clock(int(v)) },
			// The root is the target of the press: taller than the 6 px
			// track, so it is easy to hit.
			Class: "h-4 cursor-pointer",
			Props: TestID("seek"),
		},
		Div(Class("flex justify-between text-xs text-muted-foreground tabular-nums"),
			Span(TextFn(func() string { return clock(tr.Secs.Get()) })),
			Span(Text(clock(int(total)))),
		),
	)
}

// clock formats seconds as m:ss.
func clock(s int) string { return fmt.Sprintf("%d:%02d", s/60, s%60) }

// Controls are the previous, play/pause and next buttons, each a
// kit.Button with an icon and a tooltip. It reads Playing, so a toggle
// rebuilds only the controls.
type Controls struct{ Tr *Transport }

func (p Controls) Build() tela.Node {
	tr := p.Tr
	playing := tr.Playing.Get()
	toggle, toggleIcon := "Pause", icons.Pause
	if !playing {
		toggle, toggleIcon = "Play", icons.Play
	}
	return Div(Class("flex items-center justify-center gap-6"),
		kit.Tooltip{
			Trigger: kit.Button{Variant: kit.Ghost, Size: transport, Icon: icons.SkipBack, AriaLabel: "Previous track",
				OnClick: tr.Prev, Class: "text-muted-foreground hover:text-foreground"},
			Content: Text("Previous"),
		},
		kit.Tooltip{
			Trigger: kit.Button{Size: play, Icon: toggleIcon, AriaLabel: toggle, OnClick: tr.Toggle,
				Class: "shadow-lg hover:scale-105 active:scale-95"},
			Content: Text(toggle),
		},
		kit.Tooltip{
			Trigger: kit.Button{Variant: kit.Ghost, Size: transport, Icon: icons.SkipForward, AriaLabel: "Next track",
				OnClick: tr.Next, Class: "text-muted-foreground hover:text-foreground"},
			Content: Text("Next"),
		},
	)
}

// VolumeRow is a mute kit.Toggle and a volume kit.Slider. The player makes
// no sound, so the volume is local state of this row.
type VolumeRow struct{}

func (VolumeRow) Build() tela.Node {
	volume, setVolume := tela.UseState(70.0)
	muted, setMuted := tela.UseState(false)
	ic := icons.Volume2
	if muted {
		ic = icons.VolumeX
	}
	return Div(Class("flex items-center gap-3"),
		kit.Toggle{
			AriaLabel: "Mute", Icon: ic, Pressed: &muted, OnPressedChange: setMuted.Set,
			Class: "shrink-0 text-muted-foreground",
		},
		kit.Slider{
			AriaLabel: "Volume", Value: &volume, OnValueChange: setVolume.Set, Step: 5, Disabled: muted,
			FormatValue: func(v float64) string { return fmt.Sprintf("%.0f%%", v) },
			Class:       "h-4",
		},
	)
}

// Queue is the list of tracks, a kit.Card with a header. It reads Track and
// Playing in its body to mark the current row.
type Queue struct{ Tr *Transport }

func (p Queue) Build() tela.Node {
	current, playing := p.Tr.Track.Get(), p.Tr.Playing.Get()
	return Fragment(kit.Card{
		Class:        "hidden w-[420px] shrink-0 gap-2 rounded-3xl bg-card/60 py-4 lg:flex",
		TitleLevel:   2,
		TitleClass:   "text-lg leading-7 tracking-tight",
		Title:        Text("Up next"),
		Action:       Span(Class("text-xs text-muted-foreground"), Textf("%d tracks", len(tracks))),
		HeaderClass:  "items-baseline px-6 pb-1",
		ContentClass: "flex flex-col gap-1 px-3",
		Children: Fragment(
			MapKeyed(tracks, func(t Track) string { return t.Title }, func(t Track) tela.Component {
				i := indexOf(t)
				return QueueRow{Tr: p.Tr, Index: i, Current: i == current, Playing: playing && i == current}
			}),
		),
	})
}

func indexOf(t Track) int {
	for i, c := range tracks {
		if c == t {
			return i
		}
	}
	return -1
}

// QueueRow is one track: a small cover, title, artist and length, in a
// ghost kit.Button. The current row is highlighted; clicking a row plays
// it.
type QueueRow struct {
	Tr               *Transport
	Index            int
	Current, Playing bool
}

func (p QueueRow) Build() tela.Node {
	t := tracks[p.Index]
	cls := ""
	if p.Current {
		cls = "bg-accent"
	}
	return Fragment(kit.Button{
		Variant: kit.Ghost,
		Class:   "h-auto w-full justify-start gap-3 rounded-2xl p-2 pr-4 text-left font-normal " + cls,
		OnClick: func() { p.Tr.Select(p.Index) },
		Children: Fragment(
			Div(Class("flex w-5 shrink-0 justify-center text-sm text-muted-foreground tabular-nums"),
				If(!p.Playing, Textf("%d", p.Index+1)),
				If(p.Playing, Icon(icons.AudioLines, Class("size-4 text-violet-400"))),
			),
			Cover{Track: t, Small: true, Class: "size-11 rounded-xl"},
			Div(Class("flex min-w-0 flex-1 flex-col"),
				Span(Class("truncate text-sm font-medium"), Text(t.Title)),
				Span(Class("truncate text-sm text-muted-foreground"), Text(t.Artist)),
			),
			Span(Class("text-sm text-muted-foreground tabular-nums"), Text(clock(t.Seconds))),
		),
	})
}
