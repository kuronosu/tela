package style

import (
	"errors"
	"fmt"
	"strings"
)

// This file holds the grid style types and the parser of CSS track lists.
// The grid layout algorithm arrives in phase 6; the classes parse and
// survive now.

// BreadthKind selects the meaning of a TrackBreadth.
type BreadthKind uint8

const (
	// BreadthAuto is the keyword auto. It is the zero value.
	BreadthAuto BreadthKind = iota
	// BreadthLength is a length-percentage in LP.
	BreadthLength
	// BreadthFr is a flexible length: Fr fractions of the free space.
	BreadthFr
	BreadthMinContent
	BreadthMaxContent
	// BreadthFitContent is fit-content(LP).
	BreadthFitContent
)

// TrackBreadth is a minimum or maximum track size.
type TrackBreadth struct {
	Kind BreadthKind
	LP   LP
	Fr   float32
}

// TrackSize is the size of one track, minmax(Min, Max). A fixed length is
// minmax(v, v), a flexible length is minmax(auto, Nfr) and the Tailwind
// grid-cols-N tracks are minmax(0, 1fr).
type TrackSize struct{ Min, Max TrackBreadth }

// RepeatKind selects how many times a TrackRepeat repeats.
type RepeatKind uint8

const (
	// RepeatCount repeats a fixed number of times.
	RepeatCount RepeatKind = iota
	// RepeatAutoFill repeats as often as fits, keeping empty tracks.
	RepeatAutoFill
	// RepeatAutoFit repeats as often as fits and collapses empty tracks.
	RepeatAutoFit
)

// TrackEntry is one item of a track list: a single track (Count == 0 and
// one track) or repeat(Kind, Tracks).
type TrackEntry struct {
	// Repeat is false for a plain track (Tracks has one element).
	Repeat bool
	Kind   RepeatKind
	Count  int
	Tracks []TrackSize
}

// TrackList is grid-template-columns or grid-template-rows. The zero value
// is none.
type TrackList []TrackEntry

// Expand returns the tracks of the list with fixed-count repeats unrolled.
// An auto-fill or auto-fit repeat contributes its tracks once; layout
// decides the real count.
func (l TrackList) Expand() []TrackSize {
	var out []TrackSize
	for _, e := range l {
		if e.Repeat && e.Kind == RepeatCount {
			for i := 0; i < e.Count; i++ {
				out = append(out, e.Tracks...)
			}
			continue
		}
		out = append(out, e.Tracks...)
	}
	return out
}

// GridAutoFlow is the CSS grid-auto-flow property.
type GridAutoFlow uint8

const (
	GridFlowRow GridAutoFlow = iota
	GridFlowColumn
	GridFlowRowDense
	GridFlowColumnDense
)

// GridLineKind selects the meaning of a GridLine.
type GridLineKind uint8

const (
	// GridLineAuto lets the auto-placement algorithm choose. It is the zero value.
	GridLineAuto GridLineKind = iota
	// GridLineIndex is a line number: positive counts from the start, negative from the end.
	GridLineIndex
	// GridLineSpan spans N tracks.
	GridLineSpan
)

// GridLine is the start or end of a grid-row or grid-column placement.
type GridLine struct {
	Kind GridLineKind
	N    int
}

// GridPlacement is grid-column or grid-row.
type GridPlacement struct{ Start, End GridLine }

// parseTrackList parses a CSS track list such as "200px 1fr",
// "repeat(3, minmax(0, 1fr))" or "repeat(auto-fill, minmax(200px, 1fr))".
// Line names are not supported.
func parseTrackList(s string, root float32) (TrackList, error) {
	s = strings.TrimSpace(s)
	if s == "none" {
		return nil, nil
	}
	var out TrackList
	for _, f := range fieldsTop(s) {
		if strings.HasPrefix(f, "[") {
			return nil, errors.New("grid line names are not supported; place items with col-start-N, col-end-N or col-span-N")
		}
		if strings.HasPrefix(f, "repeat(") && strings.HasSuffix(f, ")") {
			parts := splitTop(f[len("repeat("):len(f)-1], ',')
			if len(parts) != 2 {
				return nil, fmt.Errorf("repeat() takes a count and a track list: repeat(3, 1fr), got %q", f)
			}
			e := TrackEntry{Repeat: true}
			switch c := strings.TrimSpace(parts[0]); c {
			case "auto-fill":
				e.Kind = RepeatAutoFill
			case "auto-fit":
				e.Kind = RepeatAutoFit
			default:
				n, ok := parseUint(c)
				if !ok || n < 1 || n > 1000 {
					return nil, fmt.Errorf("repeat() count %q must be an integer from 1 to 1000, auto-fill or auto-fit", c)
				}
				e.Count = n
			}
			for _, t := range fieldsTop(strings.TrimSpace(parts[1])) {
				ts, err := parseTrackSize(t, root)
				if err != nil {
					return nil, err
				}
				e.Tracks = append(e.Tracks, ts)
			}
			if len(e.Tracks) == 0 {
				return nil, fmt.Errorf("repeat() in %q has no tracks", f)
			}
			out = append(out, e)
			continue
		}
		ts, err := parseTrackSize(f, root)
		if err != nil {
			return nil, err
		}
		out = append(out, TrackEntry{Tracks: []TrackSize{ts}})
	}
	if len(out) == 0 {
		return nil, errors.New("the track list is empty")
	}
	return out, nil
}

