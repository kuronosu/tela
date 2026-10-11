package gfx

import (
	"math"
	"slices"
	"testing"
)

func TestGaussLegendreRule(t *testing.T) {
	// The 8-point rule integrates x^k exactly on [-1, 1] for k <= 15.
	for k := range 16 {
		got := 0.0
		for i := range 4 {
			got += glWeights[i] * (math.Pow(glNodes[i], float64(k)) + math.Pow(-glNodes[i], float64(k)))
		}
		want := 0.0
		if k%2 == 0 {
			want = 2 / float64(k+1)
		}
		if math.Abs(got-want) > 1e-15 {
			t.Errorf("x^%d: %v, want %v", k, got, want)
		}
	}
}

// polyArcLen measures a segment by summing many chords.
func polyArcLen(s *cseg, t0, t1 float64) float64 {
	const n = 100000
	l := 0.0
	prev := s.eval(t0)
	for i := 1; i <= n; i++ {
		q := s.eval(t0 + (t1-t0)*float64(i)/n)
		l += q.sub(prev).norm()
		prev = q
	}
	return l
}

func TestArcLength(t *testing.T) {
	segs := []cseg{
		cubicSeg(v2{0, 0}, v2{100, 0}, v2{-20, 100}, v2{100, 100}),
		cubicSeg(v2{0, 0}, v2{150, 100}, v2{-50, 100}, v2{100, 0}),
		cubicSeg(v2{0, 0}, v2{100, 100}, v2{0, 100}, v2{100, 0}), // cusp
		cubicSeg(v2{0, 0}, v2{0, 0}, v2{30, 40}, v2{30, 40}),     // straight, zero end speeds
		quadSeg(v2{0, 0}, v2{50, 100}, v2{100, 0}),
	}
	const acc = 1e-6
	for i := range segs {
		s := &segs[i]
		sl := s.measure(acc)
		if want := polyArcLen(s, 0, 1); math.Abs(sl.l-want) > 1e-5 {
			t.Errorf("seg %d: length %v, want %v", i, sl.l, want)
		}
		for _, d := range []float64{0, 0.1, 13.7, sl.l / 2, sl.l * 0.93, sl.l} {
			u := s.invArcLen(&sl, d, acc)
			if got := polyArcLen(s, 0, u); math.Abs(got-d) > 1e-5 {
				t.Errorf("seg %d: invArcLen(%v) = %v, whose length is %v", i, d, u, got)
			}
		}
	}
	if want := 50.0; math.Abs(segs[3].measure(acc).l-want) > 1e-6 {
		t.Errorf("straight cubic: length %v, want 50", segs[3].measure(acc).l)
	}
}

// dashList dashes p and returns, for each dash, its points (start and the
// segment end points) and whether it is closed.
type dashOut struct {
	pts    []v2
	closed bool
	length float64
}

func dashPath(t *testing.T, p Path, pattern []float32, offset float32) []dashOut {
	t.Helper()
	st, ok := sanitizeStroke(StrokeStyle{Width: 1, Dashes: pattern, DashOffset: offset})
	if !ok || st.Dashes == nil {
		t.Fatalf("invalid pattern %v", pattern)
	}
	subs, ok := dashSubpaths(pathSubpaths(p), st.Dashes, st.DashOffset, 0.01)
	if !ok {
		t.Fatal("dashing refused")
	}
	var out []dashOut
	for _, sp := range subs {
		if !sp.draws {
			continue
		}
		d := dashOut{pts: []v2{sp.start}, closed: sp.closed}
		for k := range sp.segs {
			d.pts = append(d.pts, sp.segs[k].end())
			d.length += sp.segs[k].measure(1e-9).l
		}
		out = append(out, d)
	}
	return out
}

func roundV(q v2) v2 { return v2{math.Round(q.x*1e6) / 1e6, math.Round(q.y*1e6) / 1e6} }

func dashPts(ds []dashOut) [][]v2 {
	var out [][]v2
	for _, d := range ds {
		var pts []v2
		for _, q := range d.pts {
			pts = append(pts, roundV(q))
		}
		out = append(out, pts)
	}
	return out
}

func sortDashes(ds [][]v2) {
	slices.SortFunc(ds, func(a, b []v2) int {
		if a[0].x != b[0].x {
			if a[0].x < b[0].x {
				return -1
			}
			return 1
		}
		if a[0].y < b[0].y {
			return -1
		}
		if a[0].y > b[0].y {
			return 1
		}
		return 0
	})
}

