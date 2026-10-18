// Copyright (c) 2018 Visly Inc. and Taffy contributors. MIT License.
// Ported from Taffy v0.14.0 (src/compute/grid/types/{coordinates,
// grid_track,grid_track_counts,named}.rs, src/compute/grid/explicit_grid.rs
// and src/compute/grid/implicit_grid.rs); see THIRD_PARTY.md.
//
// Named lines follow CSS Grid 1 section 8.3 where Taffy differs: a lone
// identifier tries <name>-start/<name>-end before <name>; <integer> &&
// <name> counts only lines named <name>; counting past the named lines
// continues into the implicit lines on the side of the search (Taffy
// counts negative indexes back into the explicit grid); span <name> on the
// start edge searches startward from the end line.

package layout

import (
	"slices"
	"sort"
)

// maxGridTracks limits the grid to maxGridTracks tracks in each direction
// from the start of the explicit grid, and the span of an item (CSS Grid 1
// section 7.9; Taffy's limit, which Chromium 152 agrees with on the
// fixtures).
const maxGridTracks = 10000

// Grid lines are int in two coordinate systems. CSS line numbers (in
// styles) count from 1 at the start of the explicit grid and from -1 at its
// end; 0 is invalid. Origin-zero (OZ) lines count from 0 at the start of
// the explicit grid, negative before it; they are clamped to
// [-maxGridTracks, maxGridTracks].

// ozFromCSS converts CSS line n (not 0) to an origin-zero line.
func ozFromCSS(n, explicit int) int {
	// Taffy stores lines as i16: larger values saturate first.
	n = min(max(n, -32768), 32767)
	var oz int
	if n > 0 {
		oz = n - 1
	} else {
		oz = n + explicit + 1
	}
	return clampOZ(oz)
}

func clampOZ(l int) int { return min(max(l, -maxGridTracks), maxGridTracks) }

// lineSpan is a pair of origin-zero lines.
type lineSpan struct{ start, end int }

// span returns the number of tracks between the lines.
func (l lineSpan) span() int { return max(l.end-l.start, 0) }

// trackCounts holds the number of tracks of an axis before, in and after
// the explicit grid.
type trackCounts struct{ neg, explicit, pos int }

func (c trackCounts) len() int           { return c.neg + c.explicit + c.pos }
func (c trackCounts) implicitStart() int { return -c.neg }
func (c trackCounts) implicitEnd() int   { return c.explicit + c.pos }

// ozToTrack converts an origin-zero line to the index of the track that
// follows it in an occupancy list (0 is the first track of the implicit
// grid).
func (c trackCounts) ozToTrack(l int) int { return l + c.neg }

// trackToOZ converts a track index of an occupancy list to the line before
// it.
func (c trackCounts) trackToOZ(i int) int { return i - c.neg }

// vecIndex converts an origin-zero line to its index in a track vector
// (lines at even indexes, tracks at odd ones), if the line exists.
func (c trackCounts) vecIndex(l int) (int, bool) {
	if l < -c.neg || l > c.explicit+c.pos {
		return 0, false
	}
	return 2 * (l + c.neg), true
}

// impliedNeg is the number of negative implicit tracks a start at l
// implies; impliedPos the number of positive implicit tracks an end at l
// implies.
func impliedNeg(l int) int { return max(-l, 0) }
func impliedPos(l, explicit int) int {
	return max(l-explicit, 0)
}

// tfKind classifies a track sizing function.
type tfKind uint8

const (
	tfAuto tfKind = iota
	tfFixed
	tfMinContent
	tfMaxContent
	tfFitContent
	tfFr
)

// trackFn is a resolved min or max track sizing function.
type trackFn struct {
	kind tfKind
	// px and pct are the value of a fixed size or a fit-content()
	// argument: px + pct × basis. hasPct says it needs a basis.
	px, pct float32
	hasPct  bool
	fr      float32
}

var zeroTrackFn = trackFn{kind: tfFixed}

