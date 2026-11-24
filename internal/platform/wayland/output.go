//go:build linux

package wayland

import (
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// output is a bound wl_output. Only its integer scale, the fallback of
// scaleFor, and the refresh rate of its current mode matter. Both arrive
// in events of their own and are applied on done (version 2+).
type output struct {
	b    *Backend
	name uint32 // registry name
	wl   *protocols.WlOutput
	outputProps
	pending outputProps // received, not applied yet
}

// outputProps are the properties of an output.
type outputProps struct {
	scale   int32
	refresh int32 // of the current mode, in mHz; unknown when not positive
}

func (b *Backend) addOutput(name, version uint32) {
	o := &output{b: b, name: name, outputProps: outputProps{scale: 1}, pending: outputProps{scale: 1}}
	o.wl = protocols.WlRegistryBind[protocols.WlOutput](b.registry, name, min(version, outputVersion))
	o.wl.SetScaleHandler(func(e protocols.WlOutputScaleEvent) {
		o.pending.scale = max(1, e.Factor)
		o.received()
	})
	o.wl.SetModeHandler(func(e protocols.WlOutputModeEvent) {
		// Some compositors list every mode of the output (deprecated by
		// the protocol), one of them flagged current.
		if e.Flags.Has(protocols.WlOutputModeCurrent) {
			o.pending.refresh = e.Refresh
			o.received()
		}
	})
	o.wl.SetDoneHandler(func(protocols.WlOutputDoneEvent) { o.apply() })
	b.outputs = append(b.outputs, o)
}

// received applies a property at once when no done event follows it
// (version 1).
func (o *output) received() {
	if o.wl.Version() < 2 {
		o.apply()
	}
}

// apply makes the properties received the output's. The refresh rate is
// read at each FrameEvent (Window.refresh), so its change needs no more.
func (o *output) apply() {
	scaled := o.pending.scale != o.scale
	o.outputProps = o.pending
	if !scaled {
		return
	}
	for _, w := range o.b.loop.Windows() {
		if slices.Contains(w.outputs, o) {
			w.updateScale()
		}
	}
}

// removeOutput handles the output's global going away.
func (b *Backend) removeOutput(o *output) {
	for _, w := range b.loop.Windows() {
		if i := slices.Index(w.outputs, o); i >= 0 {
			w.outputs = slices.Delete(w.outputs, i, i+1)
			w.updateScale()
		}
	}
	o.release()
}

func (o *output) release() {
	if o.wl.Destroyed() {
		return
	}
	if o.wl.Version() >= 3 {
		o.wl.Release()
	}
}

// outputFor returns the bound output behind a wl_output proxy.
func (b *Backend) outputFor(wl *protocols.WlOutput) *output {
	for _, o := range b.outputs {
		if o.wl == wl {
			return o
		}
	}
	return nil
}

// fractional reports whether windows scale through wp_viewporter, with
// wp_fractional_scale_v1 telling the scale: both globals are needed (the
// scale alone cannot be applied, and a viewport without it would only ever
// show integer scales, which set_buffer_scale already does).
func (b *Backend) fractional() bool { return b.fractionalScale != nil && b.viewporter != nil }

// canScale reports whether buffers can have a scale other than 1: through
// a viewport, or with wl_surface.set_buffer_scale (wl_compositor version
// 3+).
func (b *Backend) canScale() bool { return b.fractional() || b.compositor.Version() >= 3 }

// maxOutputScale is the largest scale of all outputs (1 without outputs).
// New windows start with it: guessing the HiDPI scale avoids a second
// configure on single-monitor HiDPI setups, which are the common case.
// With fractional scaling, compositors report wl_output.scale rounded up
// (2 for 1.5 on Hyprland and KWin), so the guess is replaced by the
// surface's preferred_scale when it arrives, often before the first
// configure.
func (b *Backend) maxOutputScale() int32 {
	s := int32(1)
	for _, o := range b.outputs {
		s = max(s, o.scale)
	}
	return s
}

// fractionalDenominator is the denominator of wp_fractional_scale_v1
// scales: preferred_scale sends the numerator.
const fractionalDenominator = 120

// fractionalScale converts a wp_fractional_scale_v1 preferred_scale to a
// scale: n/120, computed in float64 as platform.PhysicalSize expects (so
// that its rounding of halves matches the exact rational).
func fractionalScale(n uint32) float64 { return float64(n) / fractionalDenominator }

// scaleFor decides a window's scale:
//
//  1. wp_fractional_scale_v1.preferred_scale (fractional, in 120ths),
//     once the compositor sent it. Only windows that scale through a
//     viewport have the object (Backend.fractional). Like winit, the
//     integer sources below are then ignored: compositors send them
//     rounded up for older clients.
//  2. wl_surface.preferred_buffer_scale, once the compositor sent it
//     (wl_compositor version 6+). This is what the compositor wants;
//     Hyprland and others send ceil(fractional scale) here.
//  3. Otherwise the largest scale of the outputs the surface entered, like
//     Gio and pre-v6 toolkits (render at the highest density the window is
//     shown at; the compositor downscales on the other outputs).
//  4. Before the surface entered any output: current, the initial guess
//     (maxOutputScale) or the scale in use.
//
// A zero fractional or preferred value means "not received".
func scaleFor(fractional uint32, preferred int32, entered []*output, current float64) float64 {
	if fractional > 0 {
		return fractionalScale(fractional)
	}
	if preferred > 0 {
		return float64(preferred)
	}
	if len(entered) == 0 {
		return max(1, current)
	}
	s := int32(1)
	for _, o := range entered {
		s = max(s, o.scale)
	}
	return float64(s)
}

// refreshFor returns the refresh interval (FrameEvent.Refresh) of a window
// whose surface is on the outputs entered: that of the fastest, since the
// compositor does not say how much of the surface each one shows. 0
// (unknown) on no output, or when no output states a rate: the protocol
// has virtual outputs report 0.
func refreshFor(entered []*output) time.Duration {
	var mhz int32
	for _, o := range entered {
		mhz = max(mhz, o.refresh)
	}
	return platform.RefreshInterval(float64(mhz) / 1000)
}

// refresh is the loop's Refresh hook.
func (w *Window) refresh() time.Duration { return refreshFor(w.outputs) }
