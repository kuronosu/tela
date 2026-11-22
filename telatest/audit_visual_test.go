package telatest_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// The rules about what is on screen (ADR-312): color-contrast, text-clipped,
// text-overlap, target-overlap, click-events-have-key-events, focus-visible.

func audit(t *testing.T, root func() tela.Node, w, h float32, rule string, opts ...telatest.Option) []telatest.Violation {
	t.Helper()
	app := telatest.MountFunc(t, root, append([]telatest.Option{telatest.Size(w, h), telatest.Instrument()}, opts...)...)
	return app.Audit(telatest.WithRules(rule))
}

func TestColorContrast(t *testing.T) {
	t.Parallel()
	page := func(class string) func() tela.Node {
		return func() tela.Node {
			return Div(Class("bg-white p-4"), P(Class(class), Text("Quarterly revenue")))
		}
	}
	vs := audit(t, page("text-sm text-zinc-400"), 300, 100, "color-contrast")
	if len(vs) != 1 {
		t.Fatalf("zinc-400 on white: %v", vs)
	}
	v := vs[0]
	for _, want := range []string{"2.6", "4.5:1", "#9f9fa9", "#ffffff"} {
		if !strings.Contains(v.Message, want) {
			t.Errorf("message %q lacks %q", v.Message, want)
		}
	}
	if !strings.Contains(v.Element, "Quarterly revenue") || !strings.HasPrefix(v.Source, "audit_visual_test.go:") {
		t.Errorf("element %q source %q", v.Element, v.Source)
	}
	if vs := audit(t, page("text-sm text-zinc-900"), 300, 100, "color-contrast"); len(vs) != 0 {
		t.Errorf("zinc-900 on white: %v", vs)
	}
	// Large text needs 3:1: #8a8a8a on white is 3.45:1.
	if vs := audit(t, page("text-sm text-[#8a8a8a]"), 300, 100, "color-contrast"); len(vs) != 1 {
		t.Errorf("small gray text: %v", vs)
	}
	if vs := audit(t, page("text-3xl text-[#8a8a8a]"), 300, 100, "color-contrast"); len(vs) != 0 {
		t.Errorf("large gray text: %v", vs)
	}
	// The text's own opacity counts...
	if vs := audit(t, page("opacity-80 text-zinc-500"), 300, 100, "color-contrast"); len(vs) != 1 {
		t.Errorf("faded text: %v", vs)
	}
	// ...but not the half-faded look of an inactive control (the label of a
	// disabled checkbox, which the tree does not tie to the control).
	if vs := audit(t, page("opacity-50 text-black"), 300, 100, "color-contrast"); len(vs) != 0 {
		t.Errorf("inactive look: %v", vs)
	}
}

func TestColorContrastDarkAndBackgrounds(t *testing.T) {
	t.Parallel()
	dark := func() tela.Node {
		return Div(Class("bg-white p-4 dark:bg-zinc-900"), P(Class("text-zinc-300 dark:text-zinc-600"), Text("Hello")))
	}
	if vs := audit(t, dark, 300, 100, "color-contrast"); len(vs) != 1 {
		t.Errorf("light: %v", vs) // zinc-300 on white
	}
	if vs := audit(t, dark, 300, 100, "color-contrast", telatest.Dark()); len(vs) != 1 {
		t.Errorf("dark: %v", vs) // zinc-600 on zinc-900
	}
	// A gradient or an image behind the text: not judged.
	grad := func() tela.Node {
		return Div(Class("bg-linear-to-r from-black to-white p-4"), P(Class("text-zinc-500"), Text("Hello")))
	}
	if vs := audit(t, grad, 300, 100, "color-contrast"); len(vs) != 0 {
		t.Errorf("gradient: %v", vs)
	}
	// The nearest opaque background wins over a gradient behind it.
	card := func() tela.Node {
		return Div(Class("bg-linear-to-r from-black to-white p-4"), Div(Class("bg-white p-2"), P(Class("text-zinc-400"), Text("Hello"))))
	}
	if vs := audit(t, card, 300, 100, "color-contrast"); len(vs) != 1 {
		t.Errorf("card on a gradient: %v", vs)
	}
	// Disabled controls are exempt.
	dis := func() tela.Node {
		return Div(Class("bg-white p-4"), Button(Class("h-9 px-4 text-zinc-300"), Disabled(true), Text("Save")))
	}
	if vs := audit(t, dis, 300, 100, "color-contrast"); len(vs) != 0 {
		t.Errorf("disabled: %v", vs)
	}
}

