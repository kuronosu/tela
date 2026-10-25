package kit

import (
	"cmp"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

// ColorPickerStyle is the recipe of [ColorPicker] and of the panel
// [ColorInput] opens. shadcn/ui has no color picker: the look is the kit's
// own, written in shadcn's language (ADR-143).
type ColorPickerStyle struct {
	// Root is the panel: a column that is 16rem wide, which is the room
	// inside a [Popover]'s p-4 and w-72.
	Root string
	// Disabled is added to the root while the picker is disabled.
	Disabled string
	// Area is the saturation and brightness area. Its background is the
	// hue at full saturation and brightness, which the component paints
	// live; AreaSaturation and AreaBrightness are the white and black
	// gradients over it.
	Area           string
	AreaSaturation string
	AreaBrightness string
	// AreaThumb is the round handle on the area.
	AreaThumb string
	// Slider is the row of a hue or alpha slider: the track, and the thumb
	// on top of it.
	Slider string
	// Track is the groove of a slider; HueTrack adds the rainbow to it and
	// AlphaTrack adds the checkerboard (the color's gradient is drawn over
	// it by AlphaFill).
	Track      string
	HueTrack   string
	AlphaTrack string
	AlphaFill  string
	// Thumb is the round handle of a slider. It is centered on the point
	// the value stands for, so it hangs over the ends of the track.
	Thumb string
	// Checkerboard is the pattern under everything that can be
	// transparent: the alpha track, the preview and the swatches. It is a
	// conic gradient that repeats every 12px (two squares of 6px).
	Checkerboard string
	// Row is the line of the preview and the hex field.
	Row string
	// Preview is the swatch next to the hex field; it holds the color.
	Preview string
	// Input is added to the classes of the kit's [Input] on the hex field.
	Input string
	// Swatches is the wrapping row of preset colors; Swatch is one of
	// them and SwatchSelected is added to the one that is the color now.
	Swatches       string
	Swatch         string
	SwatchSelected string
}

// ColorInputStyle is the recipe of [ColorInput].
type ColorInputStyle struct {
	// Trigger is added to the classes of the outline [Button] that is the
	// default trigger.
	Trigger string
	// Swatch is the small square of the color inside the trigger.
	Swatch string
	// Code is the hex code inside the trigger. Inter is proportional, so
	// its digits are tabular (tabular-nums) and its minimum width is that
	// of the widest code, "#" and six digits (the digits are wider than the
	// letters a to f, which the code is written with), so the button does
	// not change size while the color does. CodeAlpha is added to Code
	// while the input has Alpha, with the width of "#" and eight digits.
	// The widths are in em, for Inter at any size; another font needs its
	// own.
	Code      string
	CodeAlpha string
}

func init() {
	const (
		light = "#d4d4d4" // the darker squares of the checkerboard
		dark  = "#404040"
	)
	defaultStyle.ColorPicker = ColorPickerStyle{
		Root:           "flex w-64 flex-col gap-4",
		Disabled:       "opacity-50",
		Area:           "relative h-40 w-full shrink-0 cursor-crosshair rounded-md shadow-xs select-none",
		AreaSaturation: "pointer-events-none absolute inset-0 rounded-md bg-linear-to-r from-white to-transparent",
		AreaBrightness: "pointer-events-none absolute inset-0 rounded-md bg-linear-to-t from-black to-transparent ring-1 ring-black/10 ring-inset dark:ring-white/10",
		AreaThumb: "absolute top-0 left-0 size-5 rounded-full border-2 border-white shadow-md ring-ring/50 transition-[color,box-shadow] outline-hidden " +
			"hover:ring-4 focus-visible:ring-4",
		Slider:       "relative flex h-4 w-full items-center select-none",
		Track:        "relative h-3 grow cursor-pointer rounded-full shadow-xs",
		HueTrack:     "bg-linear-[90deg,#f00,#ff0,#0f0,#0ff,#00f,#f0f,#f00]/srgb",
		AlphaTrack:   "overflow-hidden",
		AlphaFill:    "pointer-events-none absolute inset-0 ring-1 ring-black/10 ring-inset dark:ring-white/10",
		Thumb:        "absolute top-auto left-0 size-4 rounded-full border-2 border-white shadow-md ring-ring/50 transition-[color,box-shadow] outline-hidden hover:ring-4 focus-visible:ring-4",
		Checkerboard: "bg-white bg-[conic-gradient(" + light + "_25%,#fff_0_50%," + light + "_0_75%,#fff_0)] bg-[length:12px_12px] dark:bg-neutral-900 dark:bg-[conic-gradient(" + dark + "_25%,#262626_0_50%," + dark + "_0_75%,#262626_0)]",
		Row:          "flex items-center gap-2",
		Preview:      "relative size-9 shrink-0 overflow-hidden rounded-md border border-border shadow-xs",
		Input:        "tabular-nums",
		Swatches:     "flex flex-wrap gap-2",
		Swatch: "relative size-6 shrink-0 overflow-hidden rounded-md border border-border shadow-xs ring-ring/50 transition-[color,box-shadow] outline-none " +
			"hover:ring-4 focus-visible:ring-4 disabled:pointer-events-none",
		SwatchSelected: "ring-2 ring-ring ring-offset-2 ring-offset-popover",
	}
	defaultStyle.ColorInput = ColorInputStyle{
		Trigger:   "justify-start font-normal",
		Swatch:    "relative size-4 shrink-0 overflow-hidden rounded-sm border border-border",
		Code:      "tabular-nums min-w-[4.53em]",
		CodeAlpha: "min-w-[5.83em]",
	}
}

// ColorPicker lets people choose a color: an area for saturation and
// brightness, a slider for the hue, optionally one for the opacity, a field
// for the hex code, and optionally a row of preset colors. The value is a
// [tela.Color], and the picker follows the kit's rule for values.
//
//	c, setC := tela.UseState(tela.Hex("#6366f1"))
//	kit.ColorPicker{Value: &c, OnValueChange: setC.Set, Alpha: true,
//	    Swatches: []tela.Color{tela.Hex("#ef4444"), tela.Hex("#22c55e"), tela.Hex("#3b82f6")}}
//
// It is built on the color hooks of package aria (aria.UseColorState, the
// area, the sliders and the field), so the area and the sliders are
// dragged with a mouse, a finger or a pen, the arrow keys move a thumb by
// one step (Shift or Page Up and Page Down by more) and each control is
// announced with its value. The hue is kept when the color goes through
// a gray or black, so dragging to the left edge of the area does not lose it.
//
// Dragging only repaints: the thumbs, the area, the preview, the hex field
// (unless it is being typed in) and the alpha track follow the pointer
// without the picker being built on every move.
// OnValueChange is called on every change, so a controlled picker builds as
// often as the state it is given does; OnValueCommit is called once when
// the user stops (a drag ends, a key, a swatch or a hex code changed the
// color), which is the moment to save it.
//
// The hex field accepts "#rgb", "#rrggbb" (and with Alpha the forms with
// four or eight digits) with or without the "#", commits on Enter and when
// it loses the focus, and goes back to the color when the text is not a
// code. Use [ColorInput] for a picker that opens from a button.
type ColorPicker struct {
	// Value points to the color of a controlled picker; nil leaves it to
	// the picker, which starts at DefaultValue. OnValueChange is called with
	// the next color on every change.
	Value *tela.Color
	// DefaultValue is the color of an uncontrolled picker at the start; nil
	// starts at opaque black (tela.Black). It is a pointer because the zero
	// Color is a color too, transparent black.
	DefaultValue  *tela.Color
	OnValueChange func(tela.Color)
	// OnValueCommit is called with the color when the user stops changing
	// it, and not for a change that left it as it was.
	OnValueCommit func(tela.Color)
	// Alpha adds the opacity slider and lets the hex field take and show
	// the opacity (#rrggbbaa). Without it the field shows #rrggbb and the
	// color keeps whatever opacity it had.
	Alpha bool
	// Swatches are preset colors: buttons that set the color.
	Swatches []tela.Color
	// Disabled dims the picker and makes it ignore the user.
	Disabled bool
	// AreaLabel, HueLabel, AlphaLabel and HexLabel name the controls for
	// assistive technologies ("Color area", "Hue", "Alpha" and "Hex" by
	// default); set them for another language. The value texts ("Hue
	// 180°") are aria's English ones.
	AreaLabel, HueLabel, AlphaLabel, HexLabel string
	// Direction is "ltr" or "rtl"; empty is the direction at the component's
	// position (tela.UseDirection: the nearest h.Dir above it, left to right
	// without one). Under "rtl" the saturation grows toward the left
	// of the area, the hue and the opacity toward the left of their sliders,
	// and ArrowLeft increases them; a Direction given here also sets the
	// direction of the panel's layout.
	Direction string
	// ID is the id of the picker's hex field, for a [Label]'s For. Inside
	// a [Field] it defaults to the field's, so the field's label points to
	// it and a press on the label focuses it.
	ID string
	// AriaLabel names the picker as a group for assistive technologies;
	// inside a Field with a Label that label is used. A picker with neither
	// is not a group.
	AriaLabel string
	// Class is added after the recipe's Root classes. Props are more props
	// for the root element.
	Class string
	Props tela.Child
}

// Build returns the picker. Inside a [Field] the hex field takes the
// field's id, description and invalid state, and the picker is named by
// the field's label.
func (p ColorPicker) Build() tela.Node {
	fc := tela.UseContext(fieldCtx)
	field := colorField{id: cmp.Or(p.ID, fc.id), label: cmp.Or(p.AriaLabel, fc.label), describedBy: fc.describedBy, invalid: fc.invalid}
	return colorPanel{p: p, parts: useColorParts(p), field: field}.node(UseStyle())
}

// colorField is what a picker takes from its [Field], or from its own ID
// and AriaLabel: the id of its hex field, its name as a group, and the
// description and invalid state of the field. The panel of a ColorInput
// has none (the button does).
type colorField struct {
	id, label   string
	describedBy tela.ElementRef
	invalid     bool
}

// colorShared is what the controls need of the color state's owner: the
// callbacks, and the last color asked for, with the last one reported as
// committed. Everything runs on the UI thread.
type colorShared struct {
	onChange, onCommit func(tela.Color)
	cur, committed     tela.Color
}

// changed is the OnChange of the color state.
func (sh *colorShared) changed(c tela.Color) {
	sh.cur = c
	if sh.onChange != nil {
		sh.onChange(c)
	}
}

// commit reports the color the user stopped on, unless it is the one that
// was committed before.
func (sh *colorShared) commit() {
	if sh.cur == sh.committed {
		return
	}
	sh.committed = sh.cur
	if sh.onCommit != nil {
		sh.onCommit(sh.cur)
	}
}

// colorParts is everything the picker's hooks give: the color state, the
// area, the two sliders and the hex field, with the sizes the thumbs are
// placed with. The component that owns the color calls the hooks (the
// state asks that component to build when a drag starts and ends, so the
// hooks must be its own) and gives the parts to colorPanel to draw; a
// ColorInput owns them while its panel is closed and gone.
type colorParts struct {
	st    *aria.ColorState
	sh    *colorShared
	color tela.Color // the color as of this build
	area  aria.ColorAreaResult
	hue   aria.ColorSliderResult
	alpha aria.ColorSliderResult
	hex   aria.ColorFieldResult

	areaSize, areaThumb, hueTrack, hueThumb, alphaTrack, alphaThumb *tela.Value[tela.Size]

	// rtl is a right-to-left picker; rootDir is the h.Dir its own Direction
	// asks for.
	rtl     bool
	rootDir tela.Child
}

// useColorParts is the hook of a picker, called by the component that
// owns its color: ColorPicker, or ColorInput, whose panel is in a Popover
// that exists only while open. All the hooks are called whatever the
// props are (the alpha slider's too, drawn or not), so that a picker can
// change its Alpha or its Disabled.
//
// A nil DefaultValue starts at opaque black.
func useColorParts(p ColorPicker) colorParts {
	def := tela.Black
	if p.DefaultValue != nil {
		def = *p.DefaultValue
	}
	sh := tela.UseOnce(func() *colorShared {
		start := def
		if p.Value != nil {
			start = *p.Value
		}
		return &colorShared{cur: start, committed: start}
	})
	sh.onChange, sh.onCommit = p.OnValueChange, p.OnValueCommit
	st := aria.UseColorState(aria.ColorStateOptions{Value: p.Value, DefaultValue: &def, OnChange: sh.changed})
	end := func(tela.Color) { sh.commit() }
	dir, rootDir := useDirection("ColorPicker", p.Direction)
	c := colorParts{st: st, sh: sh, color: st.Color(), rtl: dir == "rtl", rootDir: rootDir}
	c.area = aria.UseColorArea(aria.ColorAreaOptions{State: st, Disabled: p.Disabled, AriaLabel: p.AreaLabel, OnChangeEnd: end, Direction: dir})
	c.hue = aria.UseColorSlider(aria.ColorSliderOptions{State: st, Channel: aria.ColorHue, Disabled: p.Disabled, AriaLabel: p.HueLabel, OnChangeEnd: end, Direction: dir})
	c.alpha = aria.UseColorSlider(aria.ColorSliderOptions{State: st, Channel: aria.ColorAlpha, Disabled: p.Disabled, AriaLabel: p.AlphaLabel, OnChangeEnd: end, Direction: dir})
	c.hex = aria.UseColorField(aria.ColorFieldOptions{State: st, Alpha: p.Alpha, Disabled: p.Disabled, AriaLabel: orStr(p.HexLabel, "Hex")})
	c.areaSize, c.areaThumb = tela.UseSize(c.area.AreaRef), tela.UseSize(c.area.ThumbRef)
	c.hueTrack, c.hueThumb = tela.UseSize(c.hue.TrackRef), tela.UseSize(c.hue.ThumbRef)
	c.alphaTrack, c.alphaThumb = tela.UseSize(c.alpha.TrackRef), tela.UseSize(c.alpha.ThumbRef)
	return c
}

// colorPanel draws the picker from its parts.
type colorPanel struct {
	p     ColorPicker
	parts colorParts
	field colorField
}

// Build is the component of a ColorInput's popover, which draws the parts its
// owner made.
func (c colorPanel) Build() tela.Node { return c.node(UseStyle()) }

// node draws the panel with the active style.
func (c colorPanel) node(style *Style) tela.Node {
	s := &style.ColorPicker
	p, k := c.p, c.parts
	kids := []tela.Child{cls(s.Root, when(p.Disabled, s.Disabled), p.Class), k.rootDir, p.Props, c.areaNode(s)}
	if c.field.label != "" {
		kids = append(kids, h.Role("group"), h.AriaLabel(c.field.label))
	}
	kids = append(kids, c.hueNode(s))
	if p.Alpha {
		kids = append(kids, c.alphaNode(s))
	}
	kids = append(kids, h.Div(cls(s.Row), colorSwatch(k.st, style, s.Preview), c.hexNode(style)))
	if len(p.Swatches) > 0 {
		now := aria.FormatHexColor(k.color, true)
		sw := make([]tela.Child, 0, len(p.Swatches)+1)
		sw = append(sw, cls(s.Swatches))
		for _, col := range p.Swatches {
			hex := aria.FormatHexColor(col, col.A < 1)
			selected := aria.FormatHexColor(col, true) == now
			sw = append(sw, h.Button(h.Key(hex), cls(s.Checkerboard, s.Swatch, when(selected, s.SwatchSelected)),
				h.AriaLabel(hex), h.AriaPressed(selected), h.Disabled(p.Disabled),
				h.OnClick(func() { k.st.Set(col); k.sh.commit() }),
				h.Div(h.Class("absolute inset-0"), h.Style(tela.Bg(col)))))
		}
		kids = append(kids, h.Div(sw...))
	}
	return h.Div(kids...)
}

// colorSwatch shows the color live over the checkerboard: the preview of
// the picker and the swatch of a [ColorInput]. Its fill is painted by a
// function, so it follows a drag without a build.
func colorSwatch(st *aria.ColorState, style *Style, class string) tela.Node {
	return h.Div(cls(style.ColorPicker.Checkerboard, class),
		h.Div(h.Class("absolute inset-0"), h.Style(tela.BgFn(st.LiveColor))))
}

// opaque returns c with its opacity at 1.
func opaque(c tela.Color) tela.Color { c.A = 1; return c }

// hueColor is the color of the hue at full saturation and brightness.
func hueColor(c aria.HSB) tela.Color { return aria.HSB{H: c.H, S: 100, B: 100, A: 1}.Color() }

// areaNode is the saturation and brightness area and its thumb.
func (c colorPanel) areaNode(s *ColorPickerStyle) tela.Node {
	k := c.parts
	st, area, size, self := k.st, k.area, k.areaSize, k.areaThumb
	return h.Div(cls(s.Area), area.AreaProps,
		h.Style(tela.BgFn(func() tela.Color { return hueColor(st.LiveHSB()) })),
		// The saturation is at its least at the start of the area: the
		// right, in a right-to-left picker.
		h.Div(cls(s.AreaSaturation, when(k.rtl, "bg-linear-to-l"))), h.Div(cls(s.AreaBrightness)),
		h.Div(cls(s.AreaThumb), area.ThumbProps,
			h.Style(tela.BgFn(func() tela.Color { return opaque(st.LiveColor()) }),
				tela.TranslateFn(func() (x, y float32) {
					px, py := area.ThumbPercent()
					sz, me := size.Get(), self.Get()
					return float32(px)/100*sz.Width - me.Width/2, float32(py)/100*sz.Height - me.Height/2
				}))))
}

// hueNode is the hue slider: the rainbow, which does not depend on the
// color (a track that followed the saturation and brightness would turn
// gray with a gray color), and a thumb filled with the hue.
func (c colorPanel) hueNode(s *ColorPickerStyle) tela.Node {
	k := c.parts
	st := k.st
	hue := s.HueTrack
	if k.rtl {
		hue = strings.Replace(hue, "linear-[90deg", "linear-[270deg", 1)
	}
	return sliderNode(s, k.rtl, k.hue, k.hueTrack, k.hueThumb,
		[]tela.Child{cls(s.Track, hue)},
		func() tela.Color { return hueColor(st.LiveHSB()) })
}

// alphaNode is the opacity slider: a checkerboard with the gradient from
// clear to the color over it. The gradient has no paint-phase form, so it
// is drawn from the color of the last build and catches up when a drag of
// the area or of the hue rests or ends; the thumb is filled live.
func (c colorPanel) alphaNode(s *ColorPickerStyle) tela.Node {
	k := c.parts
	st := k.st
	clear, solid := k.color, opaque(k.color)
	clear.A = 0
	angle := float32(90)
	if k.rtl {
		angle = 270
	}
	return sliderNode(s, k.rtl, k.alpha, k.alphaTrack, k.alphaThumb,
		[]tela.Child{cls(s.Checkerboard, s.Track, s.AlphaTrack),
			h.Div(cls(s.AlphaFill), h.Style(tela.BackgroundGradient(tela.LinearGradient(angle, tela.Stop(clear), tela.Stop(solid)))))},
		func() tela.Color { return opaque(st.LiveColor()) })
}

// sliderNode is a slider row: its track, with the given classes and
// children, and its thumb, centered on the value and filled by fill. A
// right-to-left row has its minimum at the right, where its thumb is
// anchored (right-0 and not end-0: the keys and the drag follow the
// picker's Direction, which the thumb must agree with).
func sliderNode(s *ColorPickerStyle, rtl bool, sl aria.ColorSliderResult, track, self *tela.Value[tela.Size], trackKids []tela.Child, fill func() tela.Color) tela.Node {
	return h.Div(cls(s.Slider), sl.Props,
		h.Div(append(trackKids, sl.TrackProps)...),
		h.Div(cls(s.Thumb, when(rtl, "right-0 left-auto")), sl.ThumbProps,
			h.Style(tela.BgFn(fill), tela.TranslateFn(func() (x, y float32) {
				x = float32(sl.ThumbPercent())/100*track.Get().Width - self.Get().Width/2
				if rtl {
					x = -x
				}
				return x, 0
			}))))
}

// hexNode is the hex field: the kit's Input on aria.UseColorField.
func (c colorPanel) hexNode(st *Style) tela.Node {
	k := c.parts
	sh := k.sh
	f := c.field
	kids := []tela.Child{cls(st.Input.Root, st.ColorPicker.Input), h.Dir("ltr"), k.hex.InputProps}
	if f.id != "" {
		kids = append(kids, h.ID(f.id))
	}
	if f.describedBy != (tela.ElementRef{}) {
		kids = append(kids, h.AriaDescribedBy(f.describedBy))
	}
	if f.invalid {
		kids = append(kids, h.AriaInvalid(true))
	}
	return h.Input(append(kids,
		// Enter and a blur commit the text (the hook's handlers ran first).
		h.OnBlur(sh.commit),
		h.OnKeyDown(func(e tela.KeyboardEvent) {
			if e.Key == "Enter" {
				sh.commit()
			}
		}))...)
}

// ColorInput is a button that shows a color and opens a [ColorPicker] in a
// [Popover] to change it: the swatch and the hex code on a button, the
// picker in a panel under it. It takes the values of a ColorPicker and the
// open state of a Popover.
//
//	kit.ColorInput{DefaultValue: new(tela.Hex("#6366f1")), Alpha: true, OnValueCommit: save}
//
// The color lives in the ColorInput, not in the panel, so the swatch and the
// code on the button follow a drag in the panel without building the
// component on every move, and the color survives the panel being closed.
// The panel is a dialog; the focus goes into it when it opens and returns
// to the button when it closes (Escape, or a press outside it).
//
// Trigger replaces the button, with the rule of every trigger slot (see
// [Trigger]): the button is the one you write, with the popover's props.
// Then nothing on it shows the color unless you draw it from OnValueChange.
type ColorInput struct {
	// The color, as in [ColorPicker].
	Value         *tela.Color
	DefaultValue  *tela.Color
	OnValueChange func(tela.Color)
	OnValueCommit func(tela.Color)
	Alpha         bool
	Swatches      []tela.Color
	// Disabled makes the button inert and dims it, and the panel never opens.
	Disabled bool
	// Trigger is the element that opens the panel; nil is an outline
	// [Button] with the swatch and the code.
	Trigger Trigger
	// Placement, Align and Offset place the panel as a [Popover]'s do: below
	// the button, at its start, by default.
	Placement, Align string
	Offset           float32

	// Open points to the open state of a controlled panel, as a
	// [Popover]'s does.
	Open         *bool
	DefaultOpen  bool
	OnOpenChange func(open bool)

	// AreaLabel, HueLabel, AlphaLabel and HexLabel name the picker's
	// controls, as in [ColorPicker].
	AreaLabel, HueLabel, AlphaLabel, HexLabel string
	// AriaLabel names the button, followed by its color ("Brand color,
	// #6366f1"), and the panel ("Color picker" when it is empty); inside a
	// [Field] with a Label that label is used. ID is the button's id, for a
	// [Label]'s For, and inside a Field defaults to the field's. With a
	// Trigger of your own, these are for the panel only: name the button and
	// give it its id yourself.
	AriaLabel string
	ID        string
	// Direction is "ltr" or "rtl"; empty is the direction at the component's
	// position (tela.UseDirection: the nearest h.Dir above it, left to right
	// without one). Under "rtl" the saturation grows toward the left
	// of the area, the hue and the opacity toward the left of their sliders,
	// and ArrowLeft increases them; a Direction given here also sets the
	// direction of the panel's layout, not the button's.
	Direction string

	// Class is added to the default button's classes; PanelClass to the
	// picker's root; PopoverClass to the popover's panel.
	Class        string
	PanelClass   string
	PopoverClass string
	// Props are more props for the default button.
	Props tela.Child
}

// Build returns the button and its popover.
func (c ColorInput) Build() tela.Node {
	r := &UseStyle().ColorInput
	p := ColorPicker{
		Value: c.Value, DefaultValue: c.DefaultValue, OnValueChange: c.OnValueChange, OnValueCommit: c.OnValueCommit,
		Alpha: c.Alpha, Swatches: c.Swatches, Disabled: c.Disabled, Class: c.PanelClass,
		AreaLabel: c.AreaLabel, HueLabel: c.HueLabel, AlphaLabel: c.AlphaLabel, HexLabel: c.HexLabel,
		Direction: c.Direction,
	}
	parts := useColorParts(p)
	fc := tela.UseContext(fieldCtx)
	label := cmp.Or(c.AriaLabel, fc.label)
	trig := c.Trigger
	if trig == nil {
		code := aria.FormatHexColor(parts.color, c.Alpha)
		props := []tela.Child{c.Props}
		if id := cmp.Or(c.ID, fc.id); id != "" {
			props = append(props, h.ID(id))
		}
		if fc.describedBy != (tela.ElementRef{}) {
			props = append(props, h.AriaDescribedBy(fc.describedBy))
		}
		if fc.invalid {
			props = append(props, h.AriaInvalid(true))
		}
		name := ""
		if label != "" {
			name = label + ", " + code
		}
		trig = Button{Variant: Outline, Disabled: c.Disabled, Class: joinClass(r.Trigger, c.Class), AriaLabel: name,
			Props: tela.MergeProps(props...), Children: colorInputLabel{st: parts.st, alpha: c.Alpha}}
	}
	return h.Fragment(Popover{
		Trigger: trig, Placement: c.Placement, Align: orStr(c.Align, "start"), Offset: c.Offset,
		Open: c.Open, DefaultOpen: c.DefaultOpen, OnOpenChange: c.OnOpenChange, Disabled: c.Disabled,
		AriaLabel: orStr(label, "Color picker"), Class: c.PopoverClass,
		Children: colorPanel{p: p, parts: parts},
	})
}

func orStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// colorInputLabel is the content of the default trigger: the swatch and the
// code, both following the color live (the code is a paint-phase text, so a
// drag in the panel changes it without a build).
type colorInputLabel struct {
	st    *aria.ColorState
	alpha bool
}

func (l colorInputLabel) Build() tela.Node {
	style := UseStyle()
	r := &style.ColorInput
	st, alpha := l.st, l.alpha
	return h.Fragment(colorSwatch(st, style, r.Swatch),
		h.Span(cls(r.Code, when(alpha, r.CodeAlpha)), h.Dir("ltr"),
			h.TextFn(func() string { return aria.FormatHexColor(st.LiveColor(), alpha) })))
}