// lengthFn returns a fixed trackFn from a length-percentage Dim (gaps;
// auto and keywords are 0).
func lengthFn(d Dim) trackFn {
	switch d.Unit {
	case UnitPx:
		return trackFn{kind: tfFixed, px: d.Value}
	case UnitPercent:
		return trackFn{kind: tfFixed, pct: d.Value, hasPct: true}
	case UnitCalc:
		return trackFn{kind: tfFixed, px: d.Px, pct: d.Value, hasPct: true}
	}
	return zeroTrackFn
}

// minFn returns the min track sizing function of a Dim.
func minFn(d Dim) trackFn {
	switch d.Unit {
	case UnitPx, UnitPercent, UnitCalc:
		return lengthFn(d)
	case UnitMinContent:
		return trackFn{kind: tfMinContent}
	case UnitMaxContent:
		return trackFn{kind: tfMaxContent}
	}
	return trackFn{kind: tfAuto}
}

// maxFn returns the max track sizing function of a Dim.
func maxFn(d Dim) trackFn {
	switch d.Unit {
	case UnitFr:
		return trackFn{kind: tfFr, fr: d.Value}
	case UnitFitContentPx:
		return trackFn{kind: tfFitContent, px: d.Value}
	case UnitFitContentPercent:
		return trackFn{kind: tfFitContent, px: d.Px, pct: d.Value, hasPct: true}
	}
	return minFn(d)
}

// value returns px + pct × basis, if it resolves.
func (f trackFn) value(basis opt) opt {
	if !f.hasPct {
		return some(f.px)
	}
	if !basis.ok {
		return none
	}
	return some(float32(basis.v*f.pct) + f.px)
}

// definite returns the size of a fixed function (Taffy's definite_value).
func (f trackFn) definite(basis opt) opt {
	if f.kind != tfFixed {
		return none
	}
	return f.value(basis)
}

// definiteLimit returns the size of a fixed function or the argument of
// fit-content() (Taffy's definite_limit).
func (f trackFn) definiteLimit(basis opt) opt {
	if f.kind == tfFitContent {
		return f.value(basis)
	}
	return f.definite(basis)
}

// hasDefinite reports whether definite resolves.
func (f trackFn) hasDefinite(basis opt) bool { return f.definite(basis).ok }

// percentOf resolves a fixed function with a percentage against basis
// (Taffy's resolved_percentage_size); others give none.
func (f trackFn) percentOf(basis float32) opt {
	if f.kind != tfFixed || !f.hasPct {
		return none
	}
	return f.value(some(basis))
}

func (f trackFn) usesPercentage() bool {
	return f.hasPct && (f.kind == tfFixed || f.kind == tfFitContent)
}

// isIntrinsic reports auto, min-content, max-content and fit-content().
func (f trackFn) isIntrinsic() bool {
	return f.kind == tfAuto || f.kind == tfMinContent || f.kind == tfMaxContent || f.kind == tfFitContent
}

// isMaxContentAlike reports auto, max-content and fit-content(), which
// CSS Grid's algorithm treats as max-content.
func (f trackFn) isMaxContentAlike() bool {
	return f.kind == tfAuto || f.kind == tfMaxContent || f.kind == tfFitContent
}

func (f trackFn) isMinOrMaxContent() bool { return f.kind == tfMinContent || f.kind == tfMaxContent }

// gridTrack is a track or a gutter (gutters are sized like fixed tracks)
// during layout.
type gridTrack struct {
	gutter    bool
	collapsed bool
	min, max  trackFn

	offset float32 // from the container's border box
	base   float32 // base size
	limit  float32 // growth limit (may be +Inf)

	// alignAdj is added to the track's size estimate when items in the
	// other axis are measured (content alignment).
	alignAdj float32

	incurred, basePlanned, limitPlanned float32
	infGrowable                         bool
	limitSet                            bool // limitPlanned holds a span-1 item's contribution
}

func newTrack(t TrackSize) gridTrack {
	return gridTrack{min: minFn(t.Min), max: maxFn(t.Max)}
}

func newGutter(gap Dim) gridTrack {
	f := lengthFn(gap)
	return gridTrack{gutter: true, min: f, max: f}
}

// collapse makes the track a collapsed zero-sized track.
func (t *gridTrack) collapse() {
	t.collapsed = true
	t.min, t.max = zeroTrackFn, zeroTrackFn
}

