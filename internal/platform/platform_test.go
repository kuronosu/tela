package platform

import (
	"image"
	"math"
	"math/rand/v2"
	"strings"
	"testing"
)

func TestPhysicalSizeRounding(t *testing.T) {
	cases := []struct {
		size  Size
		scale float64
		want  image.Point
	}{
		{Size{800, 600}, 1, image.Pt(800, 600)},
		{Size{800, 600}, 2, image.Pt(1600, 1200)},
		{Size{101, 51}, 1.5, image.Pt(152, 77)},  // 151.5, 76.5: halves round up
		{Size{100, 50}, 1.25, image.Pt(125, 63)}, // 62.5 rounds away from zero
		{Size{3, 6}, 7.0 / 6, image.Pt(4, 7)},    // 3.5, 7
		{Size{6, 1}, 13.0 / 12, image.Pt(7, 1)},  // 6.5
		{Size{10.2, 10.6}, 1, image.Pt(10, 11)},  // fractional logical sizes
		{Size{0, -5}, 2, image.Pt(0, 0)},         // empty and negative
		{Size{float32(math.NaN()), 1}, 1, image.Pt(0, 1)},
	}
	for _, c := range cases {
		if got := PhysicalSize(c.size, c.scale); got != c.want {
			t.Errorf("PhysicalSize(%v, %v) = %v, want %v", c.size, c.scale, got, c.want)
		}
	}
	if got := (Metrics{Size{101, 51}, 1.5}).Physical(); got != image.Pt(152, 77) {
		t.Errorf("Metrics.Physical = %v", got)
	}
}

// TestPhysicalSizeMatchesWaylandFractional checks the rule against exact
// integer arithmetic for every wp_fractional_scale_v1 scale from 1x to 4x:
// buffer = round_half_away(W * n / 120) = floor((W*n + 60) / 120).
func TestPhysicalSizeMatchesWaylandFractional(t *testing.T) {
	for n := 120; n <= 480; n++ {
		scale := float64(n) / 120
		for w := 1; w <= 4096; w++ {
			want := (w*n + 60) / 120
			if got := PhysicalSize(Size{float32(w), 1}, scale).X; got != want {
				t.Fatalf("PhysicalSize(%d, %d/120) = %d, want %d", w, n, got, want)
			}
		}
	}
}

func TestLogicalSizeRoundTrip(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	scales := []float64{1, 1.25, 1.5, 1.75, 2, 2.25, 3, 4, 7.0 / 6, 13.0 / 12, 120.0 / 96, 144.0 / 96}
	for range 20000 {
		s := scales[r.IntN(len(scales))]
		if r.IntN(2) == 0 {
			s = 1.0/16 + r.Float64()*(16-1.0/16)
		}
		p := image.Pt(r.IntN(1<<22), r.IntN(1<<22))
		if got := PhysicalSize(LogicalSize(p, s), s); got != p {
			t.Fatalf("PhysicalSize(LogicalSize(%v, %v)) = %v", p, s, got)
		}
	}
	if got := LogicalSize(image.Pt(1017, 0), 1.25); got != (Size{813.6, 0}) {
		t.Errorf("LogicalSize = %v", got)
	}
}

func TestInvalidScalePanics(t *testing.T) {
	for _, s := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		func() {
			defer func() {
				r := recover()
				if r == nil || !strings.Contains(r.(string), "invalid display scale") {
					t.Errorf("PhysicalSize with scale %v: recover() = %v", s, r)
				}
			}()
			PhysicalSize(Size{1, 1}, s)
		}()
	}
}

// waylandCursorShapes is the wp_cursor_shape_device_v1.shape enum, version
// 1, in protocol order (value = index + 1), with "_" replaced by "-".
var waylandCursorShapes = []string{
	"default", "context-menu", "help", "pointer", "progress", "wait", "cell",
	"crosshair", "text", "vertical-text", "alias", "copy", "move", "no-drop",
	"not-allowed", "grab", "grabbing", "e-resize", "n-resize", "ne-resize",
	"nw-resize", "s-resize", "se-resize", "sw-resize", "w-resize", "ew-resize",
	"ns-resize", "nesw-resize", "nwse-resize", "col-resize", "row-resize",
	"all-scroll", "zoom-in", "zoom-out",
}

func TestCursorMatchesWaylandShapes(t *testing.T) {
	for i, name := range waylandCursorShapes {
		c := Cursor(i)
		if c.String() != name {
			t.Errorf("Cursor(%d) = %q, want %q (wp_cursor_shape value %d)", i, c, name, i+1)
		}
		if got, ok := CursorFromName(name); !ok || got != c {
			t.Errorf("CursorFromName(%q) = %v, %v", name, got, ok)
		}
	}
	if CursorZoomOut != Cursor(len(waylandCursorShapes)-1) {
		t.Errorf("CursorZoomOut = %d", CursorZoomOut)
	}
	if CursorNone.String() != "none" || !CursorNone.Valid() || Cursor(CursorNone+1).Valid() {
		t.Errorf("CursorNone misplaced")
	}
	if _, ok := CursorFromName("auto"); ok {
		t.Errorf(`"auto" must not be a platform cursor`)
	}
}

