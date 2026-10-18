package core

import (
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/anim"
	"github.com/kuronosu/tela/internal/style"
)

// Animations (ARCHITECTURE section 12): implicit CSS transitions and the
// keyframe animations of the animate-* classes, on the frame clock.
//
// # Model
//
// A box's computed style is the style its classes and props give it (the
// target: Compute's result, never changed afterwards). While a box
// animates, the style the pipeline uses (Box.cs) is a copy of the target
// with the animated properties replaced by their value at the time of the
// frame. The restyle that follows a change starts the transitions (below);
// the animation phase of the pipeline, Tree.Animate (phase 2), advances the
// running ones to the time of the frame (the Host passes the FrameEvent's
// time, so a headless clock controls everything) and installs the new
// style. A Tree whose frames carry no clock (Render, or Frame called
// without Animate) has no transitions: styles change at once.
//
// # Transitions
//
// A restyle compares the style before the change (the used style, which has
// the current value of a running transition) with the style after it, for
// each property of the new style's transition-property, and follows CSS
// Transitions Level 1, section 3: a changed property starts a transition
// from the current value with the new duration, timing function and delay;
// a running transition whose end value is still the target keeps going; one
// whose target changes is replaced by a transition from the current value,
// or, when the new target is the value the running one started from, by a
// shorter reversing one (anim.Transition.Reverse). A box that mounts does
// not transition. With reduced motion (the system setting or
// Config.ReducedMotion) transitions take no time: none starts and running
// ones end.
//
// Inherited properties (color, font size) are the exception to "a restyle
// compares": a child restyled because its parent's inherited value changed
// follows the parent's current value and starts no transition for it.
//
// # What a frame of an animation invalidates
//
//   - opacity and transform (anim.ClassLayer): the box is a layer (a
//     deferred stacking context, layer.go) for as long as it animates, even
//     when its value passes through the identity. A tick marks the box for
//     painting (markPaint): no restyle, build or layout, only the
//     compositor's effects change, and damage limits the raster to the
//     box's old and new areas.
//   - colors and the border radius (ClassPaint): the layers are recorded
//     again without placing the boxes.
//   - shadows and filters (ClassInk): the boxes are placed again too, since
//     what the box paints extends beyond its border box.
//   - inherited properties, sizes, spacing, insets and border widths
//     (ClassLayout, and the colors that children inherit): the box is styled
//     again every frame (stylePhase), which lays out and restyles the
//     children when needed.
//
// # Frames and idle
//
// After each frame, Tree.afterFrame asks the platform for another while an
// animation runs (animAfterFrame); when the last transition ends no more
// frames are requested and the app is idle. Infinite animations keep
// asking for as long as they are mounted. A keyframe animation on a box
// hidden with display: none (or inside such a box) is dormant: it asks for
// no frames, and starts again from the beginning when the box is shown, as
// in CSS. Boxes that are visible but off screen or scrolled out of view
// keep animating (the frames paint nothing and rasterize nothing).
// Tree.OnlyEndlessAnimations lets a test runner tell that all the frames
// asked for belong to infinite animations.

// animState is the animation state of a Tree.
type animState struct {
	tk anim.Ticker
	// driven reports that Animate was called: frames carry a clock, so
	// transitions run.
	driven bool
	// inherited is set while the children of a restyled box are styled:
	// they follow its inherited properties instead of transitioning them.
	inherited bool
	// dormant are the boxes with keyframe animations that wait to be shown.
	dormant []*boxAnim
	// specs caches the parsed animation shorthands.
	specs map[string]specEntry
}

type specEntry struct {
	spec anim.Spec
	kfs  *anim.Keyframes
	err  string
}

// boxAnim is the animation state of one box: its running transitions and
// keyframe animation. It is an anim.Animator.
type boxAnim struct {
	bx *Box
	// target is the style the box would have without animations: the
	// result of the last restyle. Nil when nothing runs.
	target *style.ComputedStyle
	trans  []boxTransition
	kf     *kfRun
	// layer: an animation of opacity or transform is in progress, so the
	// box is a stacking context and a layer of its own (stacking).
	layer bool
	// dormant: the keyframes wait for the box to be displayed.
	dormant bool
}