func TestDashKurboSequences(t *testing.T) {
	line := polyPath(false, Pt(0, 0), Pt(21, 0))
	// kurbo's dash_sequence: [1, 5, 2, 5] on a line of length 21.
	got := dashPts(dashPath(t, line, []float32{1, 5, 2, 5}, 0))
	sortDashes(got)
	want := [][]v2{{{0, 0}, {1, 0}}, {{6, 0}, {8, 0}}, {{13, 0}, {14, 0}}, {{19, 0}, {21, 0}}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("dash sequence: %v, want %v", got, want)
	}
	// kurbo's dash_sequence_offset: offset 3.
	got = dashPts(dashPath(t, line, []float32{1, 5, 2, 5}, 3))
	sortDashes(got)
	want = [][]v2{{{3, 0}, {5, 0}}, {{10, 0}, {11, 0}}, {{16, 0}, {18, 0}}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("dash sequence offset: %v, want %v", got, want)
	}
	// kurbo's dash_sequence_closed_path: the dash running through the start
	// point is joined with the first dash.
	got = dashPts(dashPath(t, RectPath(Rect{0, 0, 4, 4}), []float32{5, 1}, 0))
	sortDashes(got)
	want = [][]v2{{{0, 4}, {0, 0}, {4, 0}, {4, 1}}, {{4, 2}, {4, 4}, {1, 4}}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("dash closed path: %v, want %v", got, want)
	}
	// kurbo's dash_negative_offset: -60 and 60 agree (period 6).
	line28 := polyPath(false, Pt(0, 0), Pt(28, 0))
	pos := dashPts(dashPath(t, line28, []float32{4, 2}, 60))
	neg := dashPts(dashPath(t, line28, []float32{4, 2}, -60))
	if !slices.EqualFunc(pos, neg, slices.Equal) {
		t.Errorf("negative offset: %v, positive: %v", neg, pos)
	}
	// Odd patterns are doubled (kurbo's dash_odd_length_matches_doubled).
	line50 := polyPath(false, Pt(0, 0), Pt(50, 0))
	for _, off := range []float32{0, 5, 9, 10, 11, 15, 20, 25, 100, -7} {
		a := dashPts(dashPath(t, line50, []float32{10}, off))
		b := dashPts(dashPath(t, line50, []float32{10, 10}, off))
		if !slices.EqualFunc(a, b, slices.Equal) {
			t.Errorf("odd pattern, offset %v: %v vs %v", off, a, b)
		}
	}
}

// onLength returns the total length of the "on" parts of the pattern over
// [0, l] for a straight open path.
func onLength(l float64, pattern []float64, offset float64) float64 {
	period := 0.0
	for _, d := range pattern {
		period += d
	}
	off := math.Mod(offset, period)
	if off < 0 {
		off += period
	}
	total := 0.0
	pos := -off
	for i := 0; pos < l; i++ {
		d := pattern[i%len(pattern)]
		if i%2 == 0 {
			total += math.Max(0, math.Min(l, pos+d)-math.Max(0, pos))
		}
		pos += d
	}
	return total
}

func TestDashLengthsOnLine(t *testing.T) {
	line := polyPath(false, Pt(5, 7), Pt(105, 7))
	for _, pattern := range [][]float32{{10, 5}, {3, 1, 7, 2}, {0.5, 0.25}, {30, 70}} {
		p64 := []float64{}
		for _, d := range pattern {
			p64 = append(p64, float64(d))
		}
		for _, off := range []float32{0, 3, 12, -3, -12.5, 100.5, -100.5, 1e4} {
			want := onLength(100, p64, float64(off))
			got := 0.0
			for _, d := range dashPath(t, line, pattern, off) {
				got += d.length
			}
			if math.Abs(got-want) > 1e-4 {
				t.Errorf("pattern %v offset %v: dash length %v, want %v", pattern, off, got, want)
			}
			// The stroke with butt caps covers exactly that length.
			out := line.Stroke(StrokeStyle{Width: 2, Dashes: pattern, DashOffset: off}, 0.01)
			if area := signedArea(outlineLoops(out)); math.Abs(area-2*want) > 1e-3 {
				t.Errorf("pattern %v offset %v: stroke area %v, want %v", pattern, off, area, 2*want)
			}
		}
	}
}

