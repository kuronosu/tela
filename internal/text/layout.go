package text

import (
	"math"
	"slices"

	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/harfbuzz"
	"github.com/go-text/typesetting/language"
	"github.com/kuronosu/tela/internal/gfx"
)

// Layout is a paragraph shaped once and broken into lines, possibly many
// times: NewLayout itemizes and shapes, BreakLines breaks, aligns and
// positions without shaping again (Parley's model). See the "Paragraph
// layout" section of the package documentation.
//
// A Layout is not safe for concurrent mutation: BreakLines and AlignTo
// must not run concurrently with any other method. The read-only methods
// (accessors, Paint, intrinsic widths after their first call) may run
// concurrently with each other.
type Layout struct {
	coll   *Collection
	src    string
	para   ParagraphStyle
	styles []TextStyle
	sinfo  []styleInfo
	text   procText
	segs   []uint8 // per boundary of text.runes: seg* flags (grapheme, line and word boundaries)
	base   uint8   // paragraph embedding level: 0 LTR, 1 RTL

	runs     []run
	clusters []cluster
	glyphs   []glyphData
	boxes    []InlineBox
	boxClus  []int32 // cluster index of each box, -1 if the box has none

	// Results of BreakLines.
	maxWidth   float32
	alignWidth float32
	lines      []lineData
	items      []itemData
	ellipses   []ellipsisData
	cadv       []float32 // per cluster: advance on its line
	cx         []float32 // per cluster: x of its left edge in layout coordinates
	cline      []int32   // per cluster: line index, -1 when hidden
	justUnits  []uint8   // per cluster: justification opportunities after it
	width      float32
	height     float32
	hiddenFrom int32 // first cluster dropped by LineClamp, or len(clusters)

	minContent, maxContent float32 // NaN until computed
}

// styleInfo is the resolved form of a TextStyle.
type styleInfo struct {
	face  *Face   // first available font, for the span box and decorations
	size  float32 // font size in px
	paint gfx.Paint
	// Span box (CSS Inline 3): quantized ascent and descent of face, the
	// resolved line height and the box above and below the baseline.
	ascent, descent, lineHeight float32
	over, under                 float32
	normal                      bool // line-height: normal
	xHeight                     float32
	// Decoration geometry in px, y down from the baseline (top edges).
	underline, underlineThick float32
	strike, strikeThick       float32
	overline                  float32
	decoPaint                 gfx.Paint

	features []harfbuzz.Feature
	lang     language.Language
	ellipsis *ellipsisData // built on first use
}

type runKind uint8

const (
	runText runKind = iota
	runBox
	runTab
	runNewline
)

// run is a shaping item: text of one face, direction, script and shaping
// style, or a special item (inline box, tab, forced break).
type run struct {
	kind   runKind
	level  uint8
	face   *Face
	size   float32
	r0, r1 int32 // processed runes
	c0, c1 int32 // clusters
	box    int32 // inline box index for runBox
	// Quantized metrics of face at size, for line-height: normal.
	ascent, descent, gap float32
}

type clusterFlag uint16

const (
	cfSoftBefore clusterFlag = 1 << iota // soft wrap opportunity before the cluster
	cfMustBefore                         // forced line break before the cluster
	cfSpace                              // U+0020 (or a collapsed space, tab or newline)
	cfOtherSpace                         // another space separator that hangs (U+3000...)
	cfWordSep                            // word separator: word-spacing and justification
	cfTab                                // a preserved tab
	cfNewline                            // a preserved newline or other forced break character
	cfBox                                // an inline box
	cfCJK                                // CJK character: inter-character justification
	cfBidiWS                             // white space for rule L1 of UAX #9
	cfLetter                             // letter or digit, for word-break
	cfInvisible                          // controls and format characters: no letter-spacing
	cfCursive                            // a letter of a cursive script: no letter-spacing
)

// cluster is the unit of line breaking, positioning, hit-testing and
// truncation: one or more grapheme clusters that shaping cannot split
// (ligatures), always on grapheme and source character boundaries.
type cluster struct {
	r0, r1 int32 // processed runes
	g0, g1 int32 // glyphs, in visual order
	run    int32
	style  uint16
	flags  clusterFlag
	graph  uint16  // grapheme clusters it contains
	adv    float32 // advance in px with letter- and word-spacing; tabs: set by BreakLines
}

type glyphData struct {
	id          gfx.GlyphID
	adv, dx, dy float32 // px; dy grows downward
}

// BreakReason tells why a line ends.
type BreakReason uint8

const (
	// BreakNone ends the last line of the paragraph.
	BreakNone BreakReason = iota
	// BreakSoft is a wrap at a soft wrap opportunity.
	BreakSoft
	// BreakForced is a preserved newline or another forced break.
	BreakForced
	// BreakEmergency is a wrap inside a word (overflow-wrap).
	BreakEmergency
)

type lineData struct {
	c0, c1 int32 // logical cluster range, trailing white space included
	trail  int32 // first cluster of the trailing white space that hangs or is removed
	remove bool  // the trailing white space is removed (zero advance), not hanging
	i0, i1 int32 // items, in visual order
	reason BreakReason

	top, height, baseline float32
	ascent, descent       float32 // above and below the baseline
	x                     float32 // left edge of the first visual item
	width                 float32 // advance without hanging white space
	hang                  float32 // advance of hanging white space
	justify               float32 // extra advance per justification opportunity
	ellipsis              int32   // index into ellipses, -1 if none
}

type itemKind uint8

const (
	itemText itemKind = iota
	itemBox
	itemEllipsis
)

