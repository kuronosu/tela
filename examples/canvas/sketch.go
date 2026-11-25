package main

import (
	"math"
	"slices"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"

	"github.com/kuronosu/tela/examples/canvas/icons"
)

// pt is a point of a stroke, in the canvas's pixels.
type pt struct{ X, Y float64 }

// stroke is what one press, drag and release drew: the theme token of its
// ink (so that it follows the color scheme), its width, whether it erases,
// and where the pointer was.
type stroke struct {
	Ink   string
	Width float64
	Erase bool
	Pts   []pt
}

// drawStrokes draws the pad: every stroke over the one before it, smooth
// through the middle of each pair of points. An eraser stroke takes away what
// is under it (CompositeDestinationOut), whatever its color: what shows
// afterwards is the pad's own background, which is not part of the canvas.
func drawStrokes(c *tela.Canvas, strokes []stroke) {
	if len(strokes) == 0 {
		c.Font.Size = 14
		c.FillStyle = c.Color("muted-foreground")
		c.TextAlign, c.TextBaseline = tela.TextAlignCenter, tela.BaselineMiddle
		c.FillText("Draw here with the pointer", c.Width/2, c.Height/2)
		return
	}
	c.LineCap, c.LineJoin = tela.CapRound, tela.JoinRound
	for _, s := range strokes {
		c.Save()
		c.LineWidth = s.Width
		ink := c.Color(s.Ink)
		if s.Erase {
			c.GlobalCompositeOperation = tela.CompositeDestinationOut
			ink = tela.Black
		}
		c.StrokeStyle, c.FillStyle = ink, ink
		c.BeginPath()
		p := s.Pts
		if len(p) == 1 { // a click: a dot
			c.Arc(p[0].X, p[0].Y, s.Width/2, 0, 2*math.Pi)
			c.Fill()
			c.Restore()
			continue
		}
		c.MoveTo(p[0].X, p[0].Y)
		for i := 1; i < len(p)-1; i++ {
			c.QuadraticCurveTo(p[i].X, p[i].Y, (p[i].X+p[i+1].X)/2, (p[i].Y+p[i+1].Y)/2)
		}
		c.LineTo(p[len(p)-1].X, p[len(p)-1].Y)
		c.Stroke()
		c.Restore()
	}
}

// The inks and the brushes, in the order of their buttons.
var inks = []struct{ Key, Name, Class string }{
	{"foreground", "Ink", "bg-foreground"},
	{"chart-1", "Indigo", "bg-chart-1"},
	{"chart-3", "Pink", "bg-chart-3"},
	{"chart-4", "Amber", "bg-chart-4"},
	{"chart-5", "Green", "bg-chart-5"},
}

var brushes = []struct {
	Width float64
	Name  string
	Dot   string
}{
	{3, "Thin", "size-[3px]"},
	{6, "Medium", "size-[6px]"},
	{12, "Thick", "size-[12px]"},
}

// eraserScale makes the eraser wider than the brush of the same name.
const eraserScale = 2.5

// along makes a stroke of the points a pointer would give for a line through
// corners: one every few pixels, as it moves.
func along(corners ...pt) []pt {
	out := []pt{corners[0]}
	for i := 1; i < len(corners); i++ {
		a, b := corners[i-1], corners[i]
		n := int(math.Ceil(math.Hypot(b.X-a.X, b.Y-a.Y) / 4))
		for k := 1; k <= n; k++ {
			f := float64(k) / float64(n)
			out = append(out, pt{a.X + (b.X-a.X)*f, a.Y + (b.Y-a.Y)*f})
		}
	}
	return out
}

// doodle is the drawing of the previews: cursive loops with an underline, a
// ring that the eraser has crossed and a zigzag, made of points as the
// pointer would make them.
var doodle = func() []stroke {
	var loops, under, ring []pt
	for t := 0.0; t <= 4.5*math.Pi; t += 0.1 {
		loops = append(loops, pt{26 + 15*t - 24*math.Sin(t), 62 - 24*math.Cos(t)})
	}
	for x := 30.0; x <= 250; x += 4 {
		under = append(under, pt{x, 130 - 10*math.Sin((x-30)/220*math.Pi)})
	}
	for a := 0.0; a <= 2*math.Pi+0.2; a += 0.1 {
		ring = append(ring, pt{352 + 36*math.Cos(a), 66 + 36*math.Sin(a)})
	}
	return []stroke{
		{"chart-1", 4, false, loops},
		{"chart-4", 6, false, under},
		{"chart-3", 9, false, ring},
		{"chart-5", 5, false, along(pt{300, 140}, pt{322, 118}, pt{344, 140}, pt{366, 118}, pt{388, 140}, pt{410, 118}, pt{432, 140})},
		{"foreground", 26, true, along(pt{316, 36}, pt{392, 104})},
	}
}()

// SketchCard is the pad with its tools. Doodle puts the drawing of the
// previews on it.
type SketchCard struct{ Doodle bool }

