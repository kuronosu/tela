# Animation

Everything in Tela animates on the **frame clock**: the backend's vsync in a
window, a manual clock in tests. An app that is not animating requests no
frames (0% CPU), and with the system's reduced-motion setting (or
[`telatest.ReducedMotion`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#ReducedMotion)) transitions take no time and springs and tweens
jump to their targets.

There are three tools, from the most declarative to the most manual:

| You want | Use |
|---|---|
| A style change to animate (hover, state, dark mode) | Tailwind's `transition`, `duration-*`, `ease-*`, `delay-*` classes |
| An element to animate in when it appears | The `starting:` variant |
| An element to animate out before it disappears | [`h.Presence`](https://pkg.go.dev/github.com/kuronosu/tela/h#Presence) and [`h.Exit`](https://pkg.go.dev/github.com/kuronosu/tela/h#Exit) |
| A number, color, point, size or rect that follows a target | [`tela.UseSpring`](https://pkg.go.dev/github.com/kuronosu/tela#UseSpring), [`tela.UseTween`](https://pkg.go.dev/github.com/kuronosu/tela#UseTween) |
| To animate a value from a handler, an effect or a goroutine | [`tela.Animate`](https://pkg.go.dev/github.com/kuronosu/tela#Animate) |

## Transitions

Transitions follow CSS: when a property listed by the element's
`transition-*` class changes, it animates from its current value to the new
one, with the element's duration, easing and delay.

```go
Button(Class("rounded-lg bg-indigo-600 px-4 py-2 text-white transition duration-200 hover:bg-indigo-500 active:scale-95"),
	Text("Save"))
```

A transition interrupted by another change starts from where it is; going
back to where it came from takes only as long as the way back (CSS's
reversing rule). Opacity and transforms are the cheapest: their frames only
composite the element. Colors repaint it, sizes and spacing lay it out.
`animate-spin`, `animate-ping`, `animate-pulse` and `animate-bounce` run
Tailwind's keyframes.

## Entering: `starting:`

An element that starts being rendered (it mounts, or its `hidden` /
`display: none` ends) does not transition: it has no previous style. The
`starting:` variant (CSS `@starting-style`, Tailwind v4) gives it one: its
first style is computed with the `starting:` classes, and its transitions run
from there to its normal style.

```go
If(open, Div(Class("rounded-xl bg-white p-6 shadow-xl transition duration-200 ease-out "+
	"opacity-100 scale-100 starting:opacity-0 starting:scale-95"),
	Text("Hello")))
```

As with every class, the later one wins: write the `starting:` classes after
the classes they override. It works anywhere, inside a `Presence` or not.

## Exiting: `Presence` and `Exit`

A removed element is normally gone at once. Inside
[`h.Presence`](https://pkg.go.dev/github.com/kuronosu/tela/h#Presence), a child that a build removes stays mounted while it
exits, then unmounts (Motion's `AnimatePresence`). The
[`h.Exit`](https://pkg.go.dev/github.com/kuronosu/tela/h#Exit) classes of its elements apply on top of their classes, and their
transitions run to them:

```go
Presence(
	If(open, Div(Key("dialog"), Role("dialog"),
		Class("transition duration-200 ease-out opacity-100 scale-100 starting:opacity-0 starting:scale-95"),
		Exit("opacity-0 scale-95"),
		...)),
)
```

- Children are matched by key, or by position (`h.If` keeps it). Several can
  exit at once.
- The exit ends when the transitions of the elements with `Exit` classes end
  (at once without a transition, or with reduced motion). A child with no
  `Exit` classes and no [`tela.UsePresence`](https://pkg.go.dev/github.com/kuronosu/tela#UsePresence) goes at once.
- An exiting child **keeps its place in the layout** (Motion's default): put
  dialogs and toasts in an absolutely positioned overlay so the page does not
  wait for them.
- It is **inert**: the pointer goes through it, it cannot keep or take the
  focus (a focus scope inside it ends and restores the focus), and it is gone
  from the aria snapshot and from `telatest` locators.
- Adding it back while it exits cancels the exit: its transitions run back
  from where they are.
- It shows its last build; its own components still build when their state
  changes.

### Manual exits: `UsePresence`

A component inside a `Presence` child can run its own exit:
[`tela.UsePresence`](https://pkg.go.dev/github.com/kuronosu/tela#UsePresence) reports `present == false` while the child exits, and the
child waits until every such component calls `done` (and for the `Exit`
transitions, if any):

```go
func Toast() tela.Node {
	present, done := tela.UsePresence()
	target := float32(1)
	if !present {
		target = 0
	}
	shown := tela.UseTween(float32(0), target, tela.Duration(250*time.Millisecond))
	if !present && shown.Get() == target {
		done() // the tween arrived: unmount
	}
	return Div(Role("status"), Class("rounded-xl bg-slate-900 px-5 py-3 text-white"),
		Style(tela.OpacityFn(shown.Get)), Text("Saved"))
}

// Presence(If(saved, tela.C(Toast)))
```

Reading `shown.Get()` in the body makes the component build on every frame of
its exit, and only then; the rest of the time the tween is read by
`OpacityFn` only.

## Springs and tweens: `UseSpring`, `UseTween`

[`tela.UseSpring`](https://pkg.go.dev/github.com/kuronosu/tela#UseSpring)`(initial, target, opts...)` returns a [`tela.Value`](https://pkg.go.dev/github.com/kuronosu/tela#Value) that follows
`target` with a spring: when a build passes a new target, the spring moves
there from where the value is, **keeping its velocity**, so interrupting it
turns smoothly. [`tela.UseTween`](https://pkg.go.dev/github.com/kuronosu/tela#UseTween) does the same with a fixed duration and easing.
On the first build the value is `initial` and moves to `target` if they differ.

```go
func Drawer(open bool) tela.Node {
	target := float32(-320)
	if open {
		target = 0
	}
	x := tela.UseSpring(float32(-320), target, tela.Stiffness(300), tela.Damping(30))
	return Div(Class("fixed inset-y-0 left-0 w-80 bg-white shadow-xl"),
		Style(tela.TranslateFn(func() (float32, float32) { return x.Get(), 0 })))
}
```

Read the value in a deferred style closure (`TranslateFn`, `OpacityFn`,
`WidthFn`, `HeightFn`, `h.TextFn`) and the animation runs without building the
component: a `TranslateFn` or `OpacityFn` frame only composites, a `WidthFn`
frame only lays out. Reading it in the body builds the component on every
frame.

The animated types ([`tela.Animatable`](https://pkg.go.dev/github.com/kuronosu/tela#Animatable)) are `float32`, [`tela.Color`](https://pkg.go.dev/github.com/kuronosu/tela#Color)
(interpolated like CSS transitions: in sRGB between hex colors, in Oklab when
an Oklch color is involved), [`tela.Point`](https://pkg.go.dev/github.com/kuronosu/tela#Point), [`tela.Size`](https://pkg.go.dev/github.com/kuronosu/tela#Size) and [`tela.Rect`](https://pkg.go.dev/github.com/kuronosu/tela#Rect)
(component by component, each with its own velocity).

## Imperative: `Animate`

[`tela.Animate`](https://pkg.go.dev/github.com/kuronosu/tela#Animate)`(v, target, opts...)` animates any [`UseValue`](https://pkg.go.dev/github.com/kuronosu/tela#UseValue) (or the Value of
`UseSpring` and `UseTween`) from a handler, an effect or a goroutine. It is a
spring by default, a tween when given only `Duration` and `Ease`:

```go
y := tela.UseValue[float32](0)
Button(OnClick(func(tela.MouseEvent) { tela.Animate(y, 200, tela.Spring(tela.Bounce(0.25))) }), Text("Drop"))
tela.Animate(y, 0, tela.Duration(300*time.Millisecond), tela.Ease("ease-in-out"))
```

A new `Animate` on the same Value interrupts the running one from where it is
(a spring keeps the velocity: a drag that animates to the pointer on every
move, then springs back on release, never jumps). `Set` and `Update` stop it.
Like `Set`, `Animate` is safe from any goroutine; an animation applied between
two frames starts at the next one.

## Options

| Option | Applies to | Meaning |
|---|---|---|
| [`tela.Stiffness`](https://pkg.go.dev/github.com/kuronosu/tela#Stiffness)(k), [`tela.Damping`](https://pkg.go.dev/github.com/kuronosu/tela#Damping)(c) | springs | The spring's physics (defaults 100 and 10, Motion's). `Damping(2*sqrt(k))` does not overshoot |
| [`tela.Bounce`](https://pkg.go.dev/github.com/kuronosu/tela#Bounce)(b) | springs | A spring by its bounciness, 0 to 1 (with `Duration`, default 800 ms) |
| [`tela.Duration`](https://pkg.go.dev/github.com/kuronosu/tela#Duration)(d) | both | A tween's duration (default 300 ms), or a spring's (default bounce 0.3) |
| [`tela.Ease`](https://pkg.go.dev/github.com/kuronosu/tela#Ease)(css) | tweens | A CSS easing: `linear`, `ease` (the default), `ease-in`, `ease-out`, `ease-in-out`, `step-start`, `step-end`, `cubic-bezier(...)`, `steps(...)` |
| [`tela.Spring`](https://pkg.go.dev/github.com/kuronosu/tela#Spring)(opts...) | `Animate` | Makes it a spring with these options |

The keywords are CSS's curves; Tailwind's `ease-out` utility is
`Ease("cubic-bezier(0, 0, 0.2, 1)")`. Contradicting options (`Ease` on a
spring, `Stiffness` with `Duration`, spring options on `UseTween`) and invalid
easings panic in dev mode (`TELA_DEV=1`) with what to write; otherwise they are
ignored.

## Testing animations

Actions in [`telatest`](Testing) wait for animations to end, so a golden after an
action shows the end state. Mount with [`telatest.NoSettle`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#NoSettle) to capture the
frames in between with `Advance`:

```go
app := telatest.Mount(t, tela.C(Dialog), telatest.NoSettle())
app.Click(telatest.ByRole("button", "Open"))
app.Advance(100 * time.Millisecond)
app.Golden("dialog_100ms.png")
```

`examples/motion` shows all of it on one screen, with goldens of its frames.
