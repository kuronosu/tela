package layout

import "slices"

// This file defines the grid part of the layout style (ARCHITECTURE
// section 9): track lists with line names and repeat(), named areas, the
// auto-placement flow and item placements. The algorithm is in grid.go.

// TrackSize is the size of one grid track: minmax(Min, Max).
//
// Min is a length, a percentage or calc() (UnitPx, UnitPercent,
// UnitCalc), auto (UnitInitial or UnitAuto), min-content or max-content;
// any other unit (fr, fit-content) is auto. Max takes the same values plus
// a flexible length (UnitFr) and fit-content(<length-percentage>)
// (UnitFitContentPx, UnitFitContentPercent); other units are auto.
//
// CSS shorthands map as: 100px is minmax(100px, 100px), 1fr is
// minmax(auto, 1fr), fit-content(10px) is minmax(auto, fit-content(10px)),
// auto is minmax(auto, auto).
type TrackSize struct{ Min, Max Dim }

// Track returns the track size of a single breadth: TrackSize{d, d}, or
// minmax(auto, d) for a flexible length or fit-content().
func Track(d Dim) TrackSize {
	switch d.Unit {
	case UnitFr, UnitFitContentPx, UnitFitContentPercent:
		return TrackSize{Min: Auto, Max: d}
	}
	return TrackSize{Min: d, Max: d}
}

// Minmax returns minmax(min, max).
func Minmax(min, max Dim) TrackSize { return TrackSize{Min: min, Max: max} }

// RepeatKind says how a TrackListEntry repeats.
type RepeatKind uint8

const (
	// RepeatNone is a single track (the zero value): Tracks has one size.
	RepeatNone RepeatKind = iota
	// RepeatCount repeats Tracks Count times.
	RepeatCount
	// RepeatAutoFill repeats Tracks as many times as fit the container
	// (at least once), keeping empty tracks.
	RepeatAutoFill
	// RepeatAutoFit is RepeatAutoFill with the empty repeated tracks
	// collapsed.
	RepeatAutoFit
)

// TrackListEntry is one component of a track list: a single track or a
// repeat().
type TrackListEntry struct {
	Repeat RepeatKind
	// Count is the repetition count of RepeatCount (values below 1 are 1).
	Count  int
	Tracks []TrackSize
	// LineNames are the line names inside a repeat(): empty, or one set
	// per line of one repetition (len(Tracks)+1 sets: before the first
	// track, between tracks and after the last). The last set of a
	// repetition and the first of the next name the same line.
	LineNames [][]string
}

// Single returns a track list entry of one track.
func Single(t TrackSize) TrackListEntry { return TrackListEntry{Tracks: []TrackSize{t}} }

// Repeat returns repeat(count, tracks...).
func Repeat(count int, tracks ...TrackSize) TrackListEntry {
	return TrackListEntry{Repeat: RepeatCount, Count: count, Tracks: tracks}
}

// TrackList is grid-template-columns or grid-template-rows. The zero value
// is none (no explicit tracks).
//
// A template with more than one auto-fill or auto-fit repeat, or an
// auto repeat next to tracks without a fixed size (a length or a
// percentage in min or max), is invalid and behaves as none, as in CSS.
type TrackList struct {
	Entries []TrackListEntry
	// LineNames are the names of the lines between the entries: empty, or
	// len(Entries)+1 sets (LineNames[i] precedes Entries[i]). Names of
	// lines inside a repeat() are in the entry.
	LineNames [][]string
}

// Tracks returns a track list of single tracks without line names.
func Tracks(sizes ...TrackSize) TrackList {
	l := TrackList{Entries: make([]TrackListEntry, len(sizes))}
	for i, s := range sizes {
		l.Entries[i] = Single(s)
	}
	return l
}

// GridArea is a named area of grid-template-areas: the lines that bound it
// (CSS line numbers, from 1: an area in the first row and column has
// RowStart 1 and RowEnd 2).
type GridArea struct {
	Name                   string
	RowStart, RowEnd       int
	ColumnStart, ColumnEnd int
}

