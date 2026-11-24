package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"

	"github.com/kuronosu/tela/examples/canvas/icons"
)

// metric is one series of the chart: twelve months of something.
type metric struct {
	Key, Label string
	Values     []float64
	// Format writes a value as the axis, the tooltip and the text do.
	Format func(float64) string
}

var months = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

func thousands(prefix string) func(float64) string {
	return func(v float64) string { return fmt.Sprintf("%s%.1fk", prefix, v) }
}

func whole(v float64) string {
	s := fmt.Sprintf("%.0f", v)
	if len(s) > 3 {
		s = s[:len(s)-3] + "," + s[len(s)-3:]
	}
	return s
}

var metrics = []metric{
	{"visitors", "Visitors", []float64{8.2, 9.1, 8.7, 10.4, 11.9, 11.2, 13.6, 15.1, 14.4, 16.8, 18.2, 17.4}, thousands("")},
	{"signups", "Sign-ups", []float64{420, 465, 510, 480, 590, 705, 668, 790, 862, 840, 975, 1120}, whole},
	{"revenue", "Revenue", []float64{31.2, 28.4, 33.9, 36.5, 35.1, 41.8, 44.6, 43.2, 49.9, 53.4, 51.7, 58.3}, thousands("$")},
}

func metricOf(key string) metric {
	for _, m := range metrics {
		if m.Key == key {
			return m
		}
	}
	return metrics[0]
}

// morph is the series on its way to another one: what the chart shows is
// From at 0 and To at 1, and a spring moves between them.
type morph struct{ From, To []float64 }

func (m morph) at(t float64) []float64 {
	out := make([]float64, len(m.To))
	for i := range out {
		out[i] = m.From[i] + (m.To[i]-m.From[i])*t
	}
	return out
}

// niceMax is the top of the axis for values that reach max: four equal steps
// of 1, 2, 2.5, 5 or 10 times a power of ten.
func niceMax(max float64) float64 {
	raw := max / 4
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, f := range []float64{1, 2, 2.5, 5, 10} {
		if f*mag*4 >= max {
			return f * mag * 4
		}
	}
	return 10 * mag * 4
}

