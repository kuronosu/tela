package kit_test

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// shadcn/ui's TooltipContent and its arrow (new-york-v4/ui/tooltip.tsx at
// kit.ShadcnCommit), verbatim.
const (
	shadcnTooltipContent = "z-50 w-fit origin-(--radix-tooltip-content-transform-origin) animate-in rounded-md bg-foreground px-3 py-1.5 text-xs text-balance text-background fade-in-0 zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95"
	shadcnTooltipArrow   = "z-50 size-2.5 translate-y-[calc(-50%_-_2px)] rotate-45 rounded-[2px] bg-foreground fill-foreground"
)

// radixArrow is what Radix renders for a tooltip's arrow on the top side,
// centered: a span at the bottom edge pushed out by its own height, with the
// svg of shadcn's class inside.
const radixArrow = `<span style="position:absolute;left:calc(50% - 5px);bottom:0;transform:translateY(100%)">` +
	`<svg class="` + shadcnTooltipArrow + `" width="10" height="5" viewBox="0 0 30 10" preserveAspectRatio="none" style="display:block"><polygon points="0,0 30,0 15,10"/></svg></span>`

// tooltipCell is a tooltip on both sides. A text of one line is the whole
// of shadcn's markup; a text that wraps has its text-balance left out of the
// markup, because Tela wraps greedily (text-wrap: balance parses as wrap, a
// gap of the engine) and Chromium would break the lines elsewhere.
func tooltipCell(name string, t kit.Tooltip, text string, width int, wraps bool) cell {
	t.Props = h.TestID(name + "-tip")
	t.Content = h.Text(text)
	classes := strings.NewReplacer("animate-in ", "", "fade-in-0 ", "", "zoom-in-95 ", "").Replace(shadcnTooltipContent)
	if wraps {
		classes = strings.Replace(classes, "text-balance ", "", 1)
	}
	return cell{
		name: name, width: width, probes: []string{name + "-tip"}, tela: kit.StaticTooltip(t),
		html: fmt.Sprintf(`<div role="tooltip" data-probe="%s-tip" data-slot="tooltip-content" class="relative %s">%s%s</div>`,
			name, cn(staticClasses(classes), t.Class), text, radixArrow),
	}
}

func tooltipSheet() *sheet {
	// Three lines of light text on a dark box: the anti-aliasing of text weighs more here.
	return &sheet{name: "tooltip", slot: 260, mean: [2]float64{4.5, 3.5}, cells: []cell{
		tooltipCell("short", kit.Tooltip{}, "Add to library", 0, false),
		tooltipCell("sentence", kit.Tooltip{}, "Save your changes before you leave this page", 0, true),
		tooltipCell("wrapped", kit.Tooltip{}, "A longer explanation that does not fit on a single line of the tooltip and has to wrap", 200, true),
		tooltipCell("class", kit.Tooltip{Class: "px-2 py-1"}, "Compact", 0, false),
	}}
}

func TestTooltipGolden(t *testing.T)   { tooltipSheet().golden(t) }
func TestChromiumTooltip(t *testing.T) { tooltipSheet().chromium(t) }

// tipPage has two buttons with tooltips, "Save" at (16, 100) and "Print" at
// (136, 100), and an empty area below.
func tipPage(a, b kit.Tooltip) func() tela.Node {
	button := func(id, label string) kit.Button {
		return kit.Button{Variant: kit.Outline, Class: "w-24", Props: h.TestID(id), Children: h.Text(label)}
	}
	return func() tela.Node {
		a.Trigger, a.Props, a.Content = button("save", "Save"), h.TestID("tip-save"), h.Text("Saves the file")
		b.Trigger, b.Props, b.Content = button("print", "Print"), h.TestID("tip-print"), h.Text("Prints the page")
		return h.Div(h.Class("relative h-full"),
			h.Div(h.Class("absolute top-[100px] left-4"), a),
			h.Div(h.Class("absolute top-[100px] left-[136px]"), b))
	}
}

func isOpen(app *telatest.App, id string) bool { return app.Count(telatest.ByTestID(id)) == 1 }

func mountTips(t *testing.T, a, b kit.Tooltip, opts ...telatest.Option) *telatest.App {
	opts = append([]telatest.Option{telatest.Size(400, 300), telatest.Theme(kit.Theme()), telatest.NoSettle()}, opts...)
	return telatest.MountFunc(t, tipPage(a, b), opts...)
}