func (t *gridTrack) isFlexible() bool     { return t.max.kind == tfFr }
func (t *gridTrack) usesPercentage() bool { return t.min.usesPercentage() || t.max.usesPercentage() }
func (t *gridTrack) hasIntrinsic() bool   { return t.min.isIntrinsic() || t.max.isIntrinsic() }

// fitContentLimit returns the fit-content() argument of the track's max
// sizing function (+Inf for other functions, or an unresolvable
// percentage).
func (t *gridTrack) fitContentLimit(space opt) float32 {
	if t.max.kind != tfFitContent {
		return inf
	}
	return t.max.value(space).unwrapOr(inf)
}

func (t *gridTrack) fitContentLimitedGrowthLimit(space opt) float32 {
	return fmin(t.limit, t.fitContentLimit(space))
}

func (t *gridTrack) flexFactor() float32 {
	if t.max.kind == tfFr {
		return t.max.fr
	}
	return 0
}

// trackSizeOf returns a TrackSize for implicit tracks (auto when absent).
var autoTrack = TrackSize{Min: Auto, Max: Auto}

// hasFixedComponent reports whether min or max is a length-percentage.
func hasFixedComponent(t TrackSize) bool {
	return minFn(t.Min).kind == tfFixed || maxFn(t.Max).kind == tfFixed
}

// trackDefiniteValue treats a track as its max sizing function if definite,
// else its min, the max floored by the min when both are definite (used to
// count auto repetitions).
func trackDefiniteValue(t TrackSize, basis opt) float32 {
	mx := maxFn(t.Max).definite(basis)
	mn := minFn(t.Min).definite(basis)
	if mx.ok {
		return mx.maybeMax(mn).v
	}
	return mn.or0()
}

// repeatCount returns the repetition count of a RepeatCount entry.
func (e *TrackListEntry) repeatCount() int { return max(e.Count, 1) }

func (e *TrackListEntry) isAutoRepeat() bool {
	return e.Repeat == RepeatAutoFill || e.Repeat == RepeatAutoFit
}

// explicitGridSize computes the number of auto repetitions and of explicit
// tracks of a template in one axis (Taffy's
// compute_explicit_grid_size_in_axis). size is the container's inner size
// (or its max or min size) when definite; maxStrategy says whether it is a
// size or max size (the most repetitions that do not overflow) rather than
// a min size (the fewest that do).
func explicitGridSize(l *TrackList, gap Dim, size opt, maxStrategy bool) (autoReps, count int) {
	entries := l.Entries
	if len(entries) == 0 {
		return 0, 0
	}
	// A repeat without tracks makes the template invalid.
	for i := range entries {
		if entries[i].Repeat != RepeatNone && len(entries[i].Tracks) == 0 {
			return 0, 0
		}
		if entries[i].Repeat == RepeatNone && len(entries[i].Tracks) != 1 {
			return 0, 0
		}
	}
	nonAuto := 0
	autoCount := 0
	allFixed := true
	for i := range entries {
		e := &entries[i]
		switch e.Repeat {
		case RepeatNone:
			nonAuto = min(nonAuto+1, maxGridTracks)
		case RepeatCount:
			nonAuto = min(nonAuto+min(e.repeatCount(), maxGridTracks)*len(e.Tracks), maxGridTracks)
		default:
			autoCount++
		}
		for _, t := range e.Tracks {
			allFixed = allFixed && hasFixedComponent(t)
		}
	}
	// More than one auto repetition, or one next to tracks without a fixed
	// component, invalidates the template.
	if autoCount > 1 || autoCount == 1 && !allFixed {
		return 0, 0
	}
	if autoCount == 0 {
		return 0, nonAuto
	}

	insertion := 0
	var rep *TrackListEntry
	for i := range entries {
		e := &entries[i]
		if e.isAutoRepeat() {
			rep = e
			break
		}
		if e.Repeat == RepeatNone {
			insertion++
		} else {
			insertion += min(e.repeatCount(), maxGridTracks) * len(e.Tracks)
		}
	}
	repTracks := len(rep.Tracks)

	reps := 1
	if size.ok {
		inner := size.v
		var nonRepeating float32
		for i := range entries {
			e := &entries[i]
			switch e.Repeat {
			case RepeatNone:
				nonRepeating += trackDefiniteValue(e.Tracks[0], size)
			case RepeatCount:
				var sum float32
				for _, t := range e.Tracks {
					sum += trackDefiniteValue(t, size)
				}
				nonRepeating += float32(sum * float32(e.repeatCount()))
			}
		}
		gapSize := gap.resolveOrZero(size)
		var perRep float32
		for _, t := range rep.Tracks {
			perRep += trackDefiniteValue(t, size)
		}
		first := nonRepeating + perRep + float32(float32(max(nonAuto+repTracks-1, 0))*gapSize)
		if first <= inner {
			perRepUsed := perRep + float32(float32(repTracks)*gapSize)
			fit := (inner - first) / perRepUsed
			// Saturating conversions: an infinite count (zero-sized
			// tracks) is clamped below.
			var n float64
			if maxStrategy {
				n = floor64(fit)
			} else {
				n = ceil64(fit)
			}
			if n != n || n < 0 {
				n = 0
			}
			reps = int(min(n, maxGridTracks)) + 1
		}
	}

	// Keep the tracks before the repetition; truncate those after it.
	remaining := max(maxGridTracks-insertion, 0)
	if remaining == 0 {
		reps = 0
	} else {
		maxReps := (remaining + repTracks - 1) / repTracks
		reps = min(max(reps, 1), maxReps)
	}
	return reps, min(nonAuto+repTracks*reps, maxGridTracks)
}

