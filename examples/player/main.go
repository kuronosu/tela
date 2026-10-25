// Command player is a music player built on package kit: a card with cover
// art, title, artist, a seek bar (kit.Slider), previous/play/next buttons
// (kit.Button with kit.Tooltip), a mute kit.Toggle with a volume kit.Slider,
// and a queue of tracks (kit.Card, a kit.Button per row).
//
// The progress is driven by a goroutine ticker through tela.UseValue (see
// Transport): the seek bar is a kit.Slider whose ValueSource is that Value,
// so every tick repaints its knob and range without building or laying out
// a single component. The time label is a tela.TextFn of a second Value that
// changes once per second. Dragging the bar stores each position in the same
// Value, which costs a repaint as well.
//
// The look is kit's default style with the dark color scheme forced (a
// player is a dark app: App.ColorScheme) and the theme's tokens, so the
// colors are bg-card, text-muted-foreground and the like; the cover art and
// the violet accents are the example's own. Click or drag the bar to seek,
// click a queue row to play it. The icons are Lucide's (examples/player/icons).
//
// `tela snapshot ./examples/player` renders it without a window.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
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
	return Fragment(kit.StyleProvider{Style: playerStyle, Children: Div(
		Class("flex min-h-screen items-center justify-center gap-6 overflow-hidden",
			"bg-background bg-radial-[at_25%_15%] from-violet-950/70 to-background to-70% px-6 py-3 text-foreground"),
		NowPlaying{Tr: tr},
		Queue{Tr: tr},
	)})
}

// Preview is the screen of `tela snapshot` and `tela inspect`: the player
// a minute and 24 seconds into the first track.
func Preview() tela.Component { return Player{StartAt: 84} }

var theme = kit.Theme()

// App is the window: kit's theme, always dark.
var App = tela.App{
	Title:       "Tela — player",
	AppID:       "dev.telaui.player",
	Width:       1280,
	Height:      800,
	Theme:       &theme,
	ColorScheme: tela.Dark,
	RootClass:   "bg-background text-foreground",
	Root:        Player{},
}

func main() { tela.Main(App) }
