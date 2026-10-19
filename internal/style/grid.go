package style

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// This file holds the grid style types, the parser of CSS track lists and
// grid lines, and their CSS serializations (for inspect). Layout is in
// internal/layout (core maps these types onto layout's).

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

// TrackEntry is one item of a track list: a single track (Repeat false
// and one track) or repeat(Kind, Tracks).
type TrackEntry struct {
	// Repeat is false for a plain track (Tracks has one element).
	Repeat bool
	Kind   RepeatKind
	Count  int
	Tracks []TrackSize
	// LineNames are the line names inside a repeat(): none, or one set
	// per line of one repetition (len(Tracks)+1 sets).
	LineNames [][]string
}

// TrackList is grid-template-columns or grid-template-rows. The zero value
// is none.
type TrackList struct {
	Entries []TrackEntry
	// LineNames are the names of the lines between the entries: none, or
	// len(Entries)+1 sets (LineNames[i] precedes Entries[i]).
	LineNames [][]string
}

// IsNone reports whether the list is none (no explicit tracks).
func (l TrackList) IsNone() bool { return len(l.Entries) == 0 }

// Expand returns the tracks of the list with fixed-count repeats unrolled.
// An auto-fill or auto-fit repeat contributes its tracks once; layout
// decides the real count.
func (l TrackList) Expand() []TrackSize {
	var out []TrackSize
	for _, e := range l.Entries {
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

var gridFlowNames = [...]string{"row", "column", "row dense", "column dense"}

func (f GridAutoFlow) String() string { return enumName(gridFlowNames[:], int(f)) }

// GridLineKind selects the meaning of a GridLine.
type GridLineKind uint8

const (
	// GridLineAuto lets the auto-placement algorithm choose. It is the zero value.
	GridLineAuto GridLineKind = iota
	// GridLineIndex is a line number: positive counts from the start, negative from the end.
	GridLineIndex
	// GridLineSpan spans N tracks.
	GridLineSpan
	// GridLineNamed is the Nth line named Name (N == 0: a lone name,
	// which first matches the line Name-start or Name-end of a named
	// area; negative N counts from the end).
	GridLineNamed
	// GridLineNamedSpan spans to the Nth line named Name (N >= 1).
	GridLineNamedSpan
)

// GridLine is the start or end of a grid-row or grid-column placement.
type GridLine struct {
	Kind GridLineKind
	N    int
	Name string
}

// GridPlacement is grid-column or grid-row.
type GridPlacement struct{ Start, End GridLine }

// parseTrackList parses a CSS track list such as "200px 1fr",
// "[sidebar] 200px [main] 1fr", "repeat(3, minmax(0, 1fr))" or
// "repeat(auto-fill, [card] minmax(200px, 1fr))".
func parseTrackList(s string, root float32) (TrackList, error) {
	s = strings.TrimSpace(s)
	if s == "none" {
		return TrackList{}, nil
	}
	var out TrackList
	var names []string
	pending, anyNames := false, false
	for _, f := range fieldsTop(s) {
		if strings.HasPrefix(f, "[") {
			n, err := parseLineNames(f)
			if err != nil {
				return TrackList{}, err
			}
			if pending {
				return TrackList{}, fmt.Errorf("two line name lists in a row in %q; put all the names of a line in one list: [a b]", s)
			}
			names, pending, anyNames = n, true, true
			continue
		}
		out.LineNames = append(out.LineNames, names)
		names, pending = nil, false
		if strings.HasPrefix(f, "repeat(") && strings.HasSuffix(f, ")") {
			e, err := parseRepeat(f, root)
			if err != nil {
				return TrackList{}, err
			}
			out.Entries = append(out.Entries, e)
			continue
		}
		ts, err := parseTrackSize(f, root)
		if err != nil {
			return TrackList{}, err
		}
		out.Entries = append(out.Entries, TrackEntry{Tracks: []TrackSize{ts}})
	}
	if len(out.Entries) == 0 {
		return TrackList{}, errors.New("the track list is empty")
	}
	out.LineNames = append(out.LineNames, names)
	if !anyNames {
		out.LineNames = nil
	}
	autos := 0
	for _, e := range out.Entries {
		if e.Repeat && e.Kind != RepeatCount {
			autos++
		}
	}
	if autos > 1 {
		return TrackList{}, fmt.Errorf("a track list can have only one repeat(auto-fill) or repeat(auto-fit), in %q", s)
	}
	if autos == 1 {
		for _, e := range out.Entries {
			for _, t := range e.Tracks {
				if !fixedTrack(t) {
					return TrackList{}, fmt.Errorf("next to repeat(auto-fill) or repeat(auto-fit) every track needs a fixed size "+
						"(a length or a percentage in minmax()), in %q", s)
				}
			}
		}
	}
	return out, nil
}

// parseRepeat parses "repeat(count, tracks)".
func parseRepeat(f string, root float32) (TrackEntry, error) {
	parts := splitTop(f[len("repeat("):len(f)-1], ',')
	if len(parts) != 2 {
		return TrackEntry{}, fmt.Errorf("repeat() takes a count and a track list: repeat(3, 1fr), got %q", f)
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
			return TrackEntry{}, fmt.Errorf("repeat() count %q must be an integer from 1 to 1000, auto-fill or auto-fit", c)
		}
		e.Count = n
	}
	var names []string
	pending, anyNames := false, false
	for _, t := range fieldsTop(strings.TrimSpace(parts[1])) {
		if strings.HasPrefix(t, "[") {
			n, err := parseLineNames(t)
			if err != nil {
				return TrackEntry{}, err
			}
			if pending {
				return TrackEntry{}, fmt.Errorf("two line name lists in a row in %q", f)
			}
			names, pending, anyNames = n, true, true
			continue
		}
		ts, err := parseTrackSize(t, root)
		if err != nil {
			return TrackEntry{}, err
		}
		if e.Kind != RepeatCount && !fixedTrack(ts) {
			return TrackEntry{}, fmt.Errorf("repeat(%s) needs fixed track sizes (a length or a percentage in minmax()), got %q",
				strings.TrimSpace(parts[0]), t)
		}
		e.LineNames = append(e.LineNames, names)
		names, pending = nil, false
		e.Tracks = append(e.Tracks, ts)
	}
	if len(e.Tracks) == 0 {
		return TrackEntry{}, fmt.Errorf("repeat() in %q has no tracks", f)
	}
	e.LineNames = append(e.LineNames, names)
	if !anyNames {
		e.LineNames = nil
	}
	return e, nil
}

// fixedTrack reports whether a track size has a fixed component: a
// length-percentage minimum or maximum.
func fixedTrack(t TrackSize) bool { return t.Min.Kind == BreadthLength || t.Max.Kind == BreadthLength }

// parseLineNames parses "[a b]".
func parseLineNames(f string) ([]string, error) {
	if !strings.HasSuffix(f, "]") {
		return nil, fmt.Errorf("unterminated line name list %q", f)
	}
	names := strings.Fields(f[1 : len(f)-1])
	for _, n := range names {
		if !isGridIdent(n) {
			return nil, fmt.Errorf("invalid grid line name %q; use letters, digits, - and _ (not span or auto)", n)
		}
	}
	return names, nil
}

// isGridIdent reports whether s is a valid <custom-ident> for a grid line
// name: an identifier that is not span, auto or a CSS-wide keyword.
func isGridIdent(s string) bool {
	switch s {
	case "", "span", "auto", "inherit", "initial", "unset", "default", "revert":
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_', c >= 0x80:
		case c >= '0' && c <= '9':
			if i == 0 || i == 1 && s[0] == '-' {
				return false
			}
		case c == '-':
			if i == 1 && s[0] == '-' {
				return false // "--" starts a custom property
			}
		default:
			return false
		}
	}
	return s != "-"
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

// parseGridLine parses a <grid-line>: "auto", "3", "-1", "span 2", a
// line name ("main"), "main 2", "span main" or "span 2 main" (the integer
// and the name in either order).
func parseGridLine(s string) (GridLine, error) {
	s = strings.TrimSpace(s)
	if s == "auto" {
		return GridLine{}, nil
	}
	bad := func() (GridLine, error) {
		return GridLine{}, fmt.Errorf("invalid grid line %q; use auto, a line number (3, -1), a line name, span N or span name", s)
	}
	span, hasNum, num, name := false, false, 0, ""
	for _, f := range strings.Fields(s) {
		switch {
		case f == "span" && !span:
			span = true
		case name == "" && isGridIdent(f):
			// Before the number: an identifier may start with a hyphen
			// ("-side"), while "-1" is not one.
			name = f
		case !hasNum && (f[0] == '-' || f[0] == '+' || f[0] >= '0' && f[0] <= '9'):
			v, ok := parseFloat(f)
			if !ok || v != float32(int(v)) || v == 0 {
				return bad()
			}
			num, hasNum = int(v), true
		default:
			return bad()
		}
	}
	switch {
	case span && hasNum && num < 1:
		return GridLine{}, fmt.Errorf("invalid span %q; use a positive integer such as \"span 2\"", s)
	case span && name != "":
		return GridLine{GridLineNamedSpan, max(num, 1), name}, nil
	case span && hasNum:
		return GridLine{GridLineSpan, num, ""}, nil
	case span:
		return bad()
	case name != "":
		return GridLine{GridLineNamed, num, name}, nil
	case hasNum:
		return GridLine{GridLineIndex, num, ""}, nil
	}
	return bad()
}

// parseGridPlacement parses "1 / 3", "span 2 / span 2", "main" or a single
// line (with a lone name, the end is the same name, as in CSS).
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
	} else if start.Kind == GridLineNamed && start.N == 0 {
		end = start
	}
	return GridPlacement{start, end}, nil
}

