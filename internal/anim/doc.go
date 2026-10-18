// Package anim is Tela's animation layer (ARCHITECTURE sections 8 and 12):
// the pieces that turn time into values, without knowing the element tree.
// internal/core drives them with the frame clock (CSS transitions and the
// animate-* classes) and the public animation hooks are built on them.
//
// # Time
//
// Nothing here reads a clock. A [Ticker] advances its [Animator]s to the time
// of a frame, which the scheduler passes (the backend's frame clock), so a
// headless backend or a test controls every animation, and an app with no
// animator is idle: the Ticker never asks for a frame itself, its owner asks
// [Ticker.Active] after each frame (0% CPU when idle). An animator reports
// [Done], [Running] or [Endless] (an infinite animation: it needs frames, but
// nothing waits for it).
//
// # Easing
//
// A [Curve] maps linear progress to eased progress: [CubicBezier] (the CSS
// keywords [Ease], [EaseIn], [EaseOut] and [EaseInOut], and Tailwind v4's
// tokens [TailwindEaseIn], [TailwindEaseOut], [TailwindEaseInOut]), [Steps]
// and [Linear]. [ParseEasing] reads a CSS <easing-function>.
//
// # Springs
//
// A [Spring] is Motion's damped oscillator (stiffness, damping, mass,
// initial velocity) solved in closed form, with the duration/bounce
// parametrization ([SpringForDuration]) and rest detection ([Rest],
// [Spring.Duration]).
//
// # Interpolation
//
// Typed interpolation of the animatable values: [Lerp], [LerpLP], [LerpDim],
// [LerpColor] (premultiplied; sRGB for legacy colors, Oklab for the others,
// as Chromium does), [LerpTransform], [LerpShadows] (paired by Tailwind's
// slots), [LerpFilters]. [Prop] names the groups of ComputedStyle
// properties that transitions animate (the bits of style.TransitionProps)
// with [Equal], [CanInterpolate] and [Blend] over two styles, and
// [Prop.Class] says what a change invalidates.
//
// # Transitions and keyframes
//
// [Transition] holds the timing state of one CSS transition and the
// reversing rule of CSS Transitions Level 1 ([Transition.Reverse]); core
// implements the start, retarget and cancel rules on top of it. [Spec] is
// the animation shorthand ("spin 1s linear infinite") with the Web
// Animations timing model ([Spec.Progress]: delay, iterations, direction,
// fill), [Keyframes] a @keyframes rule sampled by [Keyframes.At], and
// [Builtin] has Tailwind's spin, ping, pulse and bounce.
//
// # Determinism
//
// Float code is fusion-safe (ADR-041): every product that feeds an addition
// is rounded explicitly and the exponential and trigonometric functions come
// from internal/fmath, so the same inputs give the same bits on every
// architecture and pixels do not depend on the CPU (ADR-027).
package anim
