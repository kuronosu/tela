package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

func separatorAlignSheet() *sheet {
	return &sheet{name: "separator_align", cells: []cell{
		{name: "start", width: 320, tela: kit.Separator{Caption: h.Text("Yesterday"), CaptionAlign: kit.CaptionStart}},
		{name: "center", width: 320, tela: kit.Separator{Caption: h.Text("Yesterday")}},
		{name: "end", width: 320, tela: kit.Separator{Caption: h.Text("Yesterday"), CaptionAlign: kit.CaptionEnd}},
		{name: "vertical-start", tela: h.Div(h.Class("flex h-32 items-center gap-6 text-sm"),
			h.Div(h.Text("Left")), kit.Separator{Orientation: kit.Vertical, Caption: h.Text("or"), CaptionAlign: kit.CaptionStart}, h.Div(h.Text("Right")))},
		{name: "vertical-end", tela: h.Div(h.Class("flex h-32 items-center gap-6 text-sm"),
			h.Div(h.Text("Left")), kit.Separator{Orientation: kit.Vertical, Caption: h.Text("or"), CaptionAlign: kit.CaptionEnd}, h.Div(h.Text("Right")))},
	}}
}

func TestSeparatorAlignGolden(t *testing.T) { separatorAlignSheet().golden(t) }

func TestSeparatorAlignRTLGolden(t *testing.T) {
	s := separatorAlignSheet()
	s.name, s.rtl = "separator_align_rtl", true
	s.cells = s.cells[:3]
	s.golden(t)
}

func captionOffsets(t *testing.T, dir string, a kit.CaptionAlign) (fromStart, fromEnd float32) {
	t.Helper()
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Dir(dir), h.Class("w-80"), kit.Separator{Caption: h.Text("or"), CaptionAlign: a, Props: h.TestID("sep")})
	}, telatest.Size(400, 60))
	root, text := app.Bounds(telatest.ByTestID("sep")), app.Bounds(telatest.ByText("or"))
	if dir == "rtl" {
		return root.X + root.Width - (text.X + text.Width), text.X - root.X
	}
	return text.X - root.X, root.X + root.Width - (text.X + text.Width)
}

func TestSeparatorCaptionAlign(t *testing.T) {
	for _, dir := range []string{"ltr", "rtl"} {
		// start: a 24 px line and a gap of 8 before the caption.
		s, e := captionOffsets(t, dir, kit.CaptionStart)
		if s != 32 || e <= 100 {
			t.Errorf("%s start: caption %v px from the start and %v from the end, want 32 and a long line", dir, s, e)
		}
		s, e = captionOffsets(t, dir, kit.CaptionEnd)
		if e != 32 || s <= 100 {
			t.Errorf("%s end: caption %v px from the start and %v from the end, want a long line and 32", dir, s, e)
		}
		s, e = captionOffsets(t, dir, kit.CaptionCenter)
		if d := s - e; d < -0.01 || d > 0.01 {
			t.Errorf("%s center: %v px from the start, %v from the end", dir, s, e)
		}
	}
}

func TestSeparatorCaptionAlignRejectsUnknown(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("an unknown CaptionAlign did not panic in dev mode")
		}
	}()
	telatest.Mount(t, kit.Separator{Caption: h.Text("x"), CaptionAlign: "middle"})
}
