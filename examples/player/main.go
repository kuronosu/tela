// Command player is a music player: a card with cover art, title, artist, a
// progress bar, previous/play/next buttons, and a queue of tracks.
//
// The progress is driven by a goroutine ticker through tela.UseValue (see
// Transport): every tick moves the bar with tela.WidthFn and
// tela.TranslateFn, which lay out the fill and repaint the thumb without
// building a single component. The time label is a tela.TextFn of a second
// Value that changes once per second, and the bar's width comes from
// tela.UseBounds, so nothing is hard-coded or built.
//
// Click or drag the bar to seek, click a queue row to play it. The icons
// are drawn with divs and borders.
//
// `tela snapshot ./examples/player` renders it without a window.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Player is the page: the player card and, from the lg breakpoint, the
// queue next to it, over a soft radial glow. StartAt is how many seconds
// into the first track the player starts (a prop that seeds its state, as
// in the preview below).
type Player struct {
	StartAt float64
}

func (p Player) Build() tela.Node { return page(useTransport(p.StartAt)) }

// page is the content of the player for a transport.
func page(tr *Transport) tela.Node {
	return Div(Class("flex min-h-screen items-center justify-center gap-6 overflow-hidden",
		"bg-zinc-950 bg-radial-[at_25%_15%] from-violet-950/70 to-zinc-950 to-70% px-6 py-3 text-white"),
		NowPlaying{Tr: tr},
		Queue{Tr: tr},
	)
}

// Preview is the screen of `tela snapshot` and `tela inspect`: the player
// a minute and 24 seconds into the first track.
func Preview() tela.Component { return Player{StartAt: 84} }

func main() {
	tela.Main(tela.App{
		Title:  "Tela — player",
		AppID:  "dev.telaui.player",
		Width:  1280,
		Height: 800,
		Root:   Player{},
	})
}
