package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

// SliderStyle is the recipe of [Slider] (shadcn/ui's Slider).
type SliderStyle struct {
	// Root is the whole slider, which a press anywhere on moves the nearest
	// thumb.
	Root string
	// Orientations are the root's classes per orientation: Horizontal
	// spans the width, Vertical the height (shadcn's
	// data-[orientation=vertical]:).
	Orientations map[Orientation]string
	// Disabled is added to the root while the slider is disabled.
	Disabled string
	// Track is the groove, which clips the range.
	Track string
	// TrackOrientations are the groove's dimensions per orientation.
	TrackOrientations map[Orientation]string
	// Range is the filled part of the groove.
	Range string
	// RangeOrientations are the range's dimensions per orientation.
	RangeOrientations map[Orientation]string
	// Thumb is the knob. shadcn's disabled: classes of the thumb are
	// left out: Radix's thumb is a span, which never matches them.
	Thumb string
}

func init() {
	defaultStyle.Slider = SliderStyle{
		Root: "relative flex w-full items-center select-none",
		Orientations: map[Orientation]string{
			Horizontal: "",
			Vertical:   "h-full min-h-44 w-auto flex-col",
		},
		Disabled: "opacity-50",
		Track:    "relative grow overflow-hidden rounded-full bg-muted",
		TrackOrientations: map[Orientation]string{
			Horizontal: "h-1.5 w-full",
			Vertical:   "h-full w-1.5",
		},
		Range: "absolute bg-primary",
		RangeOrientations: map[Orientation]string{
			Horizontal: "h-full",
			Vertical:   "w-full",
		},
		Thumb: "block size-4 shrink-0 rounded-full border border-primary bg-white shadow-sm ring-ring/50 transition-[color,box-shadow] " +
			"hover:ring-4 focus-visible:ring-4 focus-visible:outline-hidden",
	}
}

// Slider picks a number, or a range, by dragging a knob along a track,
// shadcn/ui's Slider, on aria.UseSlider: a press on the track moves the
// nearest knob there and goes on dragging it, the arrow keys move the focused
// knob by Step (PageUp and PageDown by a tenth of the range, Home and End to
// its ends), and each knob is a slider with aria-valuenow. Dragging only
// repaints: the knob and the range follow the pointer without the
// component being built on every move.
//
// With one knob, use Value, DefaultValue and OnValueChange, the kit's rule
// for a value:
//
//	vol, setVol := tela.UseState(30.0)
//	kit.Slider{Value: &vol, OnValueChange: setVol.Set, Max: 100, Step: 5, AriaLabel: "Volume"}
//
// With several (a price range has two), the values are a slice, as in Radix
// and shadcn/ui, and the knobs never pass each other:
//
//	kit.Slider{DefaultValues: []float64{20, 80}, ThumbLabels: []string{"Minimum price", "Maximum price"}}
//
// A Vertical slider has its Min at the bottom and takes the height of its
// container (at least 11rem).
type Slider struct {
	// Value points to the value of a controlled one-knob slider; nil leaves
	// it to the slider, which starts at DefaultValue (Min when it is 0).
	// OnValueChange is called with the new value on every change.
	Value         *float64
	DefaultValue  float64
	OnValueChange func(float64)
	// ValueSource is Value for a slider whose value is moved by something
	// other than the user, many times a second, such as a seek bar a clock
	// advances: it points to a tela.Value that holds the value, and the
	// knob and the range follow it by repainting, with no build of the
	// slider or its parent on each change. The slider is controlled by it:
	// OnValueChange must store the value the user asks for in it. It is read
	// when the slider builds (aria-valuenow), so a parent that builds from
	// time to time, say once a second, keeps the semantics current. It
	// takes the place of Value and DefaultValue; use it with one knob.
	ValueSource *tela.Value[float64]
	// Values, DefaultValues and OnValuesChange do the same for any number of
	// knobs (one per value, in ascending order): they take the place of
	// the three fields above when given.
	Values         *[]float64
	DefaultValues  []float64
	OnValuesChange func([]float64)
	// OnValueCommit is called with the values when the user stops changing
	// them: the pointer is released, or a key changed a value.
	OnValueCommit func([]float64)
	// Min and Max are the ends of the track: 0 and 100 by default.
	Min, Max float64
	// Step is the distance between the values the knobs can take (1 by
	// default).
	Step float64
	// MinStepsBetweenThumbs is the least number of Steps between two
	// knobs; 0 lets them touch.
	MinStepsBetweenThumbs int
	// Orientation is Horizontal (the default) or Vertical.
	Orientation Orientation
	// Disabled dims the slider and makes it ignore the user.
	Disabled bool
	// FormatValue returns a value as people read it (aria-valuetext).
	FormatValue func(float64) string
	// AriaLabel names the slider; ThumbLabels name each knob.
	AriaLabel   string
	ThumbLabels []string
	// Class is added after the recipe's classes. TrackClass, RangeClass and
	// ThumbClass do the same for their parts.
	Class      string
	TrackClass string
	RangeClass string
	ThumbClass string
	// Props are more props for the root element.
	Props tela.Child
}