// initTracks resolves the sizing functions of the explicit and implicit
// tracks and the gutters of one axis (Taffy's initialize_grid_tracks).
// occupied reports whether a track (an occupancy index) holds items; empty
// auto-fit tracks collapse.
func initTracks(tracks []gridTrack, counts trackCounts, l *TrackList, auto []TrackSize, gap Dim, autoReps int, occupied func(int) bool) []gridTrack {
	tracks = append(tracks[:0], newGutter(gap))

	if counts.neg > 0 {
		if len(auto) == 0 {
			for range counts.neg {
				tracks = append(tracks, newTrack(autoTrack), newGutter(gap))
			}
		} else {
			// The negative implicit tracks cycle the auto tracks backwards
			// from the explicit grid.
			off := len(auto) - counts.neg%len(auto)
			for i := range counts.neg {
				tracks = append(tracks, newTrack(auto[(off+i)%len(auto)]), newGutter(gap))
			}
		}
	}

	cur := counts.neg
	limit := counts.neg + counts.explicit
	if counts.explicit > 0 {
		for i := range l.Entries {
			e := &l.Entries[i]
			switch e.Repeat {
			case RepeatNone:
				if cur < limit {
					tracks = append(tracks, newTrack(e.Tracks[0]), newGutter(gap))
					cur++
				}
			case RepeatCount:
				n := min(len(e.Tracks)*min(e.repeatCount(), maxGridTracks), limit-cur)
				for k := range n {
					tracks = append(tracks, newTrack(e.Tracks[k%len(e.Tracks)]), newGutter(gap))
					cur++
				}
			default:
				n := min(len(e.Tracks)*autoReps, limit-cur)
				for k := range n {
					t, g := newTrack(e.Tracks[k%len(e.Tracks)]), newGutter(gap)
					// Empty auto-fit tracks collapse, with the gutter after
					// them.
					if e.Repeat == RepeatAutoFit && !occupied(cur) {
						t.collapse()
						g.collapse()
					}
					tracks = append(tracks, t, g)
					cur++
				}
				// An auto-fit repetition at the end of the track list has
				// no track after it: collapse the gutters back to the last
				// track that is not collapsed.
				if e.Repeat == RepeatAutoFit && cur == counts.len() {
					for j := len(tracks) - 1; j >= 0; j-- {
						if !tracks[j].gutter && !tracks[j].collapsed {
							break
						}
						tracks[j].collapse()
					}
				}
			}
		}
	}

	// Positive implicit tracks, plus explicit tracks that the template did
	// not create (an invalid template, or grid areas larger than it).
	n := counts.pos + counts.neg + counts.explicit - cur
	for i := range n {
		t := autoTrack
		if len(auto) > 0 {
			t = auto[i%len(auto)]
		}
		tracks = append(tracks, newTrack(t), newGutter(gap))
	}

	// The outer lines take no space.
	tracks[0].collapse()
	tracks[len(tracks)-1].collapse()
	return tracks
}