func maxOf(v []float64) float64 {
	m := v[0]
	for _, x := range v {
		m = max(m, x)
	}
	return m
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

// ChartCard is the area chart with the buttons that choose its series.
// Probe is the point (0 to 11) that shows its value before the pointer is
// over the chart, or -1.
type ChartCard struct{ Probe int }

func (cc ChartCard) Build() tela.Node {
	key, setKey := tela.UseState(metrics[0].Key)
	m := metricOf(key)
	// shown moves from one series to the next; it is the only thing that
	// changes while the spring runs, and the draw below reads it.
	shown := tela.UseRef(morph{From: m.Values, To: m.Values})
	progress := tela.UseValue(float32(1))
	cursor := tela.UseValue(float32(-1)) // the pointer's x in the canvas, or -1

	choose := func(k string) {
		if k == key {
			return
		}
		cur := shown.Current
		shown.Current = morph{From: cur.at(float64(progress.Get())), To: metricOf(k).Values}
		setKey.Set(k)
		progress.Set(0)
		tela.Animate(progress, 1, tela.Stiffness(170), tela.Damping(21))
	}

	values := shown.Current.To
	peak, peakAt := values[0], 0
	sum := 0.0
	for i, v := range values {
		sum += v
		if v > peak {
			peak, peakAt = v, i
		}
	}
	last, first := values[len(values)-1], values[0]
	change := (last - first) / first * 100

	items := make([]kit.ToggleGroupItem[string], len(metrics))
	for i, mt := range metrics {
		items[i] = kit.ToggleGroupItem[string]{Value: mt.Key, Label: mt.Label}
	}

	// The values as text, for what cannot see the drawing.
	var desc strings.Builder
	desc.WriteString(m.Label + " by month: ")
	for i, v := range values {
		if i > 0 {
			desc.WriteString(", ")
		}
		fmt.Fprintf(&desc, "%s %s", months[i], m.Format(v))
	}
	desc.WriteString(".")

	stat := func(label, value string) tela.Node {
		return Div(Class("flex flex-col gap-0.5"),
			Span(Class("text-xs text-muted-foreground"), Text(label)),
			Span(Class("text-xl font-semibold tracking-tight tabular-nums"), Text(value)))
	}
	trend := "text-emerald-700 dark:text-emerald-400"
	if change < 0 {
		trend = "text-destructive"
	}

	return kit.Card{
		TitleLevel: 2,
		Class:      "min-w-0", HeaderClass: tight, ContentClass: tight, FooterClass: tight,
		Title:       cardTitle(icons.ChartArea, "Twelve months"),
		Description: Text("Move the pointer over the chart to read a point."),
		Action: kit.ToggleGroup[string]{
			Items: items, Variant: kit.Outline, Size: kit.Sm, AriaLabel: "Series", DisallowEmptySelection: true,
			Value: &key, OnValueChange: choose,
		},
		Children: Div(Class("flex flex-col gap-4"),
			Div(Class("flex flex-wrap gap-x-10 gap-y-3"),
				stat("Latest (Dec)", m.Format(last)),
				stat("Peak ("+months[peakAt]+")", m.Format(peak)),
				stat("Average", m.Format(sum/float64(len(values)))),
				Div(Class("flex flex-col gap-0.5"),
					Span(Class("text-xs text-muted-foreground"), Text("Since January")),
					Span(Class("text-xl font-semibold tracking-tight tabular-nums "+trend), Textf("%+.0f%%", change)))),
			Canvas(func(c *tela.Canvas) {
				drawChart(c, chart{
					metric: m, values: shown.Current.at(float64(progress.Get())),
					yMax:   lerp(niceMax(maxOf(shown.Current.From)), niceMax(maxOf(shown.Current.To)), float64(progress.Get())),
					cursor: float64(cursor.Get()), probe: cc.Probe,
				})
			},
				Class("block h-48 w-full cursor-crosshair"),
				TestID("chart"), Role("img"), AriaLabel(m.Label+" by month, area chart"),
				OnPointerMove(func(e tela.PointerEvent) { cursor.Set(e.LocalX) }),
				OnPointerLeave(func() { cursor.Set(-1) })),
			P(Class("sr-only"), Text(desc.String()))),
	}
}

// chart is what drawChart draws: a series (the values on their way, and the
// top of the axis on its way), where the pointer is and the point to read
// when it is nowhere.
type chart struct {
	metric metric
	values []float64
	yMax   float64
	cursor float64
	probe  int
}

// drawChart draws the axes, the area, the line and the tooltip. The labels
// are measured, so the plot starts after the widest one and the tooltip is
// as wide as its text.
func drawChart(c *tela.Canvas, ch chart) {
	n := len(ch.values)
	fg, muted, border := c.Color("foreground"), c.Color("muted-foreground"), c.Color("border")
	line := c.Color("chart-1")

	c.Font.Size, c.Font.Weight = 11, 500
	c.TextBaseline = tela.BaselineMiddle
	var widest float64
	ticks := make([]string, 5)
	for i := range ticks {
		ticks[i] = ch.metric.Format(ch.yMax * float64(i) / 4)
		widest = max(widest, c.MeasureText(ticks[i]).Width)
	}
	left, right, top, bottom := widest+14, 14.0, 12.0, 26.0
	w, h := c.Width-left-right, c.Height-top-bottom
	x := func(i int) float64 { return left + w*float64(i)/float64(n-1) }
	y := func(v float64) float64 { return top + h*(1-v/ch.yMax) }

	// Grid and the labels of the value axis.
	c.LineWidth = 1
	c.StrokeStyle = border
	c.FillStyle = muted
	c.TextAlign = tela.TextAlignRight
	c.SetLineDash(3, 4)
	for i, label := range ticks {
		gy := math.Round(y(ch.yMax*float64(i)/4)) + 0.5
		if i == 0 {
			c.SetLineDash()
		}
		c.BeginPath()
		c.MoveTo(left, gy)
		c.LineTo(left+w, gy)
		c.Stroke()
		c.FillText(label, left-8, gy)
	}
	c.SetLineDash()

	// The months, every other one when they do not fit.
	c.TextAlign, c.TextBaseline = tela.TextAlignCenter, tela.BaselineTop
	step := 1
	if w/float64(n-1) < c.MeasureText("Mmm").Width+10 {
		step = 2
	}
	for i := 0; i < n; i += step {
		c.FillText(months[i], x(i), top+h+9)
	}

	// The area under a smooth line, then the line.
	curve := func() {
		c.MoveTo(x(0), y(ch.values[0]))
		slope := monotone(ch.values, w/float64(n-1), h/ch.yMax)
		dx := w / float64(n-1) / 3
		for i := 0; i < n-1; i++ {
			c.BezierCurveTo(x(i)+dx, y(ch.values[i])+slope[i]*dx, x(i+1)-dx, y(ch.values[i+1])-slope[i+1]*dx, x(i+1), y(ch.values[i+1]))
		}
	}
	fill := c.CreateLinearGradient(0, top, 0, top+h)
	fill.AddColorStop(0, line.WithAlpha(0.38))
	fill.AddColorStop(1, line.WithAlpha(0))
	c.BeginPath()
	curve()
	c.LineTo(x(n-1), top+h)
	c.LineTo(x(0), top+h)
	c.ClosePath()
	c.FillStyle = fill
	c.Fill()

	c.BeginPath()
	curve()
	c.StrokeStyle, c.LineWidth, c.LineJoin, c.LineCap = line, 2.5, tela.JoinRound, tela.CapRound
	c.Stroke()

	// The cursor snaps to the nearest point.
	at := ch.probe
	if ch.cursor >= 0 {
		at = int(math.Round((ch.cursor - left) / w * float64(n-1)))
	}
	if at < 0 || at >= n || (ch.cursor >= 0 && (ch.cursor < left-12 || ch.cursor > left+w+12)) {
		return
	}
	px, py := x(at), y(ch.values[at])
	c.StrokeStyle, c.LineWidth = muted.WithAlpha(0.7), 1
	c.SetLineDash(4, 4)
	c.BeginPath()
	c.MoveTo(math.Round(px)+0.5, top)
	c.LineTo(math.Round(px)+0.5, top+h)
	c.Stroke()
	c.SetLineDash()

	c.FillStyle = line.WithAlpha(0.2)
	c.BeginPath()
	c.Arc(px, py, 9, 0, 2*math.Pi)
	c.Fill()
	c.FillStyle, c.StrokeStyle, c.LineWidth = c.Color("card"), line, 2.5
	c.BeginPath()
	c.Arc(px, py, 5, 0, 2*math.Pi)
	c.Fill()
	c.Stroke()

	// The tooltip: the month and the value, on the side with room.
	value := ch.metric.Format(ch.values[at])
	c.Font.Size, c.Font.Weight = 14, 600
	vw := c.MeasureText(value).Width
	c.Font.Size, c.Font.Weight = 11, 500
	mw := c.MeasureText(months[at]).Width
	bw, bh := max(vw, mw)+24, 46.0
	bx := px + 14
	if bx+bw > c.Width-4 {
		bx = px - 14 - bw
	}
	by := min(max(py-bh/2, 2), top+h-bh)
	c.Save()
	c.ShadowColor, c.ShadowBlur, c.ShadowOffsetY = tela.Black.WithAlpha(0.16), 14, 4
	c.FillStyle = c.Color("popover")
	c.BeginPath()
	c.RoundRect(bx, by, bw, bh, 8)
	c.Fill()
	c.Restore()
	c.StrokeStyle, c.LineWidth = border, 1
	c.BeginPath()
	c.RoundRect(bx+0.5, by+0.5, bw-1, bh-1, 8)
	c.Stroke()
	c.TextAlign, c.TextBaseline = tela.TextAlignLeft, tela.BaselineMiddle
	c.FillStyle = muted
	c.FillText(months[at], bx+12, by+14)
	c.FillStyle = fg
	c.Font.Size, c.Font.Weight = 14, 600
	c.FillText(value, bx+12, by+31)
}

// monotone returns the slope of the curve at each point, in pixels of y per
// pixel of x, so that the curve through the points never goes above or
// below them (Fritsch and Carlson's monotone cubic). dx and sy are the
// distance between two points and the pixels of a unit of value.
func monotone(v []float64, dx, sy float64) []float64 {
	n := len(v)
	d := make([]float64, n-1) // the secants, in pixels of y (down) per pixel of x
	for i := range d {
		d[i] = -(v[i+1] - v[i]) * sy / dx
	}
	m := make([]float64, n)
	m[0], m[n-1] = d[0], d[n-2]
	for i := 1; i < n-1; i++ {
		if d[i-1]*d[i] > 0 {
			m[i] = (d[i-1] + d[i]) / 2
		}
	}
	for i := range d {
		if d[i] == 0 {
			m[i], m[i+1] = 0, 0
			continue
		}
		a, b := m[i]/d[i], m[i+1]/d[i]
		if s := a*a + b*b; s > 9 {
			t := 3 / math.Sqrt(s)
			m[i], m[i+1] = t*a*d[i], t*b*d[i]
		}
	}
	return m
}
