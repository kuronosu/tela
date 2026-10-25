package kit_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// shadcn/ui's PopoverContent (new-york-v4/ui/popover.tsx at
// kit.ShadcnCommit), verbatim.
const shadcnPopoverContent = "z-50 w-72 origin-(--radix-popover-content-transform-origin) rounded-md border bg-popover p-4 text-popover-foreground shadow-md outline-hidden data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95"

// dimensions is the content of shadcn's popover demo: a heading, a line and
// two rows of a label and an input.
func dimensions() part {
	row := func(label, value string) part {
		return part{
			c: h.Div(h.Class("grid grid-cols-3 items-center gap-4"),
				h.Div(h.Class(kit.DefaultStyle().Label.Root), h.Text(label)),
				kit.Input{DefaultValue: value, Class: "col-span-2 h-8"}),
			html: fmt.Sprintf(`<div class="grid grid-cols-3 items-center gap-4"><label class="%s">%s</label><input value=%q class="%s"></div>`,
				shadcnLabel, label, value, cn(shadcnInput, "col-span-2 h-8")),
		}
	}
	w, mh := row("Width", "100%"), row("Max. height", "none")
	return part{
		c: h.Div(h.Class("grid gap-4"),
			h.Div(h.Class("space-y-2"), h.H4(h.Class("leading-none font-medium"), h.Text("Dimensions")),
				h.P(h.Class("text-sm text-muted-foreground"), h.Text("Set the dimensions for the layer."))),
			h.Div(h.Class("grid gap-2"), w.c, mh.c)),
		html: `<div class="grid gap-4"><div class="space-y-2"><h4 class="leading-none font-medium">Dimensions</h4><p class="text-sm text-muted-foreground">Set the dimensions for the layer.</p></div><div class="grid gap-2">` +
			w.html + mh.html + `</div></div>`,
	}
}

func popoverCell(name string, p kit.Popover, body part) cell {
	p.Props = h.TestID(name + "-panel")
	p.Children = body.c
	return cell{
		name: name, width: 320, probes: []string{name + "-panel"}, tela: kit.StaticPopover(p),
		html: fmt.Sprintf(`<div role="dialog" data-probe="%s-panel" data-slot="popover-content" class="%s">%s</div>`,
			name, cn(staticClasses(shadcnPopoverContent), p.Class), body.html),
	}
}

func popoverSheet() *sheet {
	dim := dimensions()
	text := part{c: h.P(h.Class("text-sm"), h.Text("Place content for the popover here.")),
		html: `<p class="text-sm">Place content for the popover here.</p>`}
	return &sheet{name: "popover", cells: []cell{
		popoverCell("dimensions", kit.Popover{}, dim),
		popoverCell("text", kit.Popover{}, text),
		popoverCell("class", kit.Popover{Class: "w-48"}, text),
	}}
}

func TestPopoverGolden(t *testing.T)   { popoverSheet().golden(t) }
func TestChromiumPopover(t *testing.T) { popoverSheet().chromium(t) }

var (
	dimsButton = telatest.ByRole("button", "Dimensions")
	dimsPanel  = telatest.ByRole("dialog", "Dimensions")
)

// dimsPopover is the popover of the behavior tests.
func dimsPopover(p kit.Popover) kit.Popover {
	if p.Trigger == nil {
		p.Trigger = kit.Button{Variant: kit.Outline, Children: h.Text("Dimensions")}
	}
	p.AriaLabel = "Dimensions"
	p.Children = h.Div(h.Class("grid gap-3"), h.P(h.Text("Set the dimensions.")),
		kit.Input{Placeholder: "Width", AutoFocus: true})
	return p
}

// anchored is a page with a trigger at (x, y) and the popover; the trigger
// is 96x36.
func anchored(x, y float32, p kit.Popover) func() tela.Node {
	return func() tela.Node {
		return h.Div(h.Class("relative h-full w-full"),
			h.Div(h.Style(tela.Left(x), tela.Top(y)), h.Class("absolute"), dimsPopover(p)),
			h.Button(h.Class("absolute right-6 bottom-6"), h.Text("Elsewhere")))
	}
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 0.51 }

