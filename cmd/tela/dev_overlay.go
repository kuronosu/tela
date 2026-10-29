package main

import (
	"image"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/remote"
)

// multiView shows a problem in several views.
type multiView []problemView

func (m multiView) show(p problem) {
	for _, v := range m {
		v.show(p)
	}
}

func (m multiView) clear() {
	for _, v := range m {
		v.clear()
	}
}

// overlayView shows the dev loop's problem over the window (ADR-117): a
// panel drawn by Tela's own renderer (dev_overlay_render.go) composited
// over the frame the remote host presents, through Host.SetOverlay. It is
// one of the problemViews the supervisor drives (show and clear run on the
// supervisor's goroutine). Everything else runs on the host's UI thread,
// where the state lives and nothing needs a lock: show and clear only
// post to it, and the host calls draw and handleInput there.
//
// While no problem is shown, or after the user dismissed it, no overlay is
// set on the host, so the dev loop pays nothing for it.
type overlayView struct {
	host *remote.Host

	// UI thread state.
	model     *overlayModel // nil: no problem
	dismissed bool
	pic       overlayPicture // the rendered model, for one window size
	press     pressState
	onError   func(error)
}

// pressState tracks a pointer sequence that started on the overlay, so its
// release is the overlay's too even if the pointer left the panel.
type pressState struct {
	captured bool // a press started inside the panel
	onClose  bool // ...and on the close button
}

// overlayPicture is a render of the model.
type overlayPicture struct {
	model *overlayModel
	size  platform.Size
	scale float64
	img   *image.RGBA
	geom  ovGeom
}

func newOverlayView(onError func(error)) *overlayView {
	return &overlayView{onError: onError}
}

// attach gives the view its host.
func (v *overlayView) attach(h *remote.Host) { v.host = h }

// show replaces the problem on view (supervisor goroutine).
func (v *overlayView) show(p problem) {
	m := newOverlayModel(p)
	v.host.Post(func() {
		v.model, v.dismissed = m, false
		v.press = pressState{}
		v.host.SetOverlay(v.draw)
	})
}

// clear removes the problem (supervisor goroutine).
func (v *overlayView) clear() {
	v.host.Post(func() {
		v.model, v.dismissed = nil, false
		v.pic = overlayPicture{}
		v.press = pressState{}
		v.host.SetOverlay(nil)
	})
}

// dismiss hides the panel until the next problem (UI thread).
func (v *overlayView) dismiss() {
	v.dismissed = true
	v.pic = overlayPicture{}
	v.press = pressState{}
	v.host.SetOverlay(nil)
}

// active reports whether the panel is on screen.
func (v *overlayView) active() bool { return v.model != nil && !v.dismissed }

// draw is the host's overlay function: it composites the panel over fb,
// rendering it first when the window's size or scale changed.
func (v *overlayView) draw(w platform.Window, fb *image.RGBA) {
	if !v.active() {
		return
	}
	m := w.Metrics()
	if m.Size.W <= 0 || m.Size.H <= 0 {
		return
	}
	if v.pic.model != v.model || v.pic.size != m.Size || v.pic.scale != m.Scale || v.pic.img == nil {
		img, g, err := v.model.render(m.Size, m.Scale)
		if err != nil {
			if v.onError != nil {
				v.onError(err)
			}
			v.pic = overlayPicture{}
			return
		}
		v.pic = overlayPicture{model: v.model, size: m.Size, scale: m.Scale, img: img, geom: g}
	}
	g := v.pic.geom
	r := image.Rect(
		int(g.paint.X0*float32(m.Scale)), int(g.paint.Y0*float32(m.Scale)),
		int(g.paint.X1*float32(m.Scale)+0.999), int(g.paint.Y1*float32(m.Scale)+0.999),
	)
	compositeOver(fb, v.pic.img, r)
}

// handleInput is the host's Input option: the overlay takes the pointer
// events that start inside the panel and Escape (which dismisses it); the
// app gets everything else. UI thread.
func (v *overlayView) handleInput(w platform.Window, e platform.Event) bool {
	if !v.active() {
		return false
	}
	switch e := e.(type) {
	case platform.KeyEvent:
		if e.Key != platform.KeyEscape {
			return false
		}
		if e.Action != platform.KeyPress {
			return false
		}
		// The app never sees this press, so the host's view of its
		// keyboard drops the repeats and the release that follow.
		v.dismiss()
		return true
	case platform.PointerEvent:
		return v.pointer(w, e)
	case platform.ScrollEvent:
		return v.geometry(w).panel.Contains(gfx.Point(e.Pos))
	}
	return false
}

// geometry returns the panel's geometry for w's current size.
func (v *overlayView) geometry(w platform.Window) ovGeom {
	size := w.Metrics().Size
	if v.pic.model == v.model && v.pic.size == size && v.pic.img != nil {
		return v.pic.geom
	}
	return v.model.geometry(size)
}

func (v *overlayView) pointer(w platform.Window, e platform.PointerEvent) bool {
	g := v.geometry(w)
	in := g.panel.Contains(gfx.Point(e.Pos))
	switch e.Action {
	case platform.PointerDown:
		if !in {
			return false
		}
		v.press = pressState{captured: true, onClose: g.close.Contains(gfx.Point(e.Pos))}
		return true
	case platform.PointerUp:
		if !v.press.captured {
			return false
		}
		closing := v.press.onClose && g.close.Contains(gfx.Point(e.Pos))
		if e.Buttons == 0 {
			v.press = pressState{}
		}
		if closing {
			v.dismiss()
		}
		return true
	case platform.PointerMove:
		// A drag that started in the app goes on in the app, and one
		// that started on the panel stays on the panel.
		if v.press.captured {
			return true
		}
		return in && e.Buttons == 0
	case platform.PointerCancel:
		if v.press.captured {
			v.press = pressState{}
			return true
		}
	}
	return false
}

// compositeOver draws src, premultiplied and of dst's size, over the
// opaque dst inside r.
func compositeOver(dst, src *image.RGBA, r image.Rectangle) {
	r = r.Intersect(dst.Rect).Intersect(src.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		d := dst.Pix[dst.PixOffset(r.Min.X, y):dst.PixOffset(r.Max.X, y)]
		s := src.Pix[src.PixOffset(r.Min.X, y):src.PixOffset(r.Max.X, y)]
		for i := 0; i+3 < len(s); i += 4 {
			a := uint32(s[i+3])
			switch a {
			case 0:
				continue
			case 255:
				d[i], d[i+1], d[i+2], d[i+3] = s[i], s[i+1], s[i+2], 255
				continue
			}
			k := 255 - a
			d[i] = uint8(uint32(s[i]) + (uint32(d[i])*k+127)/255)
			d[i+1] = uint8(uint32(s[i+1]) + (uint32(d[i+1])*k+127)/255)
			d[i+2] = uint8(uint32(s[i+2]) + (uint32(d[i+2])*k+127)/255)
			d[i+3] = 255
		}
	}
}