func parseTrackSize(s string, root float32) (TrackSize, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "minmax(") && strings.HasSuffix(s, ")") {
		parts := splitTop(s[len("minmax("):len(s)-1], ',')
		if len(parts) != 2 {
			return TrackSize{}, fmt.Errorf("minmax() takes two arguments: minmax(200px, 1fr), got %q", s)
		}
		lo, err := parseBreadth(parts[0], root, false)
		if err != nil {
			return TrackSize{}, err
		}
		hi, err := parseBreadth(parts[1], root, true)
		if err != nil {
			return TrackSize{}, err
		}
		return TrackSize{lo, hi}, nil
	}
	b, err := parseBreadth(s, root, true)
	if err != nil {
		return TrackSize{}, err
	}
	switch b.Kind {
	case BreadthFr:
		return TrackSize{Max: b}, nil // minmax(auto, Nfr)
	case BreadthFitContent:
		return TrackSize{Max: b}, nil
	}
	return TrackSize{b, b}, nil
}

func parseBreadth(s string, root float32, allowFr bool) (TrackBreadth, error) {
	s = strings.TrimSpace(s)
	switch s {
	case "auto":
		return TrackBreadth{}, nil
	case "min-content":
		return TrackBreadth{Kind: BreadthMinContent}, nil
	case "max-content":
		return TrackBreadth{Kind: BreadthMaxContent}, nil
	}
	if strings.HasSuffix(s, "fr") {
		v, ok := parseFloat(s[:len(s)-2])
		if !ok || v < 0 {
			return TrackBreadth{}, fmt.Errorf("invalid flexible length %q", s)
		}
		if !allowFr {
			return TrackBreadth{}, fmt.Errorf("a flexible length (%s) cannot be the minimum of minmax()", s)
		}
		return TrackBreadth{Kind: BreadthFr, Fr: v}, nil
	}
	if strings.HasPrefix(s, "fit-content(") && strings.HasSuffix(s, ")") {
		e, err := parseLengthExpr(s[len("fit-content("):len(s)-1], true)
		if err != nil {
			return TrackBreadth{}, fmt.Errorf("invalid fit-content() argument in %q: %v", s, err)
		}
		lp, err := trackLP(e, root)
		if err != nil {
			return TrackBreadth{}, err
		}
		return TrackBreadth{Kind: BreadthFitContent, LP: lp}, nil
	}
	e, err := parseLengthExpr(s, true)
	if err != nil {
		if err == errNotLength {
			return TrackBreadth{}, fmt.Errorf("invalid track size %q; use a length (200px), a percentage, fr, auto, min-content, max-content, minmax() or fit-content()", s)
		}
		return TrackBreadth{}, fmt.Errorf("invalid track size %q: %v", s, err)
	}
	lp, err := trackLP(e, root)
	if err != nil {
		return TrackBreadth{}, err
	}
	return TrackBreadth{Kind: BreadthLength, LP: lp}, nil
}

// trackLP converts a length expression to an LP at parse time, with rem
// worth root pixels. Units that need the element or the viewport are not
// supported in track sizes yet.
func trackLP(e lenExpr, root float32) (LP, error) {
	if e.fn != nil || e.em != 0 || e.vw != 0 || e.vh != 0 || e.ch != 0 {
		return LP{}, errors.New("grid track sizes support px, rem and % (em, vw, vh, ch and min()/max()/clamp() are not supported yet)")
	}
	return LP{Px: e.px + float32(e.rem*root), Pct: e.pct}, nil
}

// parseGridLine parses "auto", "3", "-1" or "span 2".
func parseGridLine(s string) (GridLine, error) {
	s = strings.TrimSpace(s)
	if s == "auto" {
		return GridLine{}, nil
	}
	if rest, ok := strings.CutPrefix(s, "span "); ok {
		n, ok := parseUint(strings.TrimSpace(rest))
		if !ok || n < 1 {
			return GridLine{}, fmt.Errorf("invalid span %q; use a positive integer such as \"span 2\"", s)
		}
		return GridLine{GridLineSpan, n}, nil
	}
	if v, ok := parseFloat(s); ok && v == float32(int(v)) && v != 0 {
		return GridLine{GridLineIndex, int(v)}, nil
	}
	return GridLine{}, fmt.Errorf("invalid grid line %q; use auto, a line number (3, -1) or span N", s)
}

// parseGridPlacement parses "1 / 3", "span 2 / span 2" or a single line.
func parseGridPlacement(s string) (GridPlacement, error) {
	parts := splitTop(s, '/')
	if len(parts) > 2 {
		return GridPlacement{}, fmt.Errorf("invalid grid placement %q; use \"start / end\"", s)
	}
	start, err := parseGridLine(parts[0])
	if err != nil {
		return GridPlacement{}, err
	}
	end := GridLine{}
	if len(parts) == 2 {
		if end, err = parseGridLine(parts[1]); err != nil {
			return GridPlacement{}, err
		}
	} else if start.Kind == GridLineSpan {
		end = GridLine{}
	}
	return GridPlacement{start, end}, nil
}
