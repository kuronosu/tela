package style

import (
	"testing"

	"github.com/kuronosu/tela/internal/text"
)

// styleDir computes the style of a Div with the classes and direction d
// under parent.
func styleDir(t testing.TB, parent *ComputedStyle, d Dir, classes string) *ComputedStyle {
	t.Helper()
	return Compute(testCx, Input{Kind: KindDiv, Parent: parent, Dir: d, Classes: []*Classes{mustCompile(t, classes)}})
}

func TestDirectionInheritance(t *testing.T) {
	root := styleDir(t, nil, DirInherit, "")
	if root.Text.Direction != text.DirectionLTR {
		t.Fatalf("root direction %v, want LTR", root.Text.Direction)
	}
	rtl := styleDir(t, root, DirRTL, "")
	if rtl.Text.Direction != text.DirectionRTL {
		t.Fatalf("dir=rtl gives %v", rtl.Text.Direction)
	}
	kid := styleDir(t, rtl, DirInherit, "")
	if kid.Text.Direction != text.DirectionRTL {
		t.Errorf("a child of an rtl element is %v, want inherited RTL", kid.Text.Direction)
	}
	back := styleDir(t, kid, DirLTR, "")
	if back.Text.Direction != text.DirectionLTR {
		t.Errorf("dir=ltr inside rtl gives %v", back.Text.Direction)
	}
	if ps := kid.ParagraphStyle(); ps.Direction != text.DirectionRTL {
		t.Errorf("paragraph direction %v, want RTL", ps.Direction)
	}
}

func TestDirectionVariants(t *testing.T) {
	root := styleDir(t, nil, DirInherit, "")
	const cls = "rtl:pl-4 ltr:pr-2 rtl:hover:pt-8"
	ltr := styleDir(t, root, DirInherit, cls)
	if ltr.Padding.Left.Px != 0 || ltr.Padding.Right.Px != 8 {
		t.Errorf("ltr: padding left %v right %v, want 0 and 8", ltr.Padding.Left.Px, ltr.Padding.Right.Px)
	}
	rtl := styleDir(t, root, DirRTL, cls)
	if rtl.Padding.Left.Px != 16 || rtl.Padding.Right.Px != 0 {
		t.Errorf("rtl: padding left %v right %v, want 16 and 0", rtl.Padding.Left.Px, rtl.Padding.Right.Px)
	}
	// Inherited: an element under an rtl ancestor matches rtl:.
	kid := styleDir(t, rtl, DirInherit, "rtl:mt-4")
	if kid.Margin.Top.LP.Px != 16 {
		t.Errorf("rtl: under an rtl parent: margin-top %v, want 16", kid.Margin.Top.LP.Px)
	}
	hov := Compute(testCx, Input{Kind: KindDiv, Parent: root, Dir: DirRTL, State: StateHover, Classes: []*Classes{mustCompile(t, cls)}})
	if hov.Padding.Top.Px != 32 {
		t.Errorf("rtl:hover: padding-top %v, want 32", hov.Padding.Top.Px)
	}
}

