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
| An element to slide or grow to its new place when the layout changes | [`h.AnimateLayout`](https://pkg.go.dev/github.com/kuronosu/tela/h#AnimateLayout) |
| An element to grow out of another one (a tab underline, a thumbnail into a detail view) | [`h.LayoutID`](https://pkg.go.dev/github.com/kuronosu/tela/h#LayoutID) |

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
	// OnDone(done) ends the exit when the tween out arrives; done does
	// nothing while the toast is present.
	shown := tela.UseTween(float32(0), target, tela.Duration(250*time.Millisecond), tela.OnDone(done))
	return Div(Role("status"), Class("rounded-xl bg-slate-900 px-5 py-3 text-white"),
		Style(tela.OpacityFn(shown.Get)), Text("Saved"))
}

// Presence(If(saved, tela.C(Toast)))
```

The toast never builds during its tweens: `OpacityFn` reads the value, and
[`tela.OnDone`](https://pkg.go.dev/github.com/kuronosu/tela#OnDone) calls `done` when the exit tween comes to rest.

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
`BgFn`, `TextColorFn`, `WidthFn`, `HeightFn`, `h.TextFn`) and the animation
runs without building the component: a `TranslateFn` or `OpacityFn` frame only
composites, a `BgFn` or `TextColorFn` frame only repaints, a `WidthFn` frame
only lays out. Reading it in the body builds the component on every frame.

```go
bg := tela.UseSpring(tela.Hex("#ffffff"), target)
Div(Class("rounded-lg p-4"), Style(tela.BgFn(bg.Get)), Text("Selected"))
```

[`tela.TextColorFn`](https://pkg.go.dev/github.com/kuronosu/tela#TextColorFn) also recolors the text of the descendants that inherit
the element's color (those that set no text color of their own); colors the
classes derived from the text color (`currentColor` borders and outlines) and
the text of `Input` and `Textarea` keep their class color.

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
[`tela.Velocity`](https://pkg.go.dev/github.com/kuronosu/tela#Velocity) gives a spring its initial velocity, such as a drag's
release velocity, and [`tela.OnDone`](https://pkg.go.dev/github.com/kuronosu/tela#OnDone) runs code when the animation
arrives:

```go
OnDragEnd(func(e tela.DragEvent) {
	tela.Animate(pos, tela.Point{}, tela.Spring(),
		tela.Velocity(tela.Point{X: e.VelocityX, Y: e.VelocityY}),
		tela.OnDone(func() { setDocked.Set(true) }))
})
```

Like `Set`, `Animate` is safe from any goroutine; an animation applied between
two frames starts at the next one.

## Options

| Option | Applies to | Meaning |
|---|---|---|
| [`tela.Stiffness`](https://pkg.go.dev/github.com/kuronosu/tela#Stiffness)(k), [`tela.Damping`](https://pkg.go.dev/github.com/kuronosu/tela#Damping)(c) | springs | The spring's physics (defaults 100 and 10, Motion's). `Damping(2*sqrt(k))` does not overshoot |
| [`tela.Bounce`](https://pkg.go.dev/github.com/kuronosu/tela#Bounce)(b) | springs | A spring by its bounciness, 0 to 1 (with `Duration`, default 800 ms) |
| [`tela.Duration`](https://pkg.go.dev/github.com/kuronosu/tela#Duration)(d) | both | A tween's duration (default 300 ms), or a spring's (default bounce 0.3); 0 applies the target at once |
| [`tela.Ease`](https://pkg.go.dev/github.com/kuronosu/tela#Ease)(css) | tweens | A CSS easing: `linear`, `ease` (the default), `ease-in`, `ease-out`, `ease-in-out`, `step-start`, `step-end`, `cubic-bezier(...)`, `steps(...)` |
| [`tela.Velocity`](https://pkg.go.dev/github.com/kuronosu/tela#Velocity)(v) | springs | The initial velocity, in the value's units per second, per component (not for colors); it replaces the velocity of an interrupted animation |
| [`tela.Spring`](https://pkg.go.dev/github.com/kuronosu/tela#Spring)(opts...) | `Animate` | Makes it a spring with these options |
| [`tela.OnDone`](https://pkg.go.dev/github.com/kuronosu/tela#OnDone)(fn) | both | Calls `fn` once, on the UI thread before the next build, when the animation comes to rest at its target; not when it is interrupted (another target, `Animate`, `Set`, `Update`, an unmount). With reduced motion or without a frame clock, right after the target applies |

The keywords are CSS's curves; Tailwind's `ease-out` utility is
`Ease("cubic-bezier(0, 0, 0.2, 1)")`. Contradicting options (`Ease` on a
spring, `Stiffness` with `Duration`, spring options or `Velocity` on
`UseTween`, a `Velocity` of a color or of another type than the value's) and
invalid easings panic in dev mode (`TELA_DEV=1`) with what to write; otherwise they are
ignored.

## Layout animations: `AnimateLayout` and `LayoutID`

[`h.AnimateLayout`](https://pkg.go.dev/github.com/kuronosu/tela/h#AnimateLayout)`()` animates every change of an element's laid-out box
(Motion's `layout`): a reorder, an expand, a sibling inserted or removed, a
breakpoint. The element is laid out at its new place at once and drawn moving
there from where it was, through a transform (FLIP), so the frames of the
animation only composite: no build, no layout.

```go
Map(tasks, func(t Task) tela.Node {
	return Div(Key(t.ID), AnimateLayout(), Class("rounded-xl bg-white p-3 shadow-sm"), Text(t.Title))
})
```

- **Timing:** a tween with the element's `duration-*` and `ease-*` classes
  (and `delay-*`) when it has one of the first two, like a transition;
  otherwise a spring (stiffness 500, damping 25, Motion's spring for `x` and
  `y`). A change during the animation starts from where the element is drawn,
  keeping its speed. (Motion's own default for layout is a 450 ms tween that
  restarts from rest; Tela's spring keeps interruptions smooth.)
- **Sizes** are drawn as a scale toward the new size, as in Motion. Rounded
  corners and box shadows are corrected so they keep their look, but text and
  children stretch while it runs, unless they have `AnimateLayout` too: then
  they keep their own size and their place in the parent. Put it on a card's
  header and body so the text stays crisp while the card grows, and give the
  card `overflow-hidden` so new content is revealed as it grows.
- **Nested:** a child with `AnimateLayout` that keeps its place inside a
  parent that moves moves with the parent.
- **What does not animate:** scrolling, transforms (`hover:scale-105`) and the
  display scale do not change the layout; an element that mounts or is shown
  appears at its place. With reduced motion and in `tela snapshot` there is no
  animation.
- While it moves, the pointer hits it where it is drawn, and
  `ElementRef.Bounds` / `tela.UseBounds` report the drawn box (like
  `getBoundingClientRect` under a CSS transform). Scrolling it into view uses
  its final place.

[`h.LayoutID`](https://pkg.go.dev/github.com/kuronosu/tela/h#LayoutID)`(id)` is a shared element transition (Motion's `layoutId`): when an
element with a LayoutID mounts in the same commit in which another with the
same ID unmounts (or while that one exits under a `Presence`), the new element
starts from the old one's box. The classic case is a tab underline that each
tab renders when it is selected:

```go
MapIndex(tabs, func(i int, label string) tela.Node {
	return Button(Key(label), Class("relative px-3 py-2"), OnClick(func(tela.MouseEvent) { setActive.Set(i) }),
		Text(label),
		If(i == active, Div(LayoutID("underline"), Class("absolute inset-x-0 bottom-0 h-0.5 bg-indigo-500"))))
})
```

IDs are global to the window. The old element is not moved: an exiting one
runs its own `Exit` classes (fade it out with `opacity-0` while the new one
grows from it). An element with a LayoutID also animates its own layout
changes. `examples/layout` shows a reorderable list, the tab underline and an
expanding card, with goldens of their frames.

## Testing animations

Actions in [`telatest`](Testing) wait for animations (layout animations included) to end, so a golden after an
action shows the end state. Mount with [`telatest.NoSettle`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#NoSettle) to capture the
frames in between with `Advance`:

```go
app := telatest.Mount(t, tela.C(Dialog), telatest.NoSettle())
app.Click(telatest.ByRole("button", "Open"))
app.Advance(100 * time.Millisecond)
app.Golden("dialog_100ms.png")
```

`examples/motion` shows all of it on one screen, with goldens of its frames.