// GridTemplate holds the track lists, line names and areas of a grid
// container.
type GridTemplate struct {
	// Columns and Rows are grid-template-columns and grid-template-rows.
	Columns, Rows TrackList
	// AutoColumns and AutoRows are grid-auto-columns and grid-auto-rows:
	// the sizes of implicit tracks, cycled (empty is auto).
	AutoColumns, AutoRows []TrackSize
	// Areas are the named areas of grid-template-areas, and AreaRows and
	// AreaColumns the size of its template (which may be larger than the
	// named areas because of unnamed cells). Each area defines the line
	// names <name>-start and <name>-end in both axes.
	Areas                 []GridArea
	AreaRows, AreaColumns int
}

// Equal reports whether two templates are the same (nil equals only nil).
func (g *GridTemplate) Equal(o *GridTemplate) bool {
	if g == nil || o == nil {
		return g == o
	}
	return g.Columns.equal(&o.Columns) && g.Rows.equal(&o.Rows) &&
		slices.Equal(g.AutoColumns, o.AutoColumns) && slices.Equal(g.AutoRows, o.AutoRows) &&
		slices.Equal(g.Areas, o.Areas) && g.AreaRows == o.AreaRows && g.AreaColumns == o.AreaColumns
}

func (l *TrackList) equal(o *TrackList) bool {
	return slices.EqualFunc(l.Entries, o.Entries, func(a, b TrackListEntry) bool {
		return a.Repeat == b.Repeat && a.Count == b.Count && slices.Equal(a.Tracks, b.Tracks) && equalNames(a.LineNames, b.LineNames)
	}) && equalNames(l.LineNames, o.LineNames)
}

func equalNames(a, b [][]string) bool {
	return slices.EqualFunc(a, b, func(x, y []string) bool { return slices.Equal(x, y) })
}

// GridAutoFlow is the CSS grid-auto-flow property.
type GridAutoFlow uint8

const (
	// GridFlowRow fills rows in turn, adding rows as needed (the initial
	// value).
	GridFlowRow GridAutoFlow = iota
	// GridFlowColumn fills columns in turn, adding columns as needed.
	GridFlowColumn
	// GridFlowRowDense is row with the dense packing algorithm, which
	// fills earlier holes with later items.
	GridFlowRowDense
	// GridFlowColumnDense is column with dense packing.
	GridFlowColumnDense
)

func (f GridAutoFlow) isDense() bool { return f == GridFlowRowDense || f == GridFlowColumnDense }

// primaryAxis returns the axis the auto-placement cursor moves along:
// horizontal (along a row) for the row flows.
func (f GridAutoFlow) primaryAxis() axis {
	if f == GridFlowColumn || f == GridFlowColumnDense {
		return vertical
	}
	return horizontal
}

// GridLineKind selects the meaning of a GridLine.
type GridLineKind uint8

const (
	// GridAuto is auto (the zero value): the item is placed by the
	// auto-placement algorithm in this axis.
	GridAuto GridLineKind = iota
	// GridLineNumber is line N: positive counts from the start of the
	// explicit grid (1 is the first line), negative from its end (-1 is
	// the last line). Line 0 is auto.
	GridLineNumber
	// GridSpan spans N tracks (N < 1 is 1).
	GridSpan
	// GridNamedLine is the Nth line named Name (N < 0 counts from the
	// end; N == 0 is 1). When no line has that name, Name-start or
	// Name-end (for the start and the end of the placement) is tried, then
	// the lines after the explicit grid count as named, as in CSS.
	GridNamedLine
	// GridNamedSpan spans until the Nth line named Name (N < 1 is 1).
	GridNamedSpan
)

// GridLine is one end of a grid placement (grid-row-start and the like).
type GridLine struct {
	Kind GridLineKind
	N    int
	Name string
}

// GridLineAt returns line n (GridLineNumber).
func GridLineAt(n int) GridLine { return GridLine{Kind: GridLineNumber, N: n} }

// GridSpanOf returns span n.
func GridSpanOf(n int) GridLine { return GridLine{Kind: GridSpan, N: n} }

// GridPlacement is grid-row or grid-column: the start and end lines of an
// item in one axis. The zero value is auto / auto.
type GridPlacement struct{ Start, End GridLine }