func (sc SketchCard) Build() tela.Node {
	var seed []stroke
	if sc.Doodle {
		seed = doodle
	}
	// The strokes are a Value that the draw reads: a stroke grows without a
	// build of this component, and only the canvas is painted again.
	strokes := tela.UseValue(seed)
	tool, setTool := tela.UseState("pen")
	ink, setInk := tela.UseState(inks[1].Key)
	width, setWidth := tela.UseState(brushes[1].Width)
	// hasInk is read by the buttons that take strokes back; it changes only
	// when the pad goes from empty to drawn on, or back, so drawing builds
	// nothing.
	hasInk := tela.UseValue(len(seed) > 0)
	drawing := tela.UseRef(false)

	add := func(p pt) {
		strokes.Update(func(s []stroke) []stroke {
			if len(s) == 0 {
				return s
			}
			last := s[len(s)-1]
			last.Pts = append(slices.Clip(last.Pts), p)
			return append(slices.Clip(s[:len(s)-1]), last)
		})
	}
	// Taking a stroke back ends the one in progress: what the pointer draws
	// next must not go on the stroke before it.
	undo := func() {
		drawing.Current = false
		s := strokes.Latest()
		s = slices.Clip(s[:max(len(s)-1, 0)])
		strokes.Set(s)
		hasInk.Set(len(s) > 0)
	}
	wipe := func() {
		drawing.Current = false
		strokes.Set(nil)
		hasInk.Set(false)
	}

	tools := kit.ToggleGroup[string]{
		AriaLabel: "Tool", Variant: kit.Outline, Size: kit.Sm, DisallowEmptySelection: true,
		Value: &tool, OnValueChange: setTool.Set,
		Items: []kit.ToggleGroupItem[string]{
			{Value: "pen", Label: "Pen", Icon: icons.Pencil},
			{Value: "eraser", Label: "Eraser", Icon: icons.Eraser},
		},
	}
	colors := make([]kit.ToggleGroupItem[string], len(inks))
	for i, k := range inks {
		colors[i] = kit.ToggleGroupItem[string]{Value: k.Key, AriaLabel: k.Name,
			Children: Span(Class("size-3.5 rounded-full " + k.Class))}
	}
	sizes := make([]kit.ToggleGroupItem[float64], len(brushes))
	for i, b := range brushes {
		sizes[i] = kit.ToggleGroupItem[float64]{Value: b.Width, AriaLabel: b.Name,
			Children: Span(Class("rounded-full bg-current " + b.Dot))}
	}

	return kit.Card{
		TitleLevel: 2,
		Class:      "min-w-0", HeaderClass: tight, ContentClass: tight, FooterClass: tight,
		Title:       cardTitle(icons.PenLine, "Sketch pad"),
		Description: Text("Draw, erase and undo. Ctrl+Z works too."),
		Action:      History{HasInk: hasInk, Undo: undo, Clear: wipe},
		Props: OnKeyDown(func(e tela.KeyboardEvent) {
			if e.CmdOrCtrl() && e.Key == "z" {
				e.PreventDefault()
				undo()
			}
		}),
		// LocalX and LocalY are where the canvas draws, inside its border.
		Children: Canvas(func(c *tela.Canvas) { drawStrokes(c, strokes.Get()) },
			Class("block h-44 w-full cursor-crosshair rounded-lg border border-border bg-muted/40",
				"outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"),
			TestID("sketch"), Role("img"), AriaLabel("Sketch pad"), TabIndex(0),
			OnPointerDown(func(e tela.PointerEvent) {
				w := width
				if tool == "eraser" {
					w *= eraserScale
				}
				drawing.Current = true
				hasInk.Set(true)
				p := pt{float64(e.LocalX), float64(e.LocalY)}
				strokes.Update(func(s []stroke) []stroke {
					return append(slices.Clip(s), stroke{Ink: ink, Width: w, Erase: tool == "eraser", Pts: []pt{p}})
				})
			}),
			OnPointerMove(func(e tela.PointerEvent) {
				if drawing.Current {
					add(pt{float64(e.LocalX), float64(e.LocalY)})
				}
			}),
			OnPointerUp(func() { drawing.Current = false }),
			OnPointerCancel(func() { drawing.Current = false })),
		Footer: Div(Class("flex flex-wrap items-center gap-x-3 gap-y-2"),
			tools,
			kit.ToggleGroup[string]{AriaLabel: "Color", Variant: kit.Outline, Size: kit.Sm, DisallowEmptySelection: true,
				Value: &ink, OnValueChange: func(k string) { setInk.Set(k); setTool.Set("pen") }, Items: colors, ItemClass: "px-2.5"},
			kit.ToggleGroup[float64]{AriaLabel: "Brush", Variant: kit.Outline, Size: kit.Sm, DisallowEmptySelection: true,
				Value: &width, OnValueChange: setWidth.Set, Items: sizes, ItemClass: "px-2.5"}),
	}
}

// History is the undo and clear buttons, which are off while there is nothing
// to take back.
type History struct {
	HasInk      *tela.Value[bool]
	Undo, Clear func()
}

func (hs History) Build() tela.Node {
	empty := !hs.HasInk.Get()
	return Div(Class("flex items-center gap-1"),
		kit.Button{Variant: kit.Ghost, Size: kit.Sm, Icon: icons.Undo2, Disabled: empty, OnClick: hs.Undo, Children: Text("Undo")},
		kit.Button{Variant: kit.Ghost, Size: kit.Sm, Icon: icons.Trash2, Disabled: empty, OnClick: hs.Clear, Children: Text("Clear")})
}
