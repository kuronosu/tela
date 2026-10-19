# motion

Tela's explicit animations (ARCHITECTURE section 12, ADR-080) on one screen:

- **A dialog** that enters with the Tailwind v4 `starting:` variant (CSS `@starting-style`) and leaves with `h.Exit` inside `h.Presence`: the removed dialog stays mounted, inert, while its backdrop and panel run their transitions to the Exit classes, then unmounts.
- **A toast** that runs its own exit: `tela.UsePresence` reports `present == false` while its Presence child exits; the toast tweens out with `tela.UseTween`, whose `tela.OnDone(done)` ends the exit when the tween arrives.
- **A card** that follows a drag through a spring: every `OnDrag` calls `tela.Animate` with the pointer's position, which retargets the spring and keeps its velocity, so the card trails the pointer smoothly; the release animates it back to its place on a bouncy spring that starts with the pointer's release velocity (`tela.Velocity`). The position is a `tela.Value[tela.Point]` read by a `TranslateFn`, so the drag never builds the component.

Run it in a window: `CGO_ENABLED=0 go run ./examples/motion`.

`motion_test.go` captures frames of the three animations with `telatest.NoSettle` (the clock moves only with `App.Advance`): `testdata/dialog_enter_80ms.png`, `dialog_open.png`, `dialog_exit_80ms.png`, `toast_enter_100ms.png`, `toast_exit_100ms.png`, `drag_moving.png` (the card trailing the pointer) and `drag_release_120ms.png`, `drag_release_300ms.png` (the spring back, overshooting). Regenerate them with `TELA_UPDATE=1 CGO_ENABLED=0 go test ./examples/motion` and look at them before committing.