// ozPlacementKind is the kind of an ozPlacement.
type ozPlacementKind uint8

const (
	ozAuto ozPlacementKind = iota
	ozLine
	ozSpan
)

// ozPlacement is one end of a placement with lines resolved to origin-zero
// lines.
type ozPlacement struct {
	kind ozPlacementKind
	v    int // the line, or the span (at least 1)
}

// ozPair is a placement in one axis.
type ozPair struct{ start, end ozPlacement }

func (p ozPair) isDefinite() bool { return p.start.kind == ozLine || p.end.kind == ozLine }

// indefiniteSpan returns the span of a placement without a line.
func (p ozPair) indefiniteSpan() int {
	switch {
	case p.start.kind == ozSpan:
		return p.start.v
	case p.end.kind == ozSpan:
		return p.end.v
	}
	return 1
}

// resolveDefinite resolves a definite placement to its lines (CSS Grid 1
// section 8.3.1: swapped lines are swapped back, equal lines span 1).
func (p ozPair) resolveDefinite() lineSpan {
	s, e := p.start, p.end
	switch {
	case s.kind == ozLine && e.kind == ozLine:
		if s.v == e.v {
			return lineSpan{s.v, s.v + 1}
		}
		return lineSpan{min(s.v, e.v), max(s.v, e.v)}
	case s.kind == ozLine && e.kind == ozSpan:
		return lineSpan{s.v, s.v + e.v}
	case s.kind == ozLine:
		return lineSpan{s.v, s.v + 1}
	case s.kind == ozSpan:
		return lineSpan{e.v - s.v, e.v}
	default:
		return lineSpan{e.v - 1, e.v}
	}
}

// resolveAbsolute resolves the placement of an absolutely positioned item:
// lines where known, ok false where the edge is the container's padding
// edge.
func (p ozPair) resolveAbsolute() (start, end int, startOK, endOK bool) {
	s, e := p.start, p.end
	switch {
	case s.kind == ozLine && e.kind == ozLine:
		if s.v == e.v {
			return s.v, s.v + 1, true, true
		}
		return min(s.v, e.v), max(s.v, e.v), true, true
	case s.kind == ozLine && e.kind == ozSpan:
		return s.v, s.v + e.v, true, true
	case s.kind == ozLine:
		return s.v, 0, true, false
	case s.kind == ozSpan && e.kind == ozLine:
		return e.v - s.v, e.v, true, true
	case s.kind == ozAuto && e.kind == ozLine:
		return 0, e.v, false, true
	}
	return 0, 0, false, false
}

// cssPlacement converts a non-named grid line to an ozPlacement: line 0 is
// auto, spans are clamped to [1, maxGridTracks].
func cssPlacement(l GridLine, explicit int) ozPlacement {
	switch l.Kind {
	case GridLineNumber:
		if l.N == 0 {
			return ozPlacement{}
		}
		return ozPlacement{ozLine, ozFromCSS(l.N, explicit)}
	case GridSpan:
		return ozPlacement{ozSpan, min(max(l.N, 1), maxGridTracks)}
	}
	return ozPlacement{}
}

// placementIgnoringNamed converts a placement with named lines as auto
// (for the grid size estimate).
func placementIgnoringNamed(p GridPlacement, explicit int) ozPair {
	return ozPair{cssPlacement(p.Start, explicit), cssPlacement(p.End, explicit)}
}

// namedLines resolves named lines in one axis: the origin-zero positions of
// each line name of the explicit grid, sorted.
type namedLines struct {
	lines    map[string][]int
	explicit int
}

// nameLookup returns the lines named name.
func (nl *namedLines) get(name string) ([]int, bool) {
	ls, ok := nl.lines[name]
	return ls, ok && len(ls) > 0
}