// boxTransition is a running transition with the styles it interpolates
// between. The end value is the target's.
type boxTransition struct {
	anim.Transition
	// from is the style before the change (immutable), radj the style
	// whose value is the reversing-adjusted start value.
	from, radj *style.ComputedStyle
}

// kfRun is a running keyframe animation.
type kfRun struct {
	raw   string
	spec  anim.Spec
	kfs   *anim.Keyframes
	start time.Duration
	// finished: the animation ended. It stays (with its raw shorthand) so
	// that a restyle does not start it again; a forwards fill keeps
	// applying its last keyframe.
	finished bool
}

var animLayerDeferred = style.Deferred{Phase: style.PhasePaint, Apply: func(*style.ComputedStyle) {}}

// animCurve converts a style's cubic-bezier to an anim curve.
func animCurve(c style.CubicBezier) anim.Curve {
	return anim.CubicBezier{X1: float64(c.X1), Y1: float64(c.Y1), X2: float64(c.X2), Y2: float64(c.Y2)}
}

func (ba *boxAnim) find(p anim.Prop) int {
	for i := range ba.trans {
		if ba.trans[i].Prop == p {
			return i
		}
	}
	return -1
}

func (ba *boxAnim) remove(i int) {
	ba.trans = slices.Delete(ba.trans, i, i+1)
}

// empty reports that the box has no animation to keep.
func (ba *boxAnim) empty() bool { return len(ba.trans) == 0 && ba.kf == nil }

// stateOf returns what the ticker should do with the box's animations once
// the finished ones were pruned (animStyle).
func (ba *boxAnim) stateOf() anim.State {
	if len(ba.trans) > 0 {
		return anim.Running
	}
	if k := ba.kf; k != nil && !k.finished {
		if k.spec.Endless() {
			return anim.Endless
		}
		return anim.Running
	}
	return anim.Done
}

// stateAt is stateOf for a box whose finished animations were not pruned.
func (ba *boxAnim) stateAt(now time.Duration) anim.State {
	for i := range ba.trans {
		if now < ba.trans[i].End() {
			return anim.Running
		}
	}
	if k := ba.kf; k != nil && !k.finished {
		if k.spec.Endless() {
			return anim.Endless
		}
		if end, _ := k.spec.End(); now-k.start < end {
			return anim.Running
		}
	}
	return anim.Done
}

// release forgets the animation state of a box that has none left.
func (ba *boxAnim) release() {
	ba.target, ba.layer = nil, false
	if ba.empty() && !ba.dormant {
		ba.bx.anim = nil
	}
}

// animRestyle is called by restyle with the box's old style (nil for a new
// box) and the style just computed, the target. It starts, retargets and
// cancels the animations of the box and returns the style the box uses
// now: cs itself when nothing animates.
func (t *Tree) animRestyle(el *Element, bx *Box, old, cs *style.ComputedStyle) *style.ComputedStyle {
	ba := bx.anim
	if ba == nil && cs.Animation == "" && !t.mayTransition(old, cs) {
		return cs // the common case: nothing animates
	}
	if ba == nil {
		ba = &boxAnim{bx: bx}
		bx.anim = ba
	}
	now := t.anim.tk.Now()
	if old != nil {
		t.updateTransitions(ba, old, cs, now)
	}
	t.updateKeyframes(ba, el, cs, now)
	if ba.empty() && !ba.dormant {
		t.anim.tk.Remove(ba)
		ba.release()
		return cs
	}
	ba.target = cs
	u := t.animStyle(ba, cs, now)
	if ba.stateOf() == anim.Done {
		t.anim.tk.Remove(ba)
		ba.release()
		return u
	}
	if t.anim.driven {
		t.anim.tk.Add(ba) // a tree without a clock shows the first state
	}
	return u
}

