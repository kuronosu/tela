package kit_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// shadcn/ui's toggleVariants (new-york-v4/ui/toggle.tsx at kit.ShadcnCommit),
// verbatim.
const (
	shadcnToggle        = "inline-flex items-center justify-center gap-2 rounded-md text-sm font-medium whitespace-nowrap transition-[color,box-shadow] outline-none hover:bg-muted hover:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 data-[state=on]:bg-accent data-[state=on]:text-accent-foreground dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4"
	shadcnToggleDefault = "bg-transparent"
	shadcnToggleOutline = "border border-input bg-transparent shadow-xs hover:bg-accent hover:text-accent-foreground"
	shadcnToggleSizeDef = "h-9 min-w-9 px-2"
	shadcnToggleSizeSm  = "h-8 min-w-8 px-1.5"
	shadcnToggleSizeLg  = "h-10 min-w-10 px-2.5"
)

// toggleClasses is toggleVariants({variant, size}) as shadcn writes it.
func toggleClasses(v kit.Variant, s kit.Size) string {
	variant := shadcnToggleDefault
	if v == kit.Outline {
		variant = shadcnToggleOutline
	}
	size := map[kit.Size]string{"": shadcnToggleSizeDef, kit.Sm: shadcnToggleSizeSm, kit.Lg: shadcnToggleSizeLg}[s]
	return cn(shadcnToggle, variant, size)
}

// toggleHTML is the markup of shadcn's Toggle; inner is its content.
func toggleHTML(probe string, t kit.Toggle, inner string) string {
	state := "off"
	if t.DefaultPressed {
		state = "on"
	}
	attrs := fmt.Sprintf(`type="button" aria-pressed="%t" data-state=%q`, t.DefaultPressed, state)
	if t.Disabled {
		attrs += ` disabled data-disabled=""`
	}
	return fmt.Sprintf(`<button data-probe=%q %s data-slot="toggle" class="%s">%s</button>`,
		probe, attrs, cn(toggleClasses(t.Variant, t.Size), t.Class), inner)
}

// toggleCell is a Toggle on both sides: icon is its icon (or the zero
// Icon) and text its label.
func toggleCell(name string, t kit.Toggle, text string) cell {
	t.Props = h.TestID(name + "-toggle")
	inner := ""
	if t.Icon != (tela.Icon{}) {
		inner = svgHTML(t.Icon)
	}
	if text != "" {
		t.Children = h.Text(text)
		inner += text
	}
	return cell{name: name, probes: []string{name + "-toggle"}, tela: t, html: toggleHTML(name+"-toggle", t, inner)}
}

func toggleSheet() *sheet {
	return &sheet{name: "toggle", slot: 120, cells: []cell{
		toggleCell("default", kit.Toggle{Icon: iconBold, AriaLabel: "Bold"}, ""),
		toggleCell("default-on", kit.Toggle{Icon: iconBold, AriaLabel: "Bold", DefaultPressed: true}, ""),
		toggleCell("outline", kit.Toggle{Variant: kit.Outline, Icon: iconItalic, AriaLabel: "Italic"}, ""),
		toggleCell("outline-on", kit.Toggle{Variant: kit.Outline, Icon: iconItalic, AriaLabel: "Italic", DefaultPressed: true}, ""),
		toggleCell("disabled", kit.Toggle{Disabled: true, Icon: iconUnderline, AriaLabel: "Underline"}, ""),
		toggleCell("outline-disabled", kit.Toggle{Variant: kit.Outline, Disabled: true, Icon: iconUnderline, AriaLabel: "Underline"}, ""),
		toggleCell("text", kit.Toggle{}, "Italic"),
		toggleCell("text-on", kit.Toggle{DefaultPressed: true}, "Italic"),
		toggleCell("icon-text", kit.Toggle{Variant: kit.Outline, Icon: iconItalic}, "Italic"),
		toggleCell("sm", kit.Toggle{Size: kit.Sm, Variant: kit.Outline, Icon: iconBold, AriaLabel: "Bold"}, ""),
		toggleCell("lg", kit.Toggle{Size: kit.Lg, Variant: kit.Outline, Icon: iconBold, AriaLabel: "Bold"}, ""),
		toggleCell("sm-text", kit.Toggle{Size: kit.Sm}, "Small"),
		toggleCell("lg-text", kit.Toggle{Size: kit.Lg, DefaultPressed: true}, "Large"),
		toggleCell("class", kit.Toggle{Class: "rounded-full", DefaultPressed: true, Icon: iconBold, AriaLabel: "Bold"}, ""),
	}}
}