// Serializations (CSS syntax), for tela inspect.

func (b TrackBreadth) String() string {
	switch b.Kind {
	case BreadthLength:
		return b.LP.String()
	case BreadthFr:
		return strconv.FormatFloat(float64(b.Fr), 'g', -1, 32) + "fr"
	case BreadthMinContent:
		return "min-content"
	case BreadthMaxContent:
		return "max-content"
	case BreadthFitContent:
		return "fit-content(" + b.LP.String() + ")"
	}
	return "auto"
}

func (t TrackSize) String() string {
	switch {
	case t.Min == t.Max:
		return t.Min.String()
	case t.Min.Kind == BreadthAuto && (t.Max.Kind == BreadthFr || t.Max.Kind == BreadthFitContent):
		return t.Max.String()
	}
	return "minmax(" + t.Min.String() + ", " + t.Max.String() + ")"
}

// nameList returns "[a b]" for set i of sets, or "".
func nameList(sets [][]string, i int) string {
	if i < len(sets) && len(sets[i]) > 0 {
		return "[" + strings.Join(sets[i], " ") + "]"
	}
	return ""
}

func (l TrackList) String() string {
	if l.IsNone() {
		return "none"
	}
	var toks []string
	add := func(t string) {
		if t != "" {
			toks = append(toks, t)
		}
	}
	for i, e := range l.Entries {
		add(nameList(l.LineNames, i))
		if !e.Repeat {
			add(e.Tracks[0].String())
			continue
		}
		var in []string
		for k, t := range e.Tracks {
			if n := nameList(e.LineNames, k); n != "" {
				in = append(in, n)
			}
			in = append(in, t.String())
		}
		if n := nameList(e.LineNames, len(e.Tracks)); n != "" {
			in = append(in, n)
		}
		count := strconv.Itoa(e.Count)
		switch e.Kind {
		case RepeatAutoFill:
			count = "auto-fill"
		case RepeatAutoFit:
			count = "auto-fit"
		}
		add("repeat(" + count + ", " + strings.Join(in, " ") + ")")
	}
	add(nameList(l.LineNames, len(l.Entries)))
	return strings.Join(toks, " ")
}

func (g GridLine) String() string {
	switch g.Kind {
	case GridLineIndex:
		return strconv.Itoa(g.N)
	case GridLineSpan:
		return "span " + strconv.Itoa(g.N)
	case GridLineNamed:
		if g.N != 0 {
			return strconv.Itoa(g.N) + " " + g.Name
		}
		return g.Name
	case GridLineNamedSpan:
		if g.N > 1 {
			return "span " + strconv.Itoa(g.N) + " " + g.Name
		}
		return "span " + g.Name
	}
	return "auto"
}

func (p GridPlacement) String() string { return p.Start.String() + " / " + p.End.String() }
