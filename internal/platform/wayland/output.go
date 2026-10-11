//go:build linux

package wayland

import (
	"slices"

	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// output is a bound wl_output. Only its integer scale matters in phase 1.
type output struct {
	b       *Backend
	name    uint32 // registry name
	wl      *protocols.WlOutput
	scale   int32 // applied scale
	pending int32 // scale received, applied on done (version 2+)
}

func (b *Backend) addOutput(name, version uint32) {
	o := &output{b: b, name: name, scale: 1, pending: 1}
	o.wl = protocols.WlRegistryBind[protocols.WlOutput](b.registry, name, min(version, outputVersion))
	o.wl.SetScaleHandler(func(e protocols.WlOutputScaleEvent) {
		o.pending = max(1, e.Factor)
		if o.wl.Version() < 2 { // no done event before version 2
			o.apply()
		}
	})
	o.wl.SetDoneHandler(func(protocols.WlOutputDoneEvent) { o.apply() })
	b.outputs = append(b.outputs, o)
}

func (o *output) apply() {
	if o.pending == o.scale {
		return
	}
	o.scale = o.pending
	for _, w := range o.b.windows {
		if slices.Contains(w.outputs, o) {
			w.updateScale()
		}
	}
}

// removeOutput handles the output's global going away.
func (b *Backend) removeOutput(o *output) {
	for _, w := range b.windows {
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

// canScale reports whether buffers can have a scale other than 1:
// wl_surface.set_buffer_scale exists since wl_compositor version 3.
func (b *Backend) canScale() bool { return b.compositor.Version() >= 3 }

// maxOutputScale is the largest scale of all outputs (1 without outputs).
// New windows start with it: guessing the HiDPI scale avoids a second
// configure on single-monitor HiDPI setups, which are the common case.
func (b *Backend) maxOutputScale() int32 {
	s := int32(1)
	for _, o := range b.outputs {
		s = max(s, o.scale)
	}
	return s
}

// scaleFor decides a window's integer buffer scale:
//
//  1. wl_surface.preferred_buffer_scale, once the compositor sent it
//     (wl_compositor version 6+). This is what the compositor wants;
//     Hyprland and others send ceil(fractional scale) here.
//  2. Otherwise the largest scale of the outputs the surface entered, like
//     Gio and pre-v6 toolkits (render at the highest density the window is
//     shown at; the compositor downscales on the other outputs).
//  3. Before the surface entered any output: the initial guess.
//
// Phase 6 adds wp_fractional_scale_v1 in front of (1).
func scaleFor(preferred int32, entered []*output, initial int32) int32 {
	if preferred > 0 {
		return preferred
	}
	if len(entered) == 0 {
		return max(1, initial)
	}
	s := int32(1)
	for _, o := range entered {
		s = max(s, o.scale)
	}
	return s
}