// itemData is a piece of a line at one bidi level: the visible clusters
// of one run, an inline box, or an ellipsis.
type itemData struct {
	kind   itemKind
	level  uint8
	run    int32 // run, or ellipsis index for itemEllipsis
	c0, c1 int32 // logical cluster range
	x, w   float32
}

type ellipsisData struct {
	face   *Face
	size   float32
	style  uint16
	glyphs []glyphData
	width  float32
}

// NewLayout lays out p with the fonts of c: text-transform and white-space
// processing, bidi, script and font itemization, shaping, and line breaking
// without a width limit (call BreakLines to wrap). p is not retained.
//
// Shaping uses each Face's go-text face, which is not safe for concurrent
// use; NewLayout serializes its shaping internally, so it may be called from
// any goroutine.
func NewLayout(c *Collection, p *Paragraph) *Layout {
	p.validate()
	if c == nil || c.Face(Query{}, 16) == nil {
		panic("text.NewLayout: the Collection has no fonts; start from NewDefaultCollection")
	}
	l := &Layout{
		coll:       c,
		src:        p.Text,
		para:       p.Style,
		minContent: float32(math.NaN()),
		maxContent: float32(math.NaN()),
	}
	l.styles = make([]TextStyle, len(p.Spans)+1)
	l.styles[0] = p.Default
	for i, s := range p.Spans {
		l.styles[i+1] = s.Style
	}
	l.boxes = slices.Clone(p.Boxes)
	order := make([]int, len(l.boxes))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return l.boxes[a].At - l.boxes[b].At })
	l.text = prepareText(p, l.styles, order)
	l.resolveStyles()
	l.shape()
	l.BreakLines(float32(math.Inf(1)))
	return l
}

// resolveStyles fills l.sinfo.
func (l *Layout) resolveStyles() {
	l.sinfo = make([]styleInfo, len(l.styles))
	for i := range l.styles {
		st := &l.styles[i]
		si := &l.sinfo[i]
		si.size = st.size()
		si.face = l.coll.Face(st.query(), si.size)
		si.paint = st.paint()
		si.decoPaint = st.Decoration.Paint
		if si.decoPaint == nil {
			si.decoPaint = si.paint
		}
		var m Metrics
		if si.face != nil {
			m = si.face.Metrics()
		}
		si.ascent, si.descent = qround(m.Ascent, si.size), qround(m.Descent, si.size)
		gap := qround(m.LineGap, si.size)
		switch st.LineHeight.Kind {
		case LineHeightScale:
			si.lineHeight = float32(st.LineHeight.Value * si.size)
		case LineHeightPx:
			si.lineHeight = st.LineHeight.Value
		default:
			si.lineHeight = si.ascent + si.descent + gap
			si.normal = true
		}
		if !finite32(si.lineHeight) || si.lineHeight < 0 {
			si.lineHeight = si.ascent + si.descent + gap
		}
		si.over, si.under = halfLeading(si.ascent, si.descent, si.lineHeight)
		// The x-height is not rounded: Chromium places vertical-align:
		// middle boxes with the exact value.
		si.xHeight = float32(m.XHeight * si.size)
		if si.xHeight <= 0 {
			si.xHeight = float32(si.ascent * 0.56)
		}
		si.underline = float32(m.UnderlineOffset * si.size)
		si.underlineThick = float32(m.UnderlineThickness * si.size)
		si.strike = float32(m.StrikeoutOffset * si.size)
		si.strikeThick = float32(m.StrikeoutThickness * si.size)
		si.overline = -si.ascent
		if t := st.Decoration.Thickness; t > 0 && finite32(t) {
			si.strike += (si.strikeThick - t) / 2 // keep the strikeout centered
			si.underlineThick, si.strikeThick = t, t
		}
		si.features = styleFeatures(st)
		if st.Language != "" {
			si.lang = language.NewLanguage(st.Language)
		}
	}
}

// qround rounds a font metric scaled to px to whole pixels, as Blink and
// Parley do for line box metrics.
func qround(em, size float32) float32 {
	return float32(math.Round(float64(em) * float64(size)))
}

// halfLeading splits the leading of a box (CSS 2.1, section 10.8.1): the
// part above the baseline is ascent plus the half-leading rounded down.
func halfLeading(ascent, descent, lineHeight float32) (over, under float32) {
	over = ascent + float32(math.Floor(float64(lineHeight-(ascent+descent))/2))
	return over, lineHeight - over
}

func finite32(v float32) bool { return v == v && !math.IsInf(float64(v), 0) }

// optionalLigatures are the features that letter-spacing disables, as in
// Chromium (CSS Text 3, section 8.2: no optional ligatures when spacing is
// added between characters).
var optionalLigatures = []string{"liga", "clig", "dlig", "hlig", "calt"}

func styleFeatures(st *TextStyle) []harfbuzz.Feature {
	var fs []harfbuzz.Feature
	add := func(tag string, v uint32) {
		if len(tag) != 4 {
			return
		}
		t := ot.MustNewTag(tag)
		for i := range fs {
			if fs[i].Tag == t {
				fs[i].Value = v
				return
			}
		}
		fs = append(fs, harfbuzz.Feature{Tag: t, Value: v, Start: harfbuzz.FeatureGlobalStart, End: harfbuzz.FeatureGlobalEnd})
	}
	if st.LetterSpacing != 0 {
		for _, t := range optionalLigatures {
			add(t, 0)
		}
	}
	for i, t := range numericTags {
		if st.Numeric&(1<<i) != 0 {
			add(t, 1)
		}
	}
	for _, f := range st.Features {
		add(f.Tag, f.Value)
	}
	return fs
}
