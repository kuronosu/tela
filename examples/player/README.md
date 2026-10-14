# player

A music player: a card with generated cover art, title, artist, a seek bar with elapsed and total time, previous / play-pause / next buttons, and a queue of six tracks (from the `lg` breakpoint, 1024 px, up).

The point of the example is the **progress bar**. A goroutine ticker advances the progress ten times a second, and nothing is built on a tick:

- `Transport` (see `transport.go`) is the state of the player, made of `tela.UseValue`s created by the custom hook `useTransport`. The clock goroutine runs in a `UseEffect(..., nil)` and stops on unmount; each tick does `tela.Post(func() { tr.advance(dt) })`, so the update runs on the UI thread in order with the other updates.
- `Elapsed` (fractional seconds) is read only inside `tela.WidthFn` (the fill) and `tela.TranslateFn` (the thumb): a change lays out the fill and repaints the thumb, with no build and no style pass.
- `Secs` (whole seconds) is what the time label reads inside `TextFn`, so its paragraph is laid out once per second and nothing builds. `TestProgressAdvancesWithoutBuilding` checks exactly that: three seconds of music is no build at all.
- `Playing` and `Track` are read in component bodies, so a toggle rebuilds the controls and a new track rebuilds the card, the seek bar and the queue.

The rest: click or drag the seek bar (`OnClick`, `OnDragStart`, `OnDrag` with `e.LocalX` over the width the handler reads from the element, `ref.Bounds().Width`; the fill and the thumb read the same width from `tela.UseBounds`, so nothing hard-codes it), click a queue row to play it, `group-hover:` on the bar reveals the thumb, and the paused state shows the play triangle. The icons are drawn with divs: a play triangle is half of a square rotated 45 degrees and clipped by a half-width box, which keeps a rounded tip (the classic zero-size box with colored side borders draws a sharp triangle too).

The cover shrinks with the window height (`size-[clamp(96px,calc(100vh-310px),332px)]`), so the controls stay visible down to about 700x420.

## Run it

- In a window: `CGO_ENABLED=0 go run ./examples/player` (1280x800).
- Without a window: `CGO_ENABLED=0 go run ./cmd/tela snapshot --size 1280x800 --scale 2 -o player.png ./examples/player` (the preview is a minute and 24 seconds into the first track).
- Tests: `CGO_ENABLED=0 go test ./examples/player`. They run the clock faster (`tickEvery` and `timeScale` are package variables) and check the progress, pause (no frames while paused), next/previous/queue, seeking by click and drag, the end of a track, the goroutine stopping on unmount, and the goldens (`testdata/player_x1.png`, `player_x2.png`; regenerate with `TELA_UPDATE=1 CGO_ENABLED=0 go test -run TestGolden ./examples/player`).
