package main

import (
	"fmt"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
)

// The page of Progress. The two bars that move by themselves (a download
// and a tween) read their value through ValueFn, so a tick paints the bar and
// builds nothing; they only run after a press, so the page is idle until then.

// downloadTick is how often the download advances, and downloadStep by how
// much (a download of about three seconds).
const (
	downloadTick = 50 * time.Millisecond
	downloadStep = 1.7
)

// Download is a bar that fills while a "download" runs: a tela.Value that an
// interval raises, read by the bar's ValueFn and by a TextFn for the number.
// Neither builds the component: only the button's label (running or not)
// does.
type Download struct {
	Label, Start, Again, Running string
	Format                       func(float64) string
}

func (d Download) Build() tela.Node {
	done := tela.UseValue(0.0)
	running, setRunning := tela.UseState(false)
	every := time.Duration(0)
	if running {
		every = downloadTick
	}
	tela.UseInterval(every, func() {
		v := done.Get() + downloadStep
		if v >= 100 {
			v = 100
			setRunning.Set(false)
		}
		done.Set(v)
	})
	label := d.Start
	switch {
	case running:
		label = d.Running
	case done.Get() >= 100:
		label = d.Again
	}
	return Div(Class("flex w-full max-w-sm flex-col gap-3"),
		Div(Class("flex flex-col gap-2"),
			Div(Class("flex justify-between text-sm"), Span(Class("font-medium"), Text(d.Label)),
				Span(Class("text-muted-foreground tabular-nums"), TestID("download-text"), TextFn(func() string { return d.Format(done.Get()) }))),
			kit.Progress{ValueFn: done.Get, AriaLabel: d.Label}),
		kit.Button{Variant: kit.Outline, Size: kit.Sm, Class: "self-start", Disabled: running, Icon: icons.Download, Children: Text(label),
			OnClick: func() {
				done.Set(0)
				setRunning.Set(true)
			}})
}

// Tween is a bar that eases to the value a press chooses: tela.UseTween gives
// a float32 that moves frame by frame, which ValueFn reads.
type Tween struct {
	Label, Reset string
}

func (t Tween) Build() tela.Node {
	target, setTarget := tela.UseState(float32(25))
	pct := tela.UseTween(target, tela.Duration(700*time.Millisecond), tela.Ease("ease-out"))
	return Div(Class("flex w-full max-w-sm flex-col gap-3"),
		kit.Progress{ValueFn: func() float64 { return float64(pct.Get()) }, AriaLabel: t.Label},
		Div(Class("flex gap-2"),
			kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text("+25%"),
				OnClick: func() { setTarget.Update(func(v float32) float32 { return min(v+25, 100) }) }},
			kit.Button{Variant: kit.Ghost, Size: kit.Sm, Children: Text(t.Reset), OnClick: func() { setTarget.Set(0) }}))
}

func progressPage() tela.Node {
	f := i18n.UseFormatter()
	tr := trFor(f)
	percent := func(v float64) string { return f.Number(v/100, i18n.Percent()) }
	done, setDone := tela.UseState(35.0)
	bump := func(d float64) func() {
		return func() { setDone.Update(func(v float64) float64 { return max(0, min(100, v+d)) }) }
	}
	return grid(
		demo("Values", "The bar moves with a transition when Value changes.", col,
			kit.Progress{Value: 13, Class: "max-w-sm", AriaLabel: tr("progress.v13", "13 percent")},
			kit.Progress{Value: 66, Class: "max-w-sm", AriaLabel: tr("progress.v66", "66 percent")},
			kit.Progress{Value: 100, Class: "max-w-sm", AriaLabel: tr("progress.done", "Done")}),
		demo("Controlled", "A task you advance by hand.", col,
			Div(Class("flex w-full max-w-sm flex-col gap-2"),
				Div(Class("flex justify-between text-sm"), Span(Class("font-medium"), Text(tr("progress.uploading", "Uploading photos"))), Span(Class("text-muted-foreground tabular-nums"), Text(percent(done)))),
				kit.Progress{Value: done, AriaLabel: tr("progress.upload", "Upload")}),
			Div(Class("flex gap-2"),
				kit.Button{Variant: kit.Outline, Size: kit.Sm, OnClick: bump(-15), Children: Text(f.Number(-15))},
				kit.Button{Variant: kit.Outline, Size: kit.Sm, OnClick: bump(15), Children: Text("+" + f.Number(15))})),
		demo("A download", "ValueFn is read when the bar is painted, so a value that changes many times a second (here a tela.Value raised every 50 ms) moves the bar, its number and aria-valuenow without building the component.", col,
			Download{Label: tr("progress.download", "Download"), Start: tr("progress.start", "Start the download"), Again: tr("progress.again", "Download again"),
				Running: tr("progress.running", "Downloading..."), Format: percent}),
		demo("A tween", "A tela.UseTween float32 as the ValueFn eases the bar to each new target, a frame at a time.", col,
			Tween{Label: tr("progress.tween", "Disk cleanup"), Reset: tr("progress.reset", "Reset")}),
		demo("Units", "Max and FormatValue: what a screen reader reads is the formatted value.", col,
			Div(Class("flex w-full max-w-sm flex-col gap-2"),
				Div(Class("flex justify-between text-sm"), Span(Class("font-medium"), Text(tr("progress.storage", "Storage"))), Span(Class("text-muted-foreground"), Text(tr("progress.storageOf", "512 MB of 2 GB")))),
				kit.Progress{Value: 512, Max: 2048, AriaLabel: tr("progress.storage", "Storage"), FormatValue: func(v float64) string { return fmt.Sprintf("%.0f MB", v) }})),
		demo("Indeterminate", "No value: the bar pulses while the amount of work is unknown.", col,
			kit.Progress{Indeterminate: true, Class: "max-w-sm", AriaLabel: tr("progress.loading", "Loading")},
			kit.Progress{Value: 72, Class: "max-w-sm", IndicatorClass: "bg-emerald-500", AriaLabel: tr("progress.custom", "A custom color")}),
	)
}