// Build returns the slider element.
func (s Slider) Build() tela.Node {
	r := &UseStyle().Slider
	_, bump := tela.UseState(0)
	o := orientationOr(s.Orientation, Horizontal)
	vertical := o == Vertical
	// An Orientation the recipe does not define is reported here, before the
	// hook is told about it.
	rootClass, trackClass, rangeClass := pick("Slider", "Orientation", r.Orientations, o),
		pick("Slider", "Orientation", r.TrackOrientations, o), pick("Slider", "Orientation", r.RangeOrientations, o)
	so := aria.SliderOptions{
		DefaultValues: s.DefaultValues, OnValuesChange: s.OnValuesChange, OnChangeEnd: s.OnValueCommit,
		Min: s.Min, Max: s.Max, Step: s.Step, MinStepsBetweenThumbs: s.MinStepsBetweenThumbs,
		Orientation: string(o), Disabled: s.Disabled, FormatValue: s.FormatValue,
		AriaLabel: s.AriaLabel, ThumbLabels: s.ThumbLabels,
	}
	src := s.ValueSource
	switch {
	case src != nil:
		so.Values = &[]float64{src.Latest()}
		so.LiveValues = func() []float64 { return []float64{src.Latest()} }
	case s.Values != nil:
		so.Values = s.Values
	case s.Value != nil:
		so.Values = &[]float64{*s.Value}
	case len(s.DefaultValues) == 0 && s.DefaultValue != 0:
		so.DefaultValues = []float64{s.DefaultValue}
	}
	if on := s.OnValueChange; on != nil && s.OnValuesChange == nil {
		so.OnValuesChange = func(v []float64) { on(v[0]) }
	}
	var dragging *bool
	if src != nil {
		// The slider is not built when the Value changes, so a change the
		// user makes with a key (or a press) builds it once, for
		// aria-valuenow; the moves of a drag do not (the drag's own builds
		// at its start and end bring the semantics up to date).
		ask := so.OnValuesChange
		so.OnValuesChange = func(v []float64) {
			if ask != nil {
				ask(v)
			}
			if !*dragging {
				bump.Update(func(n int) int { return n + 1 })
			}
		}
	}
	sl := aria.UseSlider(so)
	dragging = &sl.Dragging
	track := tela.UseSize(sl.TrackRef)

	n := len(sl.Values)
	// pct is where a knob is, read in the paint-phase closures.
	pct := sl.ThumbPercent
	if src != nil {
		lo, hi := so.Min, so.Max
		if lo == 0 && hi == 0 {
			hi = 100
		}
		pct = func(int) float64 { return (min(max(src.Get(), lo), hi) - lo) / (hi - lo) * 100 }
	}
	kids := []tela.Child{cls(r.Root, rootClass, when(s.Disabled, r.Disabled), s.Class),
		sl.Props, sl.TrackProps, s.Props}
	kids = append(kids, h.Div(cls(r.Track, trackClass, s.TrackClass),
		sliderRange{pct: pct, n: n, track: track, vertical: vertical,
			class: joinClass(r.Range, rangeClass, s.RangeClass)}))
	for i := range n {
		kids = append(kids, sliderThumb{res: sl, pct: pct, i: i, track: track, vertical: vertical, class: joinClass(r.Thumb, s.ThumbClass)})
	}
	return h.Div(kids...)
}

// sliderRange is the filled part of the track. With one knob it is as large
// as the track and moved by the knob; with several it spans the first and
// the last, which takes a layout per move, and a single knob only a paint.
type sliderRange struct {
	pct      func(int) float64
	n        int
	track    *tela.Value[tela.Size]
	vertical bool
	class    string
}

func (r sliderRange) Build() tela.Node {
	pct, track := r.pct, r.track
	if r.n == 1 {
		// The track clips what sticks out: the bar is slid back by the part
		// that is not filled.
		return h.Div(cls(r.class, "size-full", "top-0 left-0"),
			h.Style(tela.TranslateFn(func() (x, y float32) {
				rest := float32(1 - pct(0)/100)
				if r.vertical {
					return 0, rest * track.Get().Height
				}
				return -rest * track.Get().Width, 0
			})))
	}
	extent := func() (lo, hi float64) {
		lo, hi = pct(0), pct(0)
		for i := 1; i < r.n; i++ {
			p := pct(i)
			lo, hi = min(lo, p), max(hi, p)
		}
		return lo / 100, hi / 100
	}
	if r.vertical {
		return h.Div(cls(r.class, "bottom-0 left-0"),
			h.Style(tela.HeightFn(func() float32 {
				lo, hi := extent()
				return float32(hi-lo) * track.Get().Height
			}), tela.TranslateFn(func() (x, y float32) {
				lo, _ := extent()
				return 0, -float32(lo) * track.Get().Height
			})))
	}
	return h.Div(cls(r.class, "top-0 left-0"),
		h.Style(tela.WidthFn(func() float32 {
			lo, hi := extent()
			return float32(hi-lo) * track.Get().Width
		}), tela.TranslateFn(func() (x, y float32) {
			lo, _ := extent()
			return float32(lo) * track.Get().Width, 0
		})))
}

// sliderThumb is one knob. It is a component of its own to measure itself
// (the knob is kept inside the track, as Radix does: its center runs from
// half its size to the track's length minus half).
type sliderThumb struct {
	res      aria.SliderResult
	pct      func(int) float64
	i        int
	track    *tela.Value[tela.Size]
	vertical bool
	class    string
}

func (t sliderThumb) Build() tela.Node {
	res, i, track := t.res, t.i, t.track
	self := tela.UseSize(res.ThumbRef(i))
	pos := "left-0"
	if t.vertical {
		pos = "bottom-0"
	}
	return h.Div(cls("absolute", pos, t.class), res.ThumbProps(i),
		h.Style(tela.TranslateFn(func() (x, y float32) {
			p := float32(t.pct(i)) / 100
			if t.vertical {
				return 0, -p * (track.Get().Height - self.Get().Height)
			}
			return p * (track.Get().Width - self.Get().Width), 0
		})))
}