func TestWindowOptionsResolve(t *testing.T) {
	if got := (WindowOptions{}).Resolve(); got.Size != DefaultWindowSize {
		t.Errorf("zero Size resolved to %v", got.Size)
	}
	got := WindowOptions{Size: Size{100, 500}, MinSize: Size{300, 200}}.Resolve()
	if got.Size != (Size{300, 500}) {
		t.Errorf("Size not raised to MinSize: %v", got.Size)
	}
	if got := (WindowOptions{MinSize: Size{10, 10}}).ClampSize(Size{5, 50}); got != (Size{10, 50}) {
		t.Errorf("ClampSize = %v", got)
	}
	// MaxSize clamps, per axis, and MinSize wins over it.
	got = WindowOptions{Size: Size{900, 500}, MaxSize: Size{800, 0}}.Resolve()
	if got.Size != (Size{800, 500}) {
		t.Errorf("Size not clamped to MaxSize: %v", got.Size)
	}
	if got := ClampSize(Size{50, 50}, Size{100, 0}, Size{80, 40}); got != (Size{100, 40}) {
		t.Errorf("ClampSize with min above max = %v", got)
	}
	for _, bad := range []WindowOptions{
		{Size: Size{-1, 10}},
		{Size: Size{10, 0}},
		{Size: Size{float32(math.NaN()), 10}},
		{MinSize: Size{float32(math.Inf(1)), 0}},
		{MaxSize: Size{-5, 0}},
		{Modal: true},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Resolve(%+v) did not panic", bad)
				}
			}()
			bad.Resolve()
		}()
	}
}

func TestCheckFramebuffer(t *testing.T) {
	m := Metrics{Size{10, 5}, 2}
	CheckFramebuffer(image.NewRGBA(image.Rect(0, 0, 20, 10)), m)
	for _, fb := range []*image.RGBA{
		nil,
		image.NewRGBA(image.Rect(0, 0, 10, 5)),
		image.NewRGBA(image.Rect(1, 0, 21, 10)),
		{Pix: make([]byte, 10), Stride: 80, Rect: image.Rect(0, 0, 20, 10)},
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Errorf("CheckFramebuffer(%v) did not panic", fb)
				} else if s, _ := r.(string); !strings.HasPrefix(s, "platform: Present") {
					t.Errorf("panic message %q is not actionable", s)
				}
			}()
			CheckFramebuffer(fb, m)
		}()
	}
}

func TestClipDamage(t *testing.T) {
	b := image.Rect(0, 0, 100, 50)
	full := []image.Rectangle{b}
	if got := ClipDamage(nil, b, false); !equalRects(got, full) {
		t.Errorf("nil damage = %v", got)
	}
	if got := ClipDamage([]image.Rectangle{}, b, false); !equalRects(got, full) {
		t.Errorf("empty damage = %v", got)
	}
	in := []image.Rectangle{image.Rect(-5, -5, 10, 10), image.Rect(90, 40, 200, 200), image.Rect(200, 200, 300, 300), {}}
	want := []image.Rectangle{image.Rect(0, 0, 10, 10), image.Rect(90, 40, 100, 50)}
	if got := ClipDamage(in, b, false); !equalRects(got, want) {
		t.Errorf("clipped damage = %v, want %v", got, want)
	}
	if got := ClipDamage(in, b, true); !equalRects(got, full) {
		t.Errorf("full damage = %v", got)
	}
	if got := ClipDamage([]image.Rectangle{image.Rect(200, 200, 300, 300)}, b, false); got != nil {
		t.Errorf("damage entirely outside = %v, want nil", got)
	}
	if in[0] != image.Rect(-5, -5, 10, 10) {
		t.Errorf("ClipDamage modified its input")
	}
}

func equalRects(a, b []image.Rectangle) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCopyHelpers(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for i := range src.Pix {
		src.Pix[i] = byte(i)
	}
	r := image.Rect(1, 1, 3, 3)

	dst := image.NewRGBA(src.Rect)
	CopyRGBA(dst, src, r)
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			got, want := dst.RGBAAt(x, y), src.RGBAAt(x, y)
			if !image.Pt(x, y).In(r) {
				want = dst.RGBAAt(-1, -1) // zero
			}
			if got != want {
				t.Errorf("CopyRGBA pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}

	const stride = 4*4 + 8 // padded rows
	bgra := make([]byte, stride*3)
	CopyRGBAToBGRA(bgra, stride, src, r)
	CopyRGBAToBGRA(bgra, stride, src, image.Rectangle{}) // no-op
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			d := bgra[y*stride+4*x:]
			s := src.RGBAAt(x, y)
			if image.Pt(x, y).In(r) {
				if d[0] != s.B || d[1] != s.G || d[2] != s.R || d[3] != s.A {
					t.Errorf("BGRA pixel (%d,%d) = %v, src %v", x, y, d[:4], s)
				}
			} else if d[0]|d[1]|d[2]|d[3] != 0 {
				t.Errorf("BGRA pixel (%d,%d) outside r was written", x, y)
			}
		}
	}
}

func TestEventEnums(t *testing.T) {
	if !Buttons(ButtonPrimary.Mask() | ButtonBack.Mask()).Has(ButtonBack) {
		t.Errorf("Buttons.Has")
	}
	if Buttons(0xff).Has(ButtonNone) || ButtonNone.Mask() != 0 {
		t.Errorf("ButtonNone must have no mask")
	}
	if PointerCancel.String() != "cancel" || PointerPen.String() != "pen" {
		t.Errorf("enum names")
	}
	var _ Event = ConfigureEvent{}
	var _ Handler = HandlerFunc(func(Window, Event) {})
}