// mayTransition reports whether the change from old to cs may start a
// transition (a cheap test, so that boxes that only have a transition class
// allocate nothing when they restyle).
func (t *Tree) mayTransition(old, cs *style.ComputedStyle) bool {
	tr := cs.Transition
	if old == nil || tr.Properties == 0 || !t.anim.driven || t.cx.ReducedMotion {
		return false
	}
	if !(anim.Timing{Duration: tr.Duration, Delay: tr.Delay}).Runs() {
		return false
	}
	for p := anim.Prop(0); p < anim.NumProps; p++ {
		if tr.Properties&p.Mask() != 0 && !anim.Equal(p, old, cs) {
			return true
		}
	}
	return false
}

// updateTransitions implements CSS Transitions Level 1, 3.1 to 3.3, for the
// style change from old (the style before the change, which has the
// current value of a running transition) to cs.
func (t *Tree) updateTransitions(ba *boxAnim, old, cs *style.ComputedStyle, now time.Duration) {
	tr := cs.Transition
	enabled := t.anim.driven && !t.cx.ReducedMotion
	timing := anim.Timing{Duration: tr.Duration, Delay: tr.Delay, Curve: animCurve(tr.Timing)}
	for p := anim.Prop(0); p < anim.NumProps; p++ {
		i := ba.find(p)
		if tr.Properties&p.Mask() == 0 || !enabled {
			// No matching transition-property (or no time): a running
			// transition is cancelled and the property takes its value.
			if i >= 0 {
				ba.remove(i)
			}
			continue
		}
		inherited := t.anim.inherited && p.Inherited()
		if t.anim.inherited && followsColor(p) && old.Text.Color != cs.Text.Color {
			// A child whose text color follows its parent's: the colors
			// that are currentColor follow it too.
			inherited = true
		}
		if i < 0 {
			if inherited {
				continue
			}
			// The before-change value of a property that does not
			// transition is its value in the previous target (the used
			// style also has keyframe values).
			before := old
			if ba.target != nil {
				before = ba.target
			}
			if !anim.Equal(p, before, cs) && anim.CanInterpolate(p, before, cs) && timing.Runs() {
				ba.trans = append(ba.trans, boxTransition{Transition: anim.Begin(p, timing, now), from: before, radj: before})
			}
			continue
		}
		run := &ba.trans[i]
		if ba.target != nil && anim.Equal(p, ba.target, cs) {
			continue // the target did not change: it keeps going
		}
		switch {
		case inherited, anim.Equal(p, old, cs), !anim.CanInterpolate(p, old, cs), !timing.Runs():
			ba.remove(i)
		case anim.Equal(p, run.radj, cs):
			// Back to where it came from: a shorter transition.
			rev := run.Reverse(timing, now)
			ba.trans[i] = boxTransition{Transition: rev, from: old, radj: ba.target}
		default:
			ba.trans[i] = boxTransition{Transition: anim.Begin(p, timing, now), from: old, radj: old}
		}
	}
}

// followsColor reports whether p is a color property whose value is the
// text color when its style says currentColor (borders, outlines and
// decorations): Tela resolves it when the style is computed.
func followsColor(p anim.Prop) bool {
	return p == anim.PropBorderColor || p == anim.PropOutlineColor || p == anim.PropDecorationColor
}

// updateKeyframes starts, keeps or cancels the keyframe animation of the
// box for the animation shorthand of cs.
func (t *Tree) updateKeyframes(ba *boxAnim, el *Element, cs *style.ComputedStyle, now time.Duration) {
	raw := cs.Animation
	if raw == "" {
		ba.kf, ba.dormant = nil, false
		return
	}
	if ba.dormant {
		if !t.shown(el, cs) {
			return // still hidden
		}
		ba.dormant = false
	}
	if ba.kf != nil && ba.kf.raw == raw {
		return
	}
	e := t.animSpec(raw)
	if e.err != "" {
		if !t.reported[e.err] {
			t.reported[e.err] = true
			t.logf("tela: %s (in the animation of a <%v>)", e.err, ba.bx.kind)
		}
		ba.kf = nil
		return
	}
	ba.kf = &kfRun{raw: raw, spec: e.spec, kfs: e.kfs, start: now}
}