func TestTextClipped(t *testing.T) {
	t.Parallel()
	const long = "A very long caption that does not fit"
	box := func(class string) func() tela.Node {
		return func() tela.Node {
			return Div(Class("p-4"), Div(Class(class), Text(long)))
		}
	}
	vs := audit(t, box("w-20 overflow-hidden whitespace-nowrap"), 400, 300, "text-clipped")
	if len(vs) != 1 {
		t.Fatalf("overflow-hidden nowrap: %v", vs)
	}
	for _, want := range []string{"clipped", "overflow-hidden"} {
		if !strings.Contains(vs[0].Message, want) {
			t.Errorf("message %q lacks %q", vs[0].Message, want)
		}
	}
	if !strings.Contains(vs[0].Element, "A very long caption") {
		t.Errorf("element %q", vs[0].Element)
	}
	for _, ok := range []string{"w-20 truncate", "w-20 overflow-x-auto whitespace-nowrap", "w-20 overflow-hidden", "w-20 line-clamp-1"} {
		if vs := audit(t, box(ok), 400, 300, "text-clipped"); len(vs) != 0 {
			t.Errorf("%q: %v", ok, vs)
		}
	}
	// A box too short for its lines.
	if vs := audit(t, box("h-4 w-20 overflow-hidden"), 400, 300, "text-clipped"); len(vs) != 1 {
		t.Errorf("h-4 overflow-hidden: %v", vs)
	}
	// Text that runs past the window edge.
	if vs := audit(t, box("whitespace-nowrap"), 120, 100, "text-clipped"); len(vs) != 1 || !strings.Contains(vs[0].Message, "window") {
		t.Errorf("past the window: %v", vs)
	}
	// Visually hidden text (sr-only) is clipped on purpose.
	if vs := audit(t, func() tela.Node { return Div(Class("p-4"), Span(Class("sr-only"), Text(long))) }, 400, 100, "text-clipped"); len(vs) != 0 {
		t.Errorf("sr-only: %v", vs)
	}
}

func TestTextOverlap(t *testing.T) {
	t.Parallel()
	// The first block is too short for its three lines: they run onto the
	// next one.
	over := func() tela.Node {
		return Div(Class("w-24 p-2"),
			P(Class("h-4 text-sm"), Text("one two three four five six seven")),
			P(Class("text-sm"), Text("Next paragraph")))
	}
	vs := audit(t, over, 300, 200, "text-overlap")
	if len(vs) != 1 || !strings.Contains(vs[0].Message, "Next paragraph") {
		t.Fatalf("overlapping text: %v", vs)
	}
	fine := func() tela.Node {
		return Div(Class("w-24 p-2"),
			P(Class("text-sm"), Text("one two three four five six seven")),
			P(Class("text-sm"), Text("Next paragraph")))
	}
	if vs := audit(t, fine, 300, 200, "text-overlap"); len(vs) != 0 {
		t.Errorf("stacked text: %v", vs)
	}
	// A badge over an avatar is an overlay, not an overlap.
	badge := func() tela.Node {
		return Div(Class("relative size-10 p-1"), P(Class("text-sm"), Text("AB")), Span(Class("absolute top-1 left-1 text-xs"), Text("9")))
	}
	if vs := audit(t, badge, 300, 200, "text-overlap"); len(vs) != 0 {
		t.Errorf("overlay: %v", vs)
	}
}

func TestTargetOverlap(t *testing.T) {
	t.Parallel()
	row := func(second string) func() tela.Node {
		return func() tela.Node {
			return Div(Class("flex p-4"),
				Button(Class("h-9 w-24 bg-stone-200"), Text("Cancel")),
				Button(Class("h-9 w-24 bg-stone-900 text-white "+second), Text("Save")))
		}
	}
	vs := audit(t, row("-ml-12"), 400, 100, "target-overlap")
	if len(vs) != 1 || !strings.Contains(vs[0].Message, `button "Save"`) || !strings.Contains(vs[0].Element, `button "Cancel"`) {
		t.Fatalf("overlapping buttons: %v", vs)
	}
	if vs := audit(t, row("ml-2"), 400, 100, "target-overlap"); len(vs) != 0 {
		t.Errorf("apart: %v", vs)
	}
}

func TestClickEventsHaveKeyEvents(t *testing.T) {
	t.Parallel()
	vs := audit(t, func() tela.Node {
		return Div(Class("p-4"), Div(Class("h-9 w-24 bg-stone-200"), OnClick(func(tela.MouseEvent) {}), Text("Open")))
	}, 400, 100, "click-events-have-key-events")
	if len(vs) != 1 || !strings.Contains(vs[0].Fix, "Button") {
		t.Fatalf("a div with a click: %v", vs)
	}
	for name, root := range map[string]func() tela.Node{
		"button": func() tela.Node {
			return Div(Class("p-4"), Button(Class("h-9 w-24"), OnClick(func(tela.MouseEvent) {}), Text("Open")))
		},
		"tabindex": func() tela.Node {
			return Div(Class("p-4"), Div(Class("h-9 w-24"), Role("button"), TabIndex(0), OnClick(func(tela.MouseEvent) {}), Text("Open")))
		},
		"inside a button": func() tela.Node {
			return Div(Class("p-4"), Button(Class("h-9 w-24"), Div(OnClick(func(tela.MouseEvent) {}), Text("Open"))))
		},
		"a card with a button": func() tela.Node {
			return Div(Class("p-4"), Div(Class("h-20 w-40"), OnClick(func(tela.MouseEvent) {}), Button(Class("h-9 w-24"), Text("Open"))))
		},
		"a backdrop": func() tela.Node {
			return Div(Class("fixed inset-0"), OnClick(func(tela.MouseEvent) {}))
		},
	} {
		if vs := audit(t, root, 400, 100, "click-events-have-key-events"); len(vs) != 0 {
			t.Errorf("%s: %v", name, vs)
		}
	}
}