func TestPopoverOpenAndClose(t *testing.T) {
	page := pageUnder(dimsPopover(kit.Popover{}))
	app := telatest.MountFunc(t, page, telatest.Theme(kit.Theme()))
	app.ExpectCount(telatest.ByRole("dialog", ""), 0)
	app.Expect(dimsButton, telatest.HasPopup("dialog"), telatest.Expanded(false))

	app.Click(dimsButton)
	app.ExpectCount(dimsPanel, 1)
	app.Expect(dimsButton, telatest.Expanded(true))
	// Not modal: the page stays in the aria tree, and the popover is after it.
	app.ExpectCount(telatest.ByRole("heading", "Project settings"), 1)
	// The focus went in (the AutoFocus field).
	app.ExpectFocused(telatest.ByRole("textbox", "Width"))

	// Escape closes it, the focus goes back to the trigger.
	app.Press("Escape")
	app.ExpectCount(dimsPanel, 0)
	app.ExpectFocused(dimsButton)

	// The trigger toggles it, and so does the keyboard.
	app.Click(dimsButton)
	app.ExpectCount(dimsPanel, 1)
	app.Click(dimsButton)
	app.ExpectCount(dimsPanel, 0)
	app.Focus(dimsButton)
	app.Press("Space")
	app.ExpectCount(dimsPanel, 1)
	app.Press("Escape")
	app.ExpectCount(dimsPanel, 0)
	app.ExpectFocused(dimsButton)
}

func TestPopoverClosesOnOutsidePress(t *testing.T) {
	clicked := 0
	page := pageUnder(dimsPopover(kit.Popover{}), kit.Button{Class: "fixed top-6 right-6", Children: h.Text("Other"), OnClick: func() { clicked++ }})
	app := telatest.MountFunc(t, page, telatest.Theme(kit.Theme()))
	app.Click(dimsButton)
	app.ExpectCount(dimsPanel, 1)
	// A press inside does not close it.
	app.Click(telatest.ByText("Set the dimensions."))
	app.ExpectCount(dimsPanel, 1)
	// A press outside closes it (and is not delivered, as in React Aria).
	app.Click(telatest.ByRole("button", "Other"))
	app.ExpectCount(dimsPanel, 0)
	if clicked != 0 {
		t.Errorf("the press that closed the popover also clicked the button under it (%d)", clicked)
	}
	app.Click(telatest.ByRole("button", "Other"))
	if clicked != 1 {
		t.Errorf("the page is not usable after the popover closed (%d clicks)", clicked)
	}
}

func TestPopoverTabLeavesAndCloses(t *testing.T) {
	// Not modal: Tab can leave the panel, and when the focus lands on another
	// element the popover closes.
	app := telatest.MountFunc(t, pageUnder(dimsPopover(kit.Popover{})), telatest.Theme(kit.Theme()))
	app.Focus(dimsButton)
	app.Press("Enter")
	app.ExpectCount(dimsPanel, 1)
	app.Press("Tab")
	app.ExpectCount(dimsPanel, 0)
}

func TestPopoverPlacement(t *testing.T) {
	const w, hh = 800, 600
	run := func(name string, x, y float32, p kit.Popover, check func(trigger, panel tela.Rect)) {
		t.Run(name, func(t *testing.T) {
			app := telatest.MountFunc(t, anchored(x, y, p), telatest.Size(w, hh), telatest.Theme(kit.Theme()))
			app.Click(dimsButton)
			check(app.Bounds(dimsButton), app.Bounds(dimsPanel))
		})
	}
	run("bottom, centered, 4 px away", 300, 200, kit.Popover{}, func(tr, p tela.Rect) {
		if !near(p.Y, tr.Y+tr.Height+4) || !near(p.X+p.Width/2, tr.X+tr.Width/2) || p.Width != 288 {
			t.Errorf("trigger %v panel %v: want it 4 px below, centered, 288 wide", tr, p)
		}
	})
	run("bottom start", 300, 200, kit.Popover{Align: "start"}, func(tr, p tela.Rect) {
		if !near(p.X, tr.X) || !near(p.Y, tr.Y+tr.Height+4) {
			t.Errorf("trigger %v panel %v: want it below, at the trigger's left edge", tr, p)
		}
	})
	run("bottom end", 300, 200, kit.Popover{Align: "end"}, func(tr, p tela.Rect) {
		if !near(p.X+p.Width, tr.X+tr.Width) {
			t.Errorf("trigger %v panel %v: want it below, at the trigger's right edge", tr, p)
		}
	})
	run("the placement carries its alignment", 300, 200, kit.Popover{Placement: "bottom-start"}, func(tr, p tela.Rect) {
		if !near(p.X, tr.X) {
			t.Errorf("trigger %v panel %v: want it at the trigger's left edge", tr, p)
		}
	})
	run("top", 300, 300, kit.Popover{Placement: "top"}, func(tr, p tela.Rect) {
		if !near(p.Y+p.Height+4, tr.Y) {
			t.Errorf("trigger %v panel %v: want it 4 px above", tr, p)
		}
	})
	run("right", 300, 200, kit.Popover{Placement: "right", Align: "start"}, func(tr, p tela.Rect) {
		if !near(p.X, tr.X+tr.Width+4) || !near(p.Y, tr.Y) {
			t.Errorf("trigger %v panel %v: want it 4 px to the right, top aligned", tr, p)
		}
	})
	run("no offset", 300, 200, kit.Popover{Offset: kit.NoOffset}, func(tr, p tela.Rect) {
		if !near(p.Y, tr.Y+tr.Height) {
			t.Errorf("trigger %v panel %v: want it against the trigger", tr, p)
		}
	})
	run("offset", 300, 200, kit.Popover{Offset: 12}, func(tr, p tela.Rect) {
		if !near(p.Y, tr.Y+tr.Height+12) {
			t.Errorf("trigger %v panel %v: want it 12 px below", tr, p)
		}
	})
	run("flips to the top when there is no room below", 300, 540, kit.Popover{}, func(tr, p tela.Rect) {
		if !near(p.Y+p.Height+4, tr.Y) {
			t.Errorf("trigger %v panel %v: want it flipped above", tr, p)
		}
	})
	run("shifts to stay in the window", 10, 200, kit.Popover{}, func(tr, p tela.Rect) {
		if p.X < 12-0.5 || p.X+p.Width > w {
			t.Errorf("trigger %v panel %v: want it kept inside the window", tr, p)
		}
	})
}

