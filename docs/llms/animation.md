# Recipe: animation

Questions: how to animate a value without rebuilding; a spring or a tween; a gradient that turns or fades; a drawer; an entrance, an exit; boxes that move when the layout changes; how to look at motion.

**Pick by what moves.**

- A style that changes with state (hover, focus, a variant): classes, `transition`, `transition-colors`, `duration-200`, `ease-out`; looping: `animate-spin`, `pulse`, `ping`, `bounce` (stagger with `delay-150` or `[animation-delay:-0.3s]`).
- An element that appears: `starting:` classes (the style it transitions from). One that leaves: `Presence(...)` around it and `Exit("opacity-0 translate-y-2")` on it; a component with an exit of its own (a draft to save) uses `tela.UsePresence()` and calls `done`.
- A number, color, point, size or rect that follows state: `tela.UseSpring(target)` (retargets from where it is, keeping its velocity) or `tela.UseTween(target, tela.Duration(300*time.Millisecond), tela.Ease("ease-out"))`; both return a `*tela.Value[T]`. `tela.Animate(v, target, opts...)` moves any `Value` from a handler. Options: `Stiffness`, `Damping`, `Bounce`, `Duration`, `Ease`, `Spring(...)`, `Initial`. Type the target (`float32(0)`).
- Boxes that move when the layout changes (a list reorders, a panel opens): `AnimateLayout()` on the container, and `LayoutID("id")` to follow an element across parents (the sliding pill of `kit.ToggleGroup.Indicator`).
- Reduced motion: when the system asks for it, transitions take no time and the `motion-reduce:` classes apply (`motion-safe:` for the rest); `tela.UseReducedMotion()` reads it for motion computed in code, and `tela mcp`'s `prefer {"reducedMotion": true}` tries it.

**Read a `Value` in a style closure, not in the build.** `v.Get()` in a build rebuilds the component on every frame of the animation. In a `...Fn` style prop the element only repaints (or relayouts, for `WidthFn`/`HeightFn`): `tela.TranslateFn`, `OpacityFn`, `WidthFn`, `HeightFn`, `BgFn`, `TextColorFn` and `tela.BackgroundGradientFn`; `TextFn` does it for text, `h.AriaValueNowFn` and `h.AriaValueTextFn` for a value in the accessibility tree.

```go
func Drawer(open bool) tela.Node {
	target := float32(-320)
	if open {
		target = 0
	}
	x := tela.UseSpring(target) // starts at the target, then springs to new targets
	return Div(Class("fixed inset-y-0 left-0 w-80 border-r border-border bg-background p-6 shadow-xl"),
		Style(tela.TranslateFn(func() (float32, float32) { return x.Get(), 0 })), // repaints only
		Text("Menu"))
}

func Download() tela.Node {
	progress := tela.UseValue(float32(0))
	tela.UseGo(func(ctx context.Context) {
		for i := 1; i <= 10 && ctx.Err() == nil; i++ {
			time.Sleep(100 * time.Millisecond)
			progress.Set(float32(i) / 10)
		}
	}, tela.Once)
	return Div(Class("h-2 w-64 overflow-hidden rounded-full bg-muted"),
		Div(Class("h-full bg-primary"), Style(tela.WidthFn(func() float32 { return 256 * progress.Get() }))))
}
```

**A gradient that moves (`tela.BackgroundGradientFn`).** The function returns a `tela.Gradient` (`tela.LinearGradient(angleDeg, tela.Stop(color)...)`; 0 points up, 90 right, 180 down, colors blend in Oklab) and is read at paint time, so turning or recoloring it repaints only that box. It is a layer over the element's other background images (`bg-linear-*`, `bg-[url(...)]`, which stay under it; add `bg-none` to have it alone), and the background color, size, position and repeat classes still apply. A gradient without stops paints nothing.

```go
func Aurora(angle float32) tela.Node {
	from, to := tela.Hex("#6366f1"), tela.Hex("#ec4899")
	a := tela.UseTween(angle, tela.Duration(600*time.Millisecond)) // degrees; a float32
	return Div(Class("h-32 rounded-xl"), Style(tela.BackgroundGradientFn(func() tela.Gradient {
		return tela.LinearGradient(a.Get(), tela.Stop(from), tela.Stop(to))
	})))
}
```

**Looking at motion.** `tela snapshot` renders one frame. `tela mcp`'s `film {"span": 600, "action": {"click": "e5"}}` starts the motion with an action and returns frames of it on the app's clock as a contact sheet (deterministic; `apng` for an animated PNG), and `tela snapshot --film SPAN` does it for a screen that animates by itself. In a test, the clock is manual: `app.Advance(150 * time.Millisecond)` between assertions, or `telatest.NoSettle()` to capture each frame, and `app.ExpectIdle()` checks that nothing keeps running when the motion is over.