func TestLogicalUtilities(t *testing.T) {
	root := styleDir(t, nil, DirInherit, "")
	type sides struct{ top, right, bottom, left float32 }
	cases := []struct {
		class    string
		get      func(s *ComputedStyle) sides
		ltr, rtl sides
	}{
		{"ms-4 me-2", func(s *ComputedStyle) sides {
			return sides{s.Margin.Top.LP.Px, s.Margin.Right.LP.Px, s.Margin.Bottom.LP.Px, s.Margin.Left.LP.Px}
		}, sides{0, 8, 0, 16}, sides{0, 16, 0, 8}},
		{"-ms-1 mbs-2 mbe-3", func(s *ComputedStyle) sides {
			return sides{s.Margin.Top.LP.Px, s.Margin.Right.LP.Px, s.Margin.Bottom.LP.Px, s.Margin.Left.LP.Px}
		}, sides{8, 0, 12, -4}, sides{8, -4, 12, 0}},
		{"ps-4 pe-1 pbs-2 pbe-3", func(s *ComputedStyle) sides {
			return sides{s.Padding.Top.Px, s.Padding.Right.Px, s.Padding.Bottom.Px, s.Padding.Left.Px}
		}, sides{8, 4, 12, 16}, sides{8, 16, 12, 4}},
		{"start-4 end-2", func(s *ComputedStyle) sides {
			return sides{s.Inset.Top.LP.Px, s.Inset.Right.LP.Px, s.Inset.Bottom.LP.Px, s.Inset.Left.LP.Px}
		}, sides{0, 8, 0, 16}, sides{0, 16, 0, 8}},
		{"inset-s-4 inset-e-2 inset-bs-1 inset-be-3", func(s *ComputedStyle) sides {
			return sides{s.Inset.Top.LP.Px, s.Inset.Right.LP.Px, s.Inset.Bottom.LP.Px, s.Inset.Left.LP.Px}
		}, sides{4, 8, 12, 16}, sides{4, 16, 12, 8}},
		{"border-s-4 border-e-2 border-bs-1 border-be-8", func(s *ComputedStyle) sides {
			return sides{s.Border.Top.Width, s.Border.Right.Width, s.Border.Bottom.Width, s.Border.Left.Width}
		}, sides{1, 2, 8, 4}, sides{1, 4, 8, 2}},
		// Corners as top-left, top-right, bottom-right, bottom-left.
		{"rounded-s-lg", func(s *ComputedStyle) sides {
			return sides{s.Radius.TopLeft.Px, s.Radius.TopRight.Px, s.Radius.BottomRight.Px, s.Radius.BottomLeft.Px}
		}, sides{8, 0, 0, 8}, sides{0, 8, 8, 0}},
		{"rounded-e-lg", func(s *ComputedStyle) sides {
			return sides{s.Radius.TopLeft.Px, s.Radius.TopRight.Px, s.Radius.BottomRight.Px, s.Radius.BottomLeft.Px}
		}, sides{0, 8, 8, 0}, sides{8, 0, 0, 8}},
		{"rounded-ss-sm rounded-se-md rounded-ee-lg rounded-es-xl", func(s *ComputedStyle) sides {
			return sides{s.Radius.TopLeft.Px, s.Radius.TopRight.Px, s.Radius.BottomRight.Px, s.Radius.BottomLeft.Px}
		}, sides{4, 6, 8, 12}, sides{6, 4, 12, 8}},
		// Physical classes stay physical.
		{"ml-4 pr-2 left-1 rounded-tl-lg border-l-2", func(s *ComputedStyle) sides {
			return sides{s.Inset.Left.LP.Px, s.Padding.Right.Px, s.Radius.TopLeft.Px, s.Border.Left.Width + s.Margin.Left.LP.Px}
		}, sides{4, 8, 8, 18}, sides{4, 8, 8, 18}},
		// Last wins between logical and physical classes of a side.
		{"ms-4 ml-1", func(s *ComputedStyle) sides {
			return sides{0, s.Margin.Right.LP.Px, 0, s.Margin.Left.LP.Px}
		}, sides{0, 0, 0, 4}, sides{0, 16, 0, 4}},
	}
	for _, c := range cases {
		for _, d := range []Dir{DirLTR, DirRTL} {
			got := c.get(styleDir(t, root, d, c.class))
			want := c.ltr
			if d == DirRTL {
				want = c.rtl
			}
			if got != want {
				t.Errorf("%q (dir %d): got %v, want %v", c.class, d, got, want)
			}
		}
	}
	// Border colors follow the logical side too.
	s := styleDir(t, root, DirRTL, "border-s-2 border-s-red-500")
	if s.Border.Right.Color == s.Border.Left.Color || s.Border.Right.Width != 2 {
		t.Errorf("rtl border-s color: right %v left %v", s.Border.Right.Color, s.Border.Left.Color)
	}
}