// endward returns the nth line named name after line from (from excluded;
// all lines if from is nil), counting the implicit lines after the
// explicit grid as named when there are not enough.
func (nl *namedLines) endward(lines []int, from *int, n int) int {
	i := 0
	if from != nil {
		i = sort.SearchInts(lines, *from+1)
	}
	if rest := lines[i:]; n <= len(rest) {
		return rest[n-1]
	} else {
		n -= len(rest)
	}
	first := nl.explicit + 1
	if from != nil && *from >= first {
		first = *from + 1
	}
	return first + n - 1
}

// startward returns the nth line named name before line from (from
// excluded; all lines if from is nil), counting the implicit lines before
// the explicit grid as named when there are not enough.
func (nl *namedLines) startward(lines []int, from *int, n int) int {
	j := len(lines)
	if from != nil {
		j = sort.SearchInts(lines, *from)
	}
	if rest := lines[:j]; n <= len(rest) {
		return rest[len(rest)-n]
	} else {
		n -= len(rest)
	}
	first := -1
	if from != nil && *from <= 0 {
		first = *from - 1
	}
	return first - (n - 1)
}

// line resolves a named line (GridNamedLine) to an origin-zero line. start
// says whether it is the start of the placement.
func (nl *namedLines) line(l GridLine, start bool) int {
	n := l.N
	if n == 0 {
		// A lone identifier: <name>-start or <name>-end first.
		suffix := "-end"
		if start {
			suffix = "-start"
		}
		if ls, ok := nl.get(l.Name + suffix); ok {
			return ls[0]
		}
		n = 1
	}
	ls, _ := nl.get(l.Name)
	if n > 0 {
		return clampOZ(nl.endward(ls, nil, min(n, 1<<20)))
	}
	return clampOZ(nl.startward(ls, nil, min(-n, 1<<20)))
}

// resolve resolves a placement with named lines to origin-zero lines.
func (nl *namedLines) resolve(p GridPlacement) ozPair {
	var out ozPair
	conv := func(l GridLine, start bool) ozPlacement {
		switch l.Kind {
		case GridNamedLine:
			return ozPlacement{ozLine, nl.line(l, start)}
		case GridNamedSpan:
			return ozPlacement{ozSpan, 1} // resolved below against a line
		}
		return cssPlacement(l, nl.explicit)
	}
	out.start, out.end = conv(p.Start, true), conv(p.End, false)
	n := func(l GridLine) int { return min(max(l.N, 1), 1<<20) }
	switch {
	case out.start.kind == ozLine && p.End.Kind == GridNamedSpan:
		ls, _ := nl.get(p.End.Name)
		from := out.start.v
		out.end = ozPlacement{ozLine, clampOZ(nl.endward(ls, &from, n(p.End)))}
	case out.end.kind == ozLine && p.Start.Kind == GridNamedSpan:
		ls, _ := nl.get(p.Start.Name)
		from := out.end.v
		out.start = ozPlacement{ozLine, clampOZ(nl.startward(ls, &from, n(p.Start)))}
	}
	return out
}

// namedResolver holds the line names of both axes (Taffy's
// NamedLineResolver).
type namedResolver struct {
	cols, rows namedLines
}

// newNamedResolver collects the line names of a template, with repeat()
// names expanded by the auto repetition counts, and the <area>-start and
// <area>-end lines of the named areas.
func newNamedResolver(g *GridTemplate, colReps, rowReps int) namedResolver {
	var r namedResolver
	r.cols.lines = map[string][]int{}
	r.rows.lines = map[string][]int{}
	if g == nil {
		return r
	}
	collectNames(r.cols.lines, &g.Columns, colReps)
	collectNames(r.rows.lines, &g.Rows, rowReps)
	for _, a := range g.Areas {
		r.cols.lines[a.Name+"-start"] = append(r.cols.lines[a.Name+"-start"], a.ColumnStart-1)
		r.cols.lines[a.Name+"-end"] = append(r.cols.lines[a.Name+"-end"], a.ColumnEnd-1)
		r.rows.lines[a.Name+"-start"] = append(r.rows.lines[a.Name+"-start"], a.RowStart-1)
		r.rows.lines[a.Name+"-end"] = append(r.rows.lines[a.Name+"-end"], a.RowEnd-1)
	}
	for _, m := range []map[string][]int{r.cols.lines, r.rows.lines} {
		for k, ls := range m {
			slices.Sort(ls)
			m[k] = slices.Compact(ls)
		}
	}
	return r
}

