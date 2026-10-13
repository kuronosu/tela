// Command player is a music player: a card with cover art, title, artist, a
// progress bar, previous/play/next buttons, and a queue of tracks.
//
// The progress is driven by a goroutine ticker through tela.UseValue (see
// Transport): every tick moves the bar with tela.WidthFn and
// tela.TranslateFn, which lay out the fill and repaint the thumb without
// building a single component. The time label reads a second Value that
// changes once per second, so it builds once per second, not per tick.
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
// queue next to it, over a soft radial glow.
func Player() tela.Node { return playerAt(0) }

// playerAt is Player with the first track already startAt seconds in.
func playerAt(startAt float64) tela.Node {
	tr := useTransport(startAt)
	return Div(Class("flex h-screen items-center justify-center gap-6 overflow-hidden "+
		"bg-zinc-950 bg-radial-[at_25%_15%] from-violet-950/70 to-zinc-950 to-70% px-6 py-3 text-white"),
		tela.CP(NowPlaying, TransportProps{Tr: tr}),
		tela.CP(Queue, QueueProps{Tr: tr}),
	)
}

// Preview is the screen of `tela snapshot` and `tela inspect`: the player
// a minute and 24 seconds into the first track.
func Preview() tela.Node { return tela.C(func() tela.Node { return playerAt(84) }) }

func main() {
	tela.Run(tela.App{
		Title:  "Tela — player",
		AppID:  "dev.tela.player",
		Width:  1280,
		Height: 800,
		Root:   Player,
	})
}
