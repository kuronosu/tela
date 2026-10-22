package remote

import (
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// eventSamples has one value of every event type of package platform,
// with every field set.
var eventSamples = []platform.Event{
	platform.ConfigureEvent{Metrics: platform.Metrics{Size: platform.Size{W: 801.5, H: 600.25}, Scale: 1.25}},
	platform.FrameEvent{Time: 123456789 * time.Nanosecond},
	platform.CloseRequestedEvent{},
	platform.FocusEvent{Focused: true},
	platform.WakeEvent{},
	platform.SettingsEvent{Settings: platform.SystemSettings{DarkMode: true, ReducedMotion: true, DoubleClickInterval: 400 * time.Millisecond}},
	platform.PointerEvent{Action: platform.PointerDown, Kind: platform.PointerPen, ID: 7, Pos: platform.Point{X: 1.5, Y: -2.25},
		Button: platform.ButtonSecondary, Buttons: 3, Mods: platform.ModCtrl | platform.ModShift, Time: time.Second},
	platform.ScrollEvent{Kind: platform.PointerMouse, Pos: platform.Point{X: 3, Y: 4}, Delta: platform.Point{X: -1, Y: 2.5},
		Steps: platform.Point{X: 0.5, Y: -1}, Phase: platform.ScrollPhaseEnd, Mods: platform.ModAlt, Time: 2 * time.Second},
	platform.KeyEvent{Action: platform.KeyRelease, Code: platform.CodeKeyA, Key: "é", Text: "é", Location: platform.KeyLocationRight,
		Repeat: true, Synthetic: true, Mods: platform.ModSuper, Time: 3 * time.Second},
	platform.IMEEvent{DeleteBefore: 3, DeleteAfter: 4, Commit: "日本", Preedit: "ご", CursorStart: 0, CursorEnd: 3},
	platform.IMESessionEvent{Active: true},
}

// TestEveryEventHasAnEncoding fails when package platform gains an event
// type (a type with an isEvent method) that eventSamples, and therefore
// the wire encoding, does not cover.
func TestEveryEventHasAnEncoding(t *testing.T) {
	for _, e := range eventSamples {
		if got := roundTrip(t, e); !reflect.DeepEqual(got, e) {
			t.Errorf("round trip of %T: got %+v, want %+v", e, got, e)
		}
	}
	// go test runs a package's tests in its source directory, so package
	// platform is the parent directory. A test binary run elsewhere (built
	// with go test -c and copied to a machine without the repository) has
	// no sources to compare eventSamples with.
	dir := filepath.Join("..")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var declared []string
	fset := token.NewFileSet()
	sources := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		if f.Name.Name != "platform" {
			continue
		}
		sources++
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Name.Name != "isEvent" || fd.Recv == nil || len(fd.Recv.List) != 1 {
				continue
			}
			typ := fd.Recv.List[0].Type
			if s, ok := typ.(*ast.StarExpr); ok {
				typ = s.X
			}
			if id, ok := typ.(*ast.Ident); ok {
				declared = append(declared, id.Name)
			}
		}
	}
	if sources == 0 {
		abs, _ := filepath.Abs(dir)
		t.Skipf("no source file of package platform in %s (the test binary runs outside the source tree): the event types cannot be listed; the round trips above ran", abs)
	}
	if len(declared) < 10 {
		t.Fatalf("found only %d event types in package platform (%v); the parser is looking in the wrong place", len(declared), declared)
	}
	var sampled []string
	for _, e := range eventSamples {
		sampled = append(sampled, reflect.TypeOf(e).Name())
	}
	slices.Sort(declared)
	slices.Sort(sampled)
	for _, name := range declared {
		if !slices.Contains(sampled, name) {
			t.Errorf("platform.%s has no wire encoding: add it to enc.event and dec.event in protocol.go, to eventSamples, and to the host's routing", name)
		}
	}
	for _, name := range sampled {
		if !slices.Contains(declared, name) {
			t.Errorf("eventSamples has %s, which is not an event type of package platform", name)
		}
	}
}

// TestEventRoundTripRandom round-trips random values of every event type:
// a field added to an event struct but not to its encoding fails here.
func TestEventRoundTripRandom(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, sample := range eventSamples {
		typ := reflect.TypeOf(sample)
		for range 200 {
			v, ok := quick.Value(typ, r)
			if !ok {
				t.Fatalf("cannot generate %v", typ)
			}
			e := v.Interface().(platform.Event)
			if got := roundTrip(t, e); !reflect.DeepEqual(got, e) {
				t.Fatalf("round trip of %T: got %+v, want %+v", e, got, e)
			}
		}
	}
}