// collectNames records the origin-zero line of every name of a track list.
func collectNames(m map[string][]int, l *TrackList, autoReps int) {
	if len(l.LineNames) == 0 && !hasRepeatNames(l) {
		return
	}
	line := 0 // origin-zero
	for i := 0; i <= len(l.Entries); i++ {
		if i < len(l.LineNames) {
			for _, name := range l.LineNames[i] {
				m[name] = append(m[name], line)
			}
		}
		if i == len(l.Entries) {
			break
		}
		e := &l.Entries[i]
		if e.Repeat == RepeatNone {
			line++
			continue
		}
		reps := autoReps
		if e.Repeat == RepeatCount {
			reps = min(e.repeatCount(), maxGridTracks)
		}
		names := e.LineNames
		if len(names) != 0 && len(names) != len(e.Tracks)+1 {
			names = nil // malformed: the repetition's lines are unnamed
		}
		for range reps {
			for k, set := range names {
				for _, name := range set {
					m[name] = append(m[name], line+k)
				}
			}
			line += len(e.Tracks)
			// Names beyond the track limit can never resolve.
			if line > maxGridTracks {
				break
			}
		}
	}
}

func hasRepeatNames(l *TrackList) bool {
	for i := range l.Entries {
		if len(l.Entries[i].LineNames) > 0 {
			return true
		}
	}
	return false
}

// gridSizeEstimate estimates the track counts of the implicit grid from
// the explicit counts and the items' placements (Taffy's
// compute_grid_size_estimate): exact before the explicit grid and in it, a
// lower bound after it (auto-placement can add tracks).
func gridSizeEstimate(explicitCols, explicitRows int, items []gridChild) (trackCounts, trackCounts) {
	var colMin, colMax, colSpan, rowMin, rowMax, rowSpan int
	for i := range items {
		cmin, cmax, cspan := childLineRange(placementIgnoringNamed(items[i].st.GridColumn, explicitCols))
		rmin, rmax, rspan := childLineRange(placementIgnoringNamed(items[i].st.GridRow, explicitRows))
		colMin, colMax, colSpan = min(colMin, cmin), max(colMax, cmax), max(colSpan, cspan)
		rowMin, rowMax, rowSpan = min(rowMin, rmin), max(rowMax, rmax), max(rowSpan, rspan)
	}
	cols := trackCounts{impliedNeg(colMin), explicitCols, impliedPos(colMax, explicitCols)}
	rows := trackCounts{impliedNeg(rowMin), explicitRows, impliedPos(rowMax, explicitRows)}
	// An item whose span does not fit in the estimate adds tracks at the
	// end.
	if cols.len() < colSpan {
		cols.pos = colSpan - cols.explicit - cols.neg
	}
	if rows.len() < rowSpan {
		rows.pos = rowSpan - rows.explicit - rows.neg
	}
	return cols, rows
}

// childLineRange returns a conservative estimate of the smallest and
// largest lines an item uses in one axis, and its span when it has no
// line.
func childLineRange(p ozPair) (lo, hi, span int) {
	s, e := p.start, p.end
	switch {
	case s.kind == ozLine && e.kind == ozLine:
		if s.v == e.v {
			lo, hi = s.v, s.v+1
		} else {
			lo, hi = min(s.v, e.v), max(s.v, e.v)
		}
	case s.kind == ozLine && e.kind == ozSpan:
		lo, hi = s.v, s.v+e.v
	case s.kind == ozLine:
		lo, hi = s.v, s.v+1
	case e.kind == ozLine && s.kind == ozSpan:
		lo, hi = e.v-s.v, e.v
	case e.kind == ozLine:
		lo, hi = e.v, e.v
	}
	span = 1
	if s.kind != ozLine && e.kind != ozLine {
		span = p.indefiniteSpan()
	}
	return max(lo, -maxGridTracks), min(hi, maxGridTracks), span
}