func TestDashOnCurves(t *testing.T) {
	// Half of a circle's length is on with an equal dash and gap.
	r := 50.0
	circ := CirclePath(Pt(0, 0), float32(r))
	got := 0.0
	ds := dashPath(t, circ, []float32{10, 10}, 3)
	for _, d := range ds {
		got += d.length
	}
	// The cubic circle is slightly longer than the true circle.
	var total float64
	for _, sp := range pathSubpaths(circ) {
		for k := range sp.segs {
			total += sp.segs[k].measure(1e-9).l
		}
	}
	if want := onLength(total, []float64{10, 10}, 3); math.Abs(got-want) > 1e-3 {
		t.Errorf("circle: on length %v, want %v", got, want)
	}
	if math.Abs(total-2*math.Pi*r) > 0.01 {
		t.Errorf("circle length %v", total)
	}
	// Every dash is open, and consecutive dashes are separated by 10.
	for _, d := range ds {
		if d.closed {
			t.Errorf("unexpected closed dash")
		}
	}
	// A dash longer than the circle keeps it closed (joined at the start,
	// no caps).
	ds = dashPath(t, circ, []float32{1000, 10}, 0)
	if len(ds) != 1 || !ds[0].closed {
		t.Fatalf("long dash on a circle: %d dashes, closed %v", len(ds), len(ds) > 0 && ds[0].closed)
	}
	// Gap at the start of a closed path: nothing is merged.
	ds = dashPath(t, RectPath(Rect{0, 0, 10, 10}), []float32{5, 5}, 5)
	if len(ds) != 4 {
		t.Errorf("rect with gap first: %d dashes, want 4", len(ds))
	}
}

func TestDashZeroLengthDots(t *testing.T) {
	line := polyPath(false, Pt(0, 0), Pt(100, 0))
	// [0, 10]: dots at 0, 10, ..., 90. As in kurbo, a dash transition
	// exactly at the end of an open path is not taken, so there is no dot
	// at 100.
	out := line.Stroke(StrokeStyle{Width: 4, Cap: CapSquare, Dashes: []float32{0, 10}}, 0.01)
	if n := countVerb(out, MoveTo); n != 10 {
		t.Errorf("square dots: %d subpaths, want 10", n)
	}
	if area := signedArea(outlineLoops(out)); math.Abs(area-10*16) > 1e-6 {
		t.Errorf("square dots: area %v, want %v", area, 10*16)
	}
	if butt := line.Stroke(StrokeStyle{Width: 4, Dashes: []float32{0, 10}}, 0.01); !butt.IsEmpty() {
		t.Errorf("butt zero dashes: not empty")
	}
	// On a diagonal, the square dots follow the path direction.
	diag := polyPath(false, Pt(0, 0), Pt(30, 40))
	out = diag.Stroke(StrokeStyle{Width: 4, Cap: CapSquare, Dashes: []float32{0, 25}}, 0.01)
	pts := out.Points()
	if len(pts) < 4 {
		t.Fatalf("diagonal dots: %v", pts)
	}
	e := Pt(pts[1].X-pts[0].X, pts[1].Y-pts[0].Y)
	if math.Abs(float64(e.X*0.6+e.Y*0.8)) > 1e-4 && math.Abs(float64(e.X*0.8-e.Y*0.6)) > 1e-4 {
		t.Errorf("diagonal square dot edge %v is not aligned with the path", e)
	}
	// On a closed path, a dot at the start is not drawn twice.
	out = RectPath(Rect{0, 0, 10, 10}).Stroke(StrokeStyle{Width: 2, Cap: CapRound, Dashes: []float32{0, 5}}, 0.01)
	if n := countVerb(out, MoveTo); n != 8 {
		t.Errorf("round dots on a closed path: %d, want 8", n)
	}
}

func TestDashTooMany(t *testing.T) {
	// A tiny period on a long path is stroked solid.
	line := polyPath(false, Pt(0, 0), Pt(1e6, 0))
	st := StrokeStyle{Width: 2, Dashes: []float32{1e-3, 1e-3}}
	out := line.Stroke(st, 0.25)
	if area := signedArea(outlineLoops(out)); math.Abs(area-2e6) > 1 {
		t.Errorf("too many dashes: area %v, want the solid stroke", area)
	}
}