func TestToggleGolden(t *testing.T)   { toggleSheet().golden(t) }
func TestChromiumToggle(t *testing.T) { toggleSheet().chromium(t) }

func toggleButton(name string) telatest.Locator { return telatest.ByRole("button", name) }

func TestToggleUncontrolled(t *testing.T) {
	var got []bool
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(
			kit.Toggle{AriaLabel: "Bold", Children: h.Text("B"), OnPressedChange: func(b bool) { got = append(got, b) }},
			kit.Toggle{AriaLabel: "Italic", Children: h.Text("I"), DefaultPressed: true})
	})
	app.Expect(toggleButton("Bold"), telatest.Pressed(false))
	app.Expect(toggleButton("Italic"), telatest.Pressed(true))
	app.Click(toggleButton("Bold"))
	app.Expect(toggleButton("Bold"), telatest.Pressed(true))
	app.Focus(toggleButton("Bold"))
	app.Press("Space")
	app.Expect(toggleButton("Bold"), telatest.Pressed(false))
	app.Press("Enter") // unlike a checkbox, a toggle button is toggled by Enter
	app.Expect(toggleButton("Bold"), telatest.Pressed(true))
	if fmt.Sprint(got) != "[true false true]" {
		t.Errorf("OnPressedChange got %v, want [true false true]", got)
	}
}

// controlledToggle keeps its state in the caller.
type controlledToggle struct{}

func (controlledToggle) Build() tela.Node {
	on, setOn := tela.UseState(false)
	return h.Div(kit.Toggle{AriaLabel: "Mute", Pressed: &on, OnPressedChange: setOn.Set, Children: h.Text("Mute")},
		h.Span(h.Class("sr-only"), h.Text(fmt.Sprintf("muted is %t", on))))
}

func TestToggleControlled(t *testing.T) {
	app := telatest.Mount(t, controlledToggle{})
	app.ExpectText("muted is false")
	app.Click(toggleButton("Mute"))
	app.ExpectText("muted is true")
	app.Expect(toggleButton("Mute"), telatest.Pressed(true))
}

func TestToggleDisabledAndInvalid(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(kit.Toggle{AriaLabel: "Locked", Disabled: true}, kit.Toggle{AriaLabel: "Wrong", Invalid: true})
	})
	app.Expect(toggleButton("Locked"), telatest.Disabled(true))
	b := app.Bounds(toggleButton("Locked"))
	app.ClickAt(b.X+b.Width/2, b.Y+b.Height/2)
	app.Expect(toggleButton("Locked"), telatest.Pressed(false))
	app.Expect(toggleButton("Wrong"), telatest.Invalid(true))
}

func TestToggleAriaSnapshot(t *testing.T) {
	got, err := telatest.AriaSnapshot(h.Div(
		kit.Toggle{AriaLabel: "Bold", DefaultPressed: true},
		kit.Toggle{AriaLabel: "Italic"},
		kit.Toggle{AriaLabel: "Underline", Disabled: true}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`button "Bold" [pressed]`, `button "Italic"`, `button "Underline" [disabled]`} {
		if !strings.Contains(got, want) {
			t.Errorf("snapshot lacks %q:\n%s", want, got)
		}
	}
}

func TestToggleUndefinedVariantAndSize(t *testing.T) {
	for _, tc := range []struct {
		c    tela.Component
		want string
	}{
		{kit.Toggle{Variant: "outlin"}, `Toggle.Variant "outlin" is not defined by the active style, which defines Default, "outline"; did you mean "outline"?`},
		{kit.Toggle{Size: "xl"}, `Toggle.Size "xl"`},
	} {
		_, err := telatest.AriaSnapshot(tc.c)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("error %v, want %q", err, tc.want)
		}
	}
}

func ExampleToggle() {
	t := &exampleT{}
	defer t.close()
	bold := false
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.Toggle{Variant: kit.Outline, Icon: iconBold, AriaLabel: "Bold",
			OnPressedChange: func(on bool) { bold = on }})
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("button", "Bold"))
	fmt.Println("bold", bold)
	// Output: bold true
}
