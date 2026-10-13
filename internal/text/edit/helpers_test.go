package edit

import (
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/text"
)

var (
	collOnce sync.Once
	coll     *text.Collection
	collErr  error
)

// collection returns the embedded fonts plus the CJK subset of package
// text's test data (font parsing is the slow part).
func collection(t testing.TB) *text.Collection {
	t.Helper()
	collOnce.Do(func() {
		coll = text.NewDefaultCollection()
		data, err := os.ReadFile("../testdata/fonts/NotoSansJP-Subset.otf")
		if err != nil {
			collErr = err
			return
		}
		_, collErr = coll.Register(data, "Noto Sans JP")
	})
	if collErr != nil {
		t.Fatal(collErr)
	}
	return coll
}

var style16 = text.TextStyle{
	Families:   []string{"Inter", "Noto Sans Arabic", "Noto Sans JP", "Noto Color Emoji"},
	Size:       16,
	LineHeight: text.LineHeightPixels(24),
}

var inf = float32(math.Inf(1))

// driver is a minimal caller: it keeps a layout of the editor's display
// and rebuilds it only when DisplayVersion changes, as core does.
type driver struct {
	t       testing.TB
	e       *Editor
	l       *text.Layout
	ver     uint64
	width   float32
	dir     text.Direction
	rebuild int // number of layouts built
}

func newDriver(t testing.TB, value string, o Options) *driver {
	return &driver{t: t, e: New(value, o), width: inf}
}

func (d *driver) layout() *text.Layout {
	if d.l == nil || d.ver != d.e.DisplayVersion() {
		ws := text.WhiteSpacePre
		if d.e.Options().Multiline {
			ws = text.WhiteSpacePreWrap
		}
		p := &text.Paragraph{
			Text:    d.e.Display(),
			Style:   text.ParagraphStyle{WhiteSpace: ws, Direction: d.dir},
			Default: style16,
		}
		d.l = text.NewLayout(collection(d.t), p)
		d.l.BreakLines(d.width)
		d.ver = d.e.DisplayVersion()
		d.rebuild++
	}
	return d.l
}

func (d *driver) do(cmd Command) Result {
	d.t.Helper()
	var l *text.Layout
	if cmd.Kind.NeedsLayout() {
		l = d.layout()
	}
	return d.e.Do(cmd, l)
}

func (d *driver) kind(k CommandKind) Result { return d.do(Command{Kind: k}) }

func (d *driver) typ(s string) {
	for _, r := range s {
		d.do(Command{Kind: InsertText, Text: string(r)})
	}
}

// state writes the value with the selection: "|" for a caret, "[" and
// "]" around a selection, with "<" after it when the focus is first.
func state(e *Editor) string {
	s, t, back := e.SelectionRange()
	v := e.Value()
	if s == t {
		return v[:s] + "|" + v[s:]
	}
	out := v[:s] + "[" + v[s:t] + "]" + v[t:]
	if back {
		out += "<"
	}
	return out
}

// parse returns an editor for a state written like state's output.
func parse(t testing.TB, st string, o Options) *driver {
	t.Helper()
	back := strings.HasSuffix(st, "<")
	st = strings.TrimSuffix(st, "<")
	var a, f int
	switch {
	case strings.Contains(st, "|"):
		a = strings.Index(st, "|")
		f = a
		st = st[:a] + st[a+1:]
	case strings.Contains(st, "["):
		a = strings.Index(st, "[")
		st = st[:a] + st[a+1:]
		f = strings.Index(st, "]")
		st = st[:f] + st[f+1:]
	default:
		a, f = len(st), len(st)
	}
	if back {
		a, f = f, a
	}
	d := newDriver(t, st, o)
	d.e.Select(a, f)
	return d
}

func expectState(t *testing.T, what string, e *Editor, want string) {
	t.Helper()
	if got := state(e); got != want {
		t.Errorf("%s: got %q, want %q", what, got, want)
	}
}

// clock is a manual clock for Options.Now.
type clock struct{ now time.Duration }

func (c *clock) Now() time.Duration { return c.now }