func TestTooltipOpensOnHoverAtOnce(t *testing.T) {
	// shadcn's provider has no delay: the tooltip is there as the pointer
	// arrives, and it is the trigger's description.
	app := mountTips(t, kit.Tooltip{}, kit.Tooltip{})
	save := telatest.ByTestID("save")
	app.Hover(save)
	if !isOpen(app, "tip-save") {
		t.Fatal("the tooltip did not open at once")
	}
	app.Advance(300 * time.Millisecond) // the entrance ends
	app.ExpectAria(`
		- button "Save"
		- button "Print"
		- tooltip "Saves the file"
	`)
	// It is above the button, centered, touching it (the arrow overlaps).
	tip, btn := app.Bounds(telatest.ByTestID("tip-save")), app.Bounds(save)
	if !near(tip.Y+tip.Height, btn.Y) || !near(tip.X+tip.Width/2, btn.X+btn.Width/2) {
		t.Errorf("tooltip %v, trigger %v: want it above the trigger, centered", tip, btn)
	}
	// Leaving closes it after the close delay.
	app.MouseMove(300, 250)
	app.Advance(400 * time.Millisecond)
	if !isOpen(app, "tip-save") {
		t.Error("the tooltip closed before the close delay")
	}
	app.Advance(200 * time.Millisecond)
	if isOpen(app, "tip-save") {
		t.Error("the tooltip stayed open after the close delay")
	}
}

func TestTooltipDelay(t *testing.T) {
	app := mountTips(t, kit.Tooltip{Delay: 700 * time.Millisecond}, kit.Tooltip{Delay: 700 * time.Millisecond})
	save := telatest.ByTestID("save")
	app.Hover(save)
	app.Advance(600 * time.Millisecond)
	if isOpen(app, "tip-save") {
		t.Fatal("the tooltip opened before its delay")
	}
	app.Advance(150 * time.Millisecond)
	if !isOpen(app, "tip-save") {
		t.Fatal("the tooltip did not open after its delay")
	}
	// The group is warm: the other trigger shows its tooltip at once.
	app.Hover(telatest.ByTestID("print"))
	if !isOpen(app, "tip-print") || isOpen(app, "tip-save") {
		t.Errorf("warm: print %v, save %v; want true, false", isOpen(app, "tip-print"), isOpen(app, "tip-save"))
	}
	// Once cold the delay applies again.
	app.MouseMove(300, 250)
	app.Advance(2 * time.Second)
	app.Hover(save)
	app.Advance(600 * time.Millisecond)
	if isOpen(app, "tip-save") {
		t.Error("cold: the tooltip opened before its delay")
	}
}

func TestTooltipKeyboard(t *testing.T) {
	app := mountTips(t, kit.Tooltip{Delay: time.Second}, kit.Tooltip{})
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("button", "Save"))
	if !isOpen(app, "tip-save") {
		t.Fatal("keyboard focus did not open the tooltip at once, whatever the Delay")
	}
	app.Press("Escape")
	if isOpen(app, "tip-save") {
		t.Error("Escape did not close the tooltip")
	}
	// Moving the focus to the next trigger opens its tooltip.
	app.Press("Tab")
	if !isOpen(app, "tip-print") {
		t.Error("the next trigger's tooltip did not open")
	}
	app.Press("Tab")
	if isOpen(app, "tip-print") {
		t.Error("the tooltip stayed open after the focus left")
	}
}

func TestTooltipPressClosesAndTriggerStillWorks(t *testing.T) {
	clicks := 0
	app := mountTips(t, kit.Tooltip{}, kit.Tooltip{})
	_ = clicks
	save := telatest.ByTestID("save")
	app.Hover(save)
	if !isOpen(app, "tip-save") {
		t.Fatal("not open")
	}
	app.Click(save)
	if isOpen(app, "tip-save") {
		t.Error("a press on the trigger did not close the tooltip")
	}
}

func TestTooltipDisabledAndControlled(t *testing.T) {
	app := mountTips(t, kit.Tooltip{Disabled: true}, kit.Tooltip{})
	app.Hover(telatest.ByTestID("save"))
	app.Advance(time.Second)
	if isOpen(app, "tip-save") {
		t.Error("a disabled tooltip opened")
	}

	var log []bool
	open := true
	app = mountTips(t, kit.Tooltip{Open: &open, OnOpenChange: func(o bool) { log = append(log, o) }}, kit.Tooltip{})
	if !isOpen(app, "tip-save") {
		t.Fatal("a controlled open tooltip is not open")
	}
	app.Press("Escape")
	if !isOpen(app, "tip-save") {
		t.Error("a controlled tooltip closed by itself")
	}
	if fmt.Sprint(log) != "[false]" {
		t.Errorf("OnOpenChange got %v, want [false]", log)
	}
}