func TestFocusVisible(t *testing.T) {
	t.Parallel()
	row := func(class string) func() tela.Node {
		return func() tela.Node {
			return Div(Class("flex gap-4 p-4"),
				Button(Class("h-9 px-4 bg-stone-200 "+class), Text("Cancel")),
				Button(Class("h-9 px-4 bg-stone-900 text-white "+class), Text("Save")))
		}
	}
	vs := audit(t, row("outline-none"), 400, 100, "focus-visible")
	if len(vs) != 2 || !strings.Contains(vs[0].Message, "Tab") || !strings.Contains(vs[0].Fix, "focus-visible:") {
		t.Fatalf("buttons without a ring: %v", vs)
	}
	if vs := audit(t, row("outline-none focus-visible:ring-2 focus-visible:ring-blue-500"), 400, 100, "focus-visible"); len(vs) != 0 {
		t.Errorf("with a ring: %v", vs)
	}
	// A text field shows the focus with its caret (as a browser's does, and
	// WCAG counts it), even with no ring.
	field := func() tela.Node {
		return Div(Class("p-4"), Input(Class("h-9 w-40 outline-none"), Type("text"), AriaLabel("Name")))
	}
	if vs := audit(t, field, 400, 100, "focus-visible"); len(vs) != 0 {
		t.Errorf("a field without a ring: %v", vs)
	}
	// The audit leaves the focus where it found it.
	app := telatest.MountFunc(t, row("outline-none"), telatest.Size(400, 100))
	app.Focus(telatest.ByRole("button", "Save"))
	app.Audit(telatest.WithRules("focus-visible"))
	app.ExpectFocused(telatest.ByRole("button", "Save"))
}

func TestDefaultRulesExcludeLayoutAndFocus(t *testing.T) {
	t.Parallel()
	def := map[string]bool{}
	for _, r := range telatest.Rules() {
		def[r.ID] = r.Default
	}
	for _, id := range []string{"color-contrast", "click-events-have-key-events"} {
		if !def[id] {
			t.Errorf("%s is not default", id)
		}
	}
	for _, id := range []string{"focus-visible", "text-clipped", "text-overlap", "target-overlap"} {
		if d, ok := def[id]; !ok || d {
			t.Errorf("%s: present %v, default %v", id, ok, d)
		}
	}
}

// Content that scrolls under a bar is not under it until it is scrolled
// there: the part of a scroll container that is out of sight lies over
// nothing.
func TestTextOverlapIgnoresScrolledOutContent(t *testing.T) {
	t.Parallel()
	page := func() tela.Node {
		return Div(Class("flex h-full flex-col"),
			Div(Class("flex-1 overflow-y-auto"),
				Div(Class("h-[72px]")),
				P(Class("text-sm"), Text("Far below"))),
			Div(Class("h-8 shrink-0"), P(Class("text-sm"), Text("Footer"))))
	}
	if vs := audit(t, page, 300, 100, "text-overlap"); len(vs) != 0 {
		t.Errorf("scrolled-out text: %v", vs)
	}
	// Two texts in the same scroll container that run onto each other are
	// an overlap wherever they are.
	inside := func() tela.Node {
		return Div(Class("h-full overflow-y-auto"),
			Div(Class("h-[400px]")),
			Div(Class("w-24 p-2"),
				P(Class("h-4 text-sm"), Text("one two three four five six seven")),
				P(Class("text-sm"), Text("Next paragraph"))))
	}
	if vs := audit(t, inside, 300, 100, "text-overlap"); len(vs) != 1 {
		t.Errorf("overlap below the fold: %v", vs)
	}
}

// A paragraph that holds only an inline block (a badge in a card's action
// slot) has no text of its own: the badge's text is what the rules judge.
func TestAuditSkipsTextlessParagraphs(t *testing.T) {
	t.Parallel()
	page := func() tela.Node {
		return Div(Class("w-20 overflow-hidden p-2"),
			Div(Class("w-8"), Span(Class("inline-block w-40 rounded bg-stone-200"), Text("x"))))
	}
	for _, v := range audit(t, page, 300, 100, "text-clipped") {
		if strings.Contains(v.Element, `text ""`) {
			t.Errorf("a violation on empty text: %v", v)
		}
	}
}
