package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

func separatorCaptionSheet() *sheet {
	return &sheet{name: "separator_caption", cells: []cell{
		{name: "or", width: 320, tela: kit.Separator{Caption: h.Text("Or continue with"), Props: h.TestID("or-sep")}, probes: []string{"or-sep"}},
		{name: "login", width: 320, tela: h.Div(h.Class("flex w-full flex-col gap-4"),
			kit.Button{Variant: kit.Outline, Class: "w-full", Children: h.Text("Login with Google")},
			kit.Separator{Caption: h.Text("or")},
			kit.Field{Label: "Email", Children: kit.Input{Type: "email", Placeholder: "m@example.com"}})},
		{name: "uppercase", width: 320, tela: kit.Separator{Caption: h.Text("Today"), CaptionClass: "text-xs uppercase tracking-wider"}},
		{name: "icon", width: 320, tela: kit.Separator{Caption: h.Span(h.Class("flex items-center gap-1"), h.Icon(iconCheck, h.Class("size-3.5")), h.Text("Done"))}},
		{name: "vertical", tela: h.Div(h.Class("flex h-32 items-center gap-6 text-sm"),
			h.Div(h.Text("Left")), kit.Separator{Orientation: kit.Vertical, Caption: h.Text("or")}, h.Div(h.Text("Right")))},
	}}
}

func TestSeparatorCaptionGolden(t *testing.T) { separatorCaptionSheet().golden(t) }

func TestSeparatorCaptionRTLGolden(t *testing.T) {
	s := separatorCaptionSheet()
	s.name, s.rtl = "separator_caption_rtl", true
	s.cells = s.cells[:2]
	s.golden(t)
}

// The caption is centered between two lines of the same length.
func TestSeparatorCaptionIsCentered(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("w-80"), kit.Separator{Caption: h.Text("or"), Props: h.TestID("sep")})
	})
	root := app.Bounds(telatest.ByTestID("sep"))
	text := app.Bounds(telatest.ByText("or"))
	left, right := text.X-root.X, root.X+root.Width-(text.X+text.Width)
	if d := left - right; d < -0.01 || d > 0.01 {
		t.Errorf("the caption is %v px from the start and %v px from the end", left, right)
	}
	if root.Width != 320 {
		t.Errorf("the separator is %v px wide, want 320", root.Width)
	}
}

// The caption is text for assistive technologies, which the lines are not;
// the separator role would drop it, so a captioned separator has none.
func TestSeparatorCaptionIsRead(t *testing.T) {
	app := telatest.Mount(t, kit.Separator{Caption: h.Text("Or continue with")})
	app.ExpectAria(`- text: Or continue with`)
	app = telatest.Mount(t, kit.Separator{Caption: h.Text("or"), Semantic: true})
	app.ExpectAria(`- text: or`)
	// Without a caption nothing changes: a decorative separator is hidden
	// and a semantic one is a separator.
	app = telatest.Mount(t, kit.Separator{Semantic: true})
	app.ExpectAria(`- separator`)
}

func TestSeparatorCaptionVerticalFillsTheHeight(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("flex h-40"), kit.Separator{Orientation: kit.Vertical, Caption: h.Text("or"), Props: h.TestID("sep")})
	})
	if r := app.Bounds(telatest.ByTestID("sep")); r.Height != 160 {
		t.Errorf("a vertical captioned separator is %v px tall, want 160", r.Height)
	}
}