// animSpec parses (once) an animation shorthand and resolves its keyframes.
func (t *Tree) animSpec(raw string) specEntry {
	if e, ok := t.anim.specs[raw]; ok {
		return e
	}
	var e specEntry
	spec, err := anim.ParseSpec(raw)
	switch {
	case err != nil:
		e.err = `invalid animation "` + raw + `": ` + err.Error()
	case spec.Name == "":
		e.err = `the animation "` + raw + `" names no keyframes; use animate-spin, animate-ping, animate-pulse or animate-bounce`
	default:
		kfs, ok := anim.Builtin(spec.Name)
		if !ok {
			e.err = `unknown keyframes "` + spec.Name + `" in the animation "` + raw + `"; the built-in ones are spin, ping, pulse and bounce`
			break
		}
		e.spec, e.kfs = spec, kfs
	}
	if t.anim.specs == nil {
		t.anim.specs = map[string]specEntry{}
	}
	t.anim.specs[raw] = e
	return e
}

// animStyle returns the style of the box at now: a copy of target with the
// running animations applied, or target when none applies. Finished
// transitions and keyframe animations are marked or removed from ba, and
// ba.layer is updated.
func (t *Tree) animStyle(ba *boxAnim, target *style.ComputedStyle, now time.Duration) *style.ComputedStyle {
	u := *target
	over, layer := false, false
	for i := 0; i < len(ba.trans); {
		tr := &ba.trans[i]
		eased, done := tr.Progress(now)
		if done {
			ba.remove(i)
			continue
		}
		anim.Blend(tr.Prop, &u, tr.from, target, eased)
		over = true
		layer = layer || tr.Prop.Class() == anim.ClassLayer
		i++
	}
	if k := ba.kf; k != nil {
		holds := k.spec.Fill == anim.FillForwards || k.spec.Fill == anim.FillBoth
		if !k.finished || holds {
			p, phase, applies := k.spec.Progress(now - k.start)
			if phase == anim.PhaseAfter {
				k.finished = true
			}
			layer = layer || !k.finished
			if applies {
				v := k.kfs.At(p, k.spec.Curve, u.Opacity)
				if v.HasOpacity {
					u.Opacity = v.Opacity
				}
				if v.HasTransform {
					u.Transform = anim.ComposeTransform(u.Transform, v.Transform)
				}
				over = true
			}
		}
	}
	ba.layer = layer
	if layer {
		u.Deferred = append(slices.Clip(target.Deferred), animLayerDeferred)
		over = true
	}
	if !over {
		return target
	}
	return &u
}

// Advance implements anim.Animator: it moves the box's animations to now
// and installs the style they give.
func (ba *boxAnim) Advance(now time.Duration) anim.State {
	bx := ba.bx
	t := bx.tree
	el := bx.el
	if el == nil || !el.mounted || bx.cs == nil || ba.target == nil {
		return anim.Done
	}
	if k := ba.kf; k != nil && !k.finished && !t.rendered(el) {
		// Hidden by display: none: the keyframes wait to be shown.
		ba.kf, ba.dormant = nil, true
		t.anim.dormant = append(t.anim.dormant, ba)
	}
	// Properties that other boxes inherit or that change the layout need
	// the whole style phase, which also applies the end values.
	for i := range ba.trans {
		if p := ba.trans[i].Prop; p.Inherited() || p.Class() == anim.ClassLayout {
			t.markStyle(el)
			return ba.stateAt(now)
		}
	}
	old := bx.cs
	u := t.animStyle(ba, ba.target, now)
	t.animApply(bx, old, u)
	state := ba.stateOf()
	if state == anim.Done {
		ba.release()
	}
	return state
}