func roundTrip(t *testing.T, e platform.Event) platform.Event {
	t.Helper()
	m := newMsg(msgEvent)
	m.event(e)
	b := m.bytes()
	d := &dec{b: b[5:]}
	got := d.event()
	if err := d.end(); err != nil {
		t.Fatalf("decode %T: %v", e, err)
	}
	return got
}

func TestTextInputAndOptionsRoundTrip(t *testing.T) {
	states := []*platform.TextInputState{
		nil,
		{CursorArea: platform.Rect{X: 1, Y: 2, W: 0, H: 18}, Purpose: platform.TextPurposeURL, Multiline: true},
		{Purpose: platform.TextPurposePassword, Surrounding: &platform.SurroundingText{Text: "héllo", Cursor: 3, Anchor: 1}},
	}
	for _, s := range states {
		e := newMsg(msgSetTextInput)
		e.textInput(s)
		d := &dec{b: e.bytes()[5:]}
		got := d.textInput()
		if err := d.end(); err != nil || !got.Equal(s) {
			t.Errorf("text input %+v: got %+v (%v)", s, got, err)
		}
	}
	o := platform.WindowOptions{Title: "t", AppID: "com.example", Size: platform.Size{W: 3, H: 4}, MinSize: platform.Size{W: 1, H: 2}, FixedSize: true}
	e := newMsg(msgNewWindow)
	e.windowOptions(o)
	d := &dec{b: e.bytes()[5:]}
	if got := d.windowOptions(); d.end() != nil || got != o {
		t.Errorf("window options: got %+v, want %+v", got, o)
	}
}

func TestDecodeErrors(t *testing.T) {
	d := &dec{b: []byte{evPointer, 1}}
	d.event()
	if d.end() == nil {
		t.Errorf("truncated event decoded without error")
	}
	d = &dec{b: []byte{200}}
	d.event()
	if d.end() == nil {
		t.Errorf("unknown event tag decoded without error")
	}
	e := newMsg(msgPresent)
	e.rects([]image.Rectangle{image.Rect(0, 0, 20, 20)})
	d = &dec{b: e.bytes()[5:]}
	d.rects(image.Rect(0, 0, 10, 10))
	if d.end() == nil {
		t.Errorf("damage outside the frame decoded without error")
	}
	d = &dec{b: []byte{1, 2}}
	d.u8()
	if d.end() == nil {
		t.Errorf("trailing bytes not reported")
	}
}

// chunkReader returns its data a few bytes at a time.
type chunkReader struct {
	data []byte
	n    int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(c.data) == 0 {
		return 0, io.EOF
	}
	n := min(len(p), c.n, len(c.data))
	copy(p, c.data[:n])
	c.data = c.data[n:]
	return n, nil
}
func (*chunkReader) write([]byte, *os.File) error    { return nil }
func (*chunkReader) takeFile() *os.File              { return nil }
func (*chunkReader) SetReadDeadline(time.Time) error { return nil }
func (*chunkReader) Close() error                    { return nil }

func TestFrameReader(t *testing.T) {
	var stream []byte
	var want []string
	for i := range 50 {
		e := newMsg(msgCtlNotify)
		s := strings.Repeat("x", i*997)
		e.str(s)
		stream = append(stream, e.bytes()...)
		want = append(want, s)
	}
	for _, chunk := range []int{1, 3, 4096, 1 << 20} {
		r := newFrameReader(&chunkReader{data: slices.Clone(stream), n: chunk})
		r.buf = make([]byte, 16) // force growth
		for i, w := range want {
			typ, p, err := r.next()
			if err != nil {
				t.Fatalf("chunk %d frame %d: %v", chunk, i, err)
			}
			d := &dec{b: p}
			if typ != msgCtlNotify || d.str() != w || d.end() != nil {
				t.Fatalf("chunk %d frame %d: wrong content", chunk, i)
			}
		}
		if _, _, err := r.next(); err != io.EOF {
			t.Errorf("chunk %d: end of stream: %v, want EOF", chunk, err)
		}
	}
	// A truncated frame is an unexpected EOF, not a clean end.
	r := newFrameReader(&chunkReader{data: stream[:10], n: 4})
	if _, _, err := r.next(); err != nil {
		t.Fatalf("first frame: %v", err)
	}
	if _, _, err := r.next(); err != io.ErrUnexpectedEOF {
		t.Errorf("truncated frame: %v, want ErrUnexpectedEOF", err)
	}
}