func TestPopoverBadAlignPanics(t *testing.T) {
	_, err := telatest.AriaSnapshot(tela.C(pageUnder(dimsPopover(kit.Popover{Align: "midle"}))))
	if err == nil || !strings.Contains(err.Error(), `Popover.Align "midle"`) {
		t.Fatalf("error %v, want the alignment reported", err)
	}
}

func TestPopoverModal(t *testing.T) {
	app := telatest.MountFunc(t, pageUnder(dimsPopover(kit.Popover{Modal: true})), telatest.Theme(kit.Theme()))
	app.Click(dimsButton)
	app.ExpectCount(dimsPanel, 1)
	app.ExpectCount(telatest.ByRole("heading", "Project settings"), 0)
}

func TestPopoverControlled(t *testing.T) {
	var log []bool
	open := true
	app := telatest.MountFunc(t, pageUnder(dimsPopover(kit.Popover{Open: &open, OnOpenChange: func(o bool) { log = append(log, o) }})),
		telatest.Theme(kit.Theme()))
	app.ExpectCount(dimsPanel, 1)
	app.Press("Escape")
	app.ExpectCount(dimsPanel, 1) // the parent did not close it
	if fmt.Sprint(log) != "[false]" {
		t.Errorf("OnOpenChange got %v, want [false]", log)
	}
}

func TestPopoverTriggerFunc(t *testing.T) {
	avatar := kit.TriggerFunc(func(p tela.Prop) tela.Node {
		return h.Button(p, h.AriaLabel("Account"), h.Class("rounded-full"), kit.Avatar{Fallback: "AB"})
	})
	app := telatest.MountFunc(t, pageUnder(dimsPopover(kit.Popover{Trigger: avatar})), telatest.Theme(kit.Theme()))
	app.Click(telatest.ByRole("button", "Account"))
	app.ExpectCount(dimsPanel, 1)
}

func TestPopoverStyleProvider(t *testing.T) {
	s := kit.DefaultStyle()
	s.Popover.Root += " ring-4"
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.StyleProvider{Style: &s, Children: tela.C(pageUnder(dimsPopover(kit.Popover{DefaultOpen: true})))})
	}, telatest.Theme(kit.Theme()))
	if n := app.Count(telatest.ByClass("ring-4")); n != 1 {
		t.Errorf("the popover has the provider's classes %d times, want 1", n)
	}
}

func TestPopoverOpenGolden(t *testing.T) {
	page := pageUnder(h.Div(h.Class("self-center"), kit.Popover{
		Trigger: kit.Button{Variant: kit.Outline, Children: h.Text("Open popover")}, AriaLabel: "Dimensions",
		Children: dimensions().c,
	}))
	openGolden(t, "popover", 640, 460, page, func(a *telatest.App) { a.Click(telatest.ByRole("button", "Open popover")) })
}

func TestPopoverIdle(t *testing.T) {
	expectIdle(t, pageUnder(kit.Popover{DefaultOpen: true, AriaLabel: "Dimensions",
		Trigger:  kit.Button{Children: h.Text("Dimensions")},
		Children: h.P(h.Text("No fields, no caret."))}), nil)
}