// animApply installs u, the style of a frame of an animation, as the box's
// used style, invalidating only what its difference from old needs.
func (t *Tree) animApply(bx *Box, old, u *style.ComputedStyle) {
	if old == u {
		return
	}
	bx.cs = u
	var layerCh, paintCh, inkCh bool
	for p := anim.Prop(0); p < anim.NumProps; p++ {
		if p.Inherited() || p.Class() == anim.ClassLayout || anim.Equal(p, old, u) {
			continue
		}
		switch p.Class() {
		case anim.ClassLayer:
			layerCh = true
		case anim.ClassInk:
			inkCh = true
		default:
			paintCh = true
		}
	}
	wasDeferred := bx.deferPaint
	bx.deferPaint = hasPhase(u, style.PhasePaint)
	if !bx.deferPaint {
		dropObserver(bx.paintObs)
	}
	if bx.fld != nil {
		t.fieldRestyled(bx.fld, old, u)
	}
	L := &t.lay
	switch {
	case wasDeferred != bx.deferPaint, inkCh:
		// The box starts or stops being a layer, or paints beyond its
		// border box: place and record again.
		L.needFull, L.needPlace = true, true
		t.repaint = true
	case paintCh:
		L.needFull = true
		t.repaint = true
	}
	if bx.pcs != nil && !bx.deferPaint {
		bx.pcs = u
	}
	if bx.deferPaint && (layerCh || paintCh || inkCh) {
		t.markPaint(bx)
	}
	if !bx.deferPaint && layerCh {
		t.repaint = true
	}
}

// animateStyles is the style part of the animation phase: it advances the
// transitions and keyframe animations to now, the time of the frame.
func (t *Tree) animateStyles(now time.Duration) {
	t.anim.driven = true
	// The marks of the animations belong to the frame the Host runs next:
	// they do not ask for another.
	was := t.inFrame
	t.inFrame = true
	t.anim.tk.Advance(now)
	t.inFrame = was
}

// animAfterFrame asks for the next frame while an animation runs, and wakes
// the keyframe animations of boxes that were shown.
func (t *Tree) animAfterFrame() {
	if len(t.anim.dormant) > 0 {
		t.wakeDormant()
	}
	if t.anim.tk.Active() {
		t.requestFrame()
	}
}

// requestFrame asks the platform for a frame without marking any work.
func (t *Tree) requestFrame() {
	if t.asked || t.onDirty == nil {
		return
	}
	t.asked = true
	t.onDirty()
}

// wakeDormant restarts the keyframe animations of the boxes that are
// displayed again.
func (t *Tree) wakeDormant() {
	live := t.anim.dormant[:0]
	for _, ba := range t.anim.dormant {
		bx := ba.bx
		switch {
		case bx.el == nil || !bx.el.mounted || !ba.dormant:
		case t.rendered(bx.el):
			t.markStyle(bx.el) // the restyle starts the animation again
		default:
			live = append(live, ba)
		}
	}
	clear(t.anim.dormant[len(live):])
	t.anim.dormant = live
}

// rendered reports whether the element and its ancestors are displayed
// (none of them is display: none).
func (t *Tree) rendered(el *Element) bool {
	return t.shown(el, el.box.cs)
}

// shown is rendered for an element whose own style is cs (the one about to
// be installed).
func (t *Tree) shown(el *Element, cs *style.ComputedStyle) bool {
	if cs == nil || cs.Display == style.DisplayNone {
		return false
	}
	for e := el.styleParent(); e != nil; e = e.styleParent() {
		if e.box == nil || e.box.cs == nil || e.box.cs.Display == style.DisplayNone {
			return false
		}
	}
	return true
}

// OnlyEndlessAnimations reports that every animation that needs frames is
// infinite and nothing else does: the frames an app asks for belong to
// spinners and pulses, which nothing waits for (telatest does not wait for
// them).
func (t *Tree) OnlyEndlessAnimations() bool {
	return t.anim.tk.OnlyEndless() && !t.changed && !t.repaint && !t.scrollAnimating()
}

// Animating reports whether an animation or transition is running or
// waiting for a frame.
func (t *Tree) Animating() bool { return t.anim.tk.Active() }

// dropAnim removes the animations of a box that leaves the tree.
func (t *Tree) dropAnim(bx *Box) {
	if ba := bx.anim; ba != nil {
		t.anim.tk.Remove(ba)
		ba.target, ba.layer, ba.dormant = nil, false, false
		bx.anim = nil
	}
}