func TestTooltipPlacement(t *testing.T) {
	app := mountTips(t, kit.Tooltip{Placement: "bottom", Offset: 6}, kit.Tooltip{Placement: "right", Align: "start"})
	app.Hover(telatest.ByTestID("save"))
	app.Advance(300 * time.Millisecond)
	tip, btn := app.Bounds(telatest.ByTestID("tip-save")), app.Bounds(telatest.ByTestID("save"))
	if !near(tip.Y, btn.Y+btn.Height+6) {
		t.Errorf("bottom: tooltip %v, trigger %v: want it 6 px below", tip, btn)
	}
	app.Hover(telatest.ByTestID("print"))
	app.Advance(300 * time.Millisecond)
	tip, btn = app.Bounds(telatest.ByTestID("tip-print")), app.Bounds(telatest.ByTestID("print"))
	if !near(tip.X, btn.X+btn.Width) || !near(tip.Y, btn.Y) {
		t.Errorf("right start: tooltip %v, trigger %v: want it to the right, top aligned", tip, btn)
	}
}

func TestTooltipArrowPointsAtTheTrigger(t *testing.T) {
	// The tooltip is opened by its state, so the pixels are the same as in
	// a golden: the arrow is the foreground color under the tooltip, at the
	// trigger's center, and the page's background beside it.
	open := true
	app := mountTips(t, kit.Tooltip{Open: &open}, kit.Tooltip{}, telatest.RootClass("bg-background text-foreground"))
	app.Advance(300 * time.Millisecond)
	tip, btn := app.Bounds(telatest.ByTestID("tip-save")), app.Bounds(telatest.ByTestID("save"))
	cx := btn.X + btn.Width/2
	fg := app.PixelAt(tip.X+tip.Width/2, tip.Y+3)
	if (fg == color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("the tooltip is white: %v", fg)
	}
	if got := app.PixelAt(cx, tip.Y+tip.Height+2); got != fg {
		t.Errorf("under the tooltip at the trigger's center: %v, want the tooltip's color %v (the arrow)", got, fg)
	}
	if got := app.PixelAt(cx+12, tip.Y+tip.Height+2); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("beside the arrow: %v, want the page's white", got)
	}
}

func TestTooltipWithTriggerFuncAndStyle(t *testing.T) {
	s := kit.DefaultStyle()
	s.Tooltip.Root += " ring-2"
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.StyleProvider{Style: &s, Children: kit.Tooltip{
			Trigger: kit.TriggerFunc(func(p tela.Prop) tela.Node {
				return h.Button(p, h.Text("Help"))
			}),
			Content: h.Text("Opens the manual"), DefaultOpen: true}})
	}, telatest.Theme(kit.Theme()))
	app.ExpectCount(telatest.ByRole("tooltip", "Opens the manual"), 1)
	if n := app.Count(telatest.ByClass("ring-2")); n != 1 {
		t.Errorf("the tooltip has the provider's classes %d times, want 1", n)
	}
}

func TestTooltipBadAlignPanics(t *testing.T) {
	_, err := telatest.AriaSnapshot(tela.C(tipPage(kit.Tooltip{Align: "left"}, kit.Tooltip{})))
	if err == nil || !strings.Contains(err.Error(), `Tooltip.Align "left"`) {
		t.Fatalf("error %v, want the alignment reported", err)
	}
}

func TestTooltipOpenGolden(t *testing.T) {
	// The tooltips of a window are a group: only one is open at a time.
	for _, tc := range []struct {
		name string
		tip  kit.Tooltip
	}{
		{"tooltip", kit.Tooltip{Content: h.Text("Add to library")}},
		{"tooltip_bottom", kit.Tooltip{Content: h.Text("Shows under the button"), Placement: "bottom", Offset: 4}},
	} {
		page := func() tela.Node {
			tip := tc.tip
			tip.Trigger, tip.DefaultOpen = kit.Button{Variant: kit.Outline, Children: h.Text("Hover me")}, true
			return h.Div(h.Class("flex flex-col items-start gap-4 p-6 pt-16"), tip)
		}
		openGolden(t, tc.name, 360, 160, page, nil)
	}
}

func TestTooltipIdle(t *testing.T) {
	expectIdle(t, func() tela.Node {
		return h.Div(h.Class("p-16"), kit.Tooltip{Trigger: kit.Button{Children: h.Text("Hover me")}, Content: h.Text("Add to library"), DefaultOpen: true})
	}, nil)
}
