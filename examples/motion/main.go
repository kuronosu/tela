// Command motion shows Tela's explicit animations (ARCHITECTURE section
// 12): a dialog that enters with the starting: variant and leaves with
// h.Exit inside h.Presence, a toast that runs its own exit with
// tela.UsePresence and a tween that calls done (tela.OnDone), and a card
// that follows a drag through a spring (tela.Animate retargeted on every
// move, keeping its velocity) and springs back when it is released, with
// the pointer's release velocity (tela.Velocity).
package main

import (
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Motion is the example's screen.
func Motion() tela.Node {
	open, setOpen := tela.UseState(false)
	saved, setSaved := tela.UseState(false)
	return Div(Class("relative flex h-screen flex-col items-center gap-8 overflow-hidden bg-slate-100 p-10 font-sans"),
		H1(Class("text-2xl font-semibold text-slate-900"), Text("Motion")),
		Div(Class("flex gap-3"),
			Button(Class("rounded-lg bg-indigo-600 px-4 py-2 font-medium text-white shadow-sm transition-colors hover:bg-indigo-500"),
				OnClick(func(tela.MouseEvent) { setOpen.Set(true) }), Text("Open dialog")),
			Button(Class("rounded-lg bg-white px-4 py-2 font-medium text-slate-700 shadow-sm ring-1 ring-slate-200 transition-colors hover:bg-slate-50"),
				OnClick(func(tela.MouseEvent) { setSaved.Set(!saved) }), IfElse(saved, Text("Hide toast"), Text("Show toast"))),
		),
		tela.C(DragCard),
		Presence(If(saved, tela.C(Toast))),
		Presence(If(open, Div(Key("dialog"), Class("absolute inset-0 flex items-center justify-center"),
			// The backdrop and the panel enter and leave with their own
			// transitions; the dialog unmounts when both have ended.
			Div(Class("absolute inset-0 bg-slate-900/40 transition-opacity duration-200 starting:opacity-0"),
				Exit("opacity-0"), OnClick(func(tela.MouseEvent) { setOpen.Set(false) })),
			Div(Role("dialog"), AriaLabel("Changes saved"),
				Class("relative w-80 rounded-2xl bg-white p-6 shadow-2xl transition duration-200 ease-out "+
					"starting:translate-y-4 starting:scale-95 starting:opacity-0"),
				Exit("translate-y-4 scale-95 opacity-0"),
				H2(Class("text-lg font-semibold text-slate-900"), Text("Changes saved")),
				P(Class("mt-2 text-sm text-slate-500"), Text("Your profile is up to date. It will show everywhere in a minute.")),
				Div(Class("mt-6 flex justify-end"),
					Button(Class("rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white"),
						OnClick(func(tela.MouseEvent) { setOpen.Set(false) }), Text("Close"))),
			),
		))),
	)
}

// DragCard follows the pointer through a spring while it is dragged and
// springs back to its place when released. Every OnDrag retargets the
// spring, which keeps its velocity, so the card trails the pointer
// smoothly; the release starts the spring back with the pointer's
// velocity, so a fling carries the card on before it returns. The
// component never builds during the drag: the position is a Value read
// by a TranslateFn.
func DragCard() tela.Node {
	pos := tela.UseValue(tela.Point{})
	start := tela.UseRef(tela.Point{})
	return Div(Class("flex size-36 cursor-grab select-none items-center justify-center rounded-3xl "+
		"bg-linear-to-br from-sky-400 to-indigo-500 text-sm font-medium text-white shadow-xl active:cursor-grabbing"),
		Style(tela.TranslateFn(func() (float32, float32) { p := pos.Get(); return p.X, p.Y })),
		OnDragStart(func(tela.DragEvent) { start.Current = pos.Latest() }),
		OnDrag(func(e tela.DragEvent) {
			to := tela.Point{X: start.Current.X + e.OffsetX, Y: start.Current.Y + e.OffsetY}
			tela.Animate(pos, to, tela.Stiffness(700), tela.Damping(45))
		}),
		OnDragEnd(func(e tela.DragEvent) {
			tela.Animate(pos, tela.Point{}, tela.Spring(tela.Stiffness(250), tela.Damping(12)),
				tela.Velocity(tela.Point{X: e.VelocityX, Y: e.VelocityY}))
		}),
		Text("Drag me"),
	)
}

// Toast enters and leaves with its own tween: a manual exit. It tweens
// in when it mounts; while its Presence child exits, present is false and
// it tweens out, and the tween calls done when it arrives (OnDone; done
// does nothing while the toast is present). It never builds during
// either tween.
func Toast() tela.Node {
	present, done := tela.UsePresence()
	target := float32(1)
	if !present {
		target = 0
	}
	shown := tela.UseTween(target, tela.Initial(float32(0)), tela.Duration(250*time.Millisecond), tela.Ease("ease-out"), tela.OnDone(done))
	return Div(Class("absolute inset-x-0 bottom-8 flex justify-center"),
		Div(Role("status"), Class("rounded-xl bg-slate-900 px-5 py-3 text-sm text-white shadow-lg"),
			Style(tela.TranslateFn(func() (float32, float32) { return 0, 24 * (1 - shown.Get()) }),
				tela.OpacityFn(shown.Get)),
			Text("Saved to your profile")))
}

func main() {
	tela.Run(tela.App{Title: "Motion", Width: 720, Height: 540, Root: Motion})
}

// Preview is the screen of `tela snapshot` and `tela inspect`.
func Preview() tela.Node { return tela.C(Motion) }
