package style

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/text"
)

func TestFieldKindDefaults(t *testing.T) {
	cx := &Context{}
	parent := Compute(cx, Input{Kind: KindDiv, Classes: []*Classes{mustCompile(t, "text-center tracking-wide")}})
	in := Compute(cx, Input{Kind: KindInput, Parent: parent})
	eq(t, "input display", in.Display, DisplayInlineBlock)
	eq(t, "input cursor", in.Cursor, CursorText)
	eq(t, "input align", in.Text.TextAlign, text.AlignStart)
	eq(t, "input letter spacing (inherited)", in.Text.LetterSpacing, parent.Text.LetterSpacing)
	eq(t, "input overflow", in.OverflowX, OverflowClip)
	ta := Compute(cx, Input{Kind: KindTextarea, Parent: parent})
	eq(t, "textarea overflow", ta.OverflowY, OverflowAuto)
	eq(t, "textarea white-space", ta.Text.WhiteSpace, text.WhiteSpacePreWrap)
	eq(t, "textarea wrap", ta.Text.OverflowWrap, text.OverflowWrapBreakWord)
	eq(t, "form display", Compute(cx, Input{Kind: KindForm, Parent: parent}).Display, DisplayBlock)
	lab := Compute(cx, Input{Kind: KindLabel, Parent: parent})
	eq(t, "label display", lab.Display, DisplayInline)
	eq(t, "label cursor", lab.Cursor, CursorDefault)
	for _, k := range []Kind{KindInput, KindTextarea} {
		if !k.IsTextField() {
			t.Errorf("%v is not a text field", k)
		}
	}
	if KindButton.IsTextField() {
		t.Error("button is a text field")
	}
}

func TestCaretAndFieldSizing(t *testing.T) {
	cx := &Context{}
	th := DefaultTheme()
	s := Compute(cx, Input{Kind: KindInput, Classes: []*Classes{mustCompile(t, "caret-pink-500 field-sizing-content")}})
	if !s.Caret.Set || s.Caret.Color != th.Colors["pink-500"] {
		t.Errorf("caret = %+v", s.Caret)
	}
	eq(t, "field-sizing", s.FieldSizing, FieldSizingContent)
	// caret-color inherits; field-sizing does not.
	parent := Compute(cx, Input{Kind: KindDiv, Classes: []*Classes{mustCompile(t, "caret-transparent field-sizing-content")}})
	kid := Compute(cx, Input{Kind: KindInput, Parent: parent})
	if !kid.Caret.Set || kid.Caret.Color != gfx.Transparent {
		t.Errorf("inherited caret = %+v", kid.Caret)
	}
	eq(t, "field-sizing not inherited", kid.FieldSizing, FieldSizingFixed)
	if c := Compute(cx, Input{Kind: KindInput}); c.Caret.Set {
		t.Error("caret is not auto by default")
	}
}

func TestPseudoVariants(t *testing.T) {
	cx := &Context{}
	th := DefaultTheme()
	cl := mustCompile(t, "text-slate-900 placeholder:text-slate-400 placeholder:italic selection:bg-pink-300 focus:placeholder:text-red-500")
	s := Compute(cx, Input{Kind: KindInput, Classes: []*Classes{cl}})
	if s.Text.Color != th.Colors["slate-900"] {
		t.Errorf("text color %v: a pseudo rule leaked into the element", s.Text.Color)
	}
	if s.Placeholder == nil || s.Placeholder.Color != th.Colors["slate-400"] || s.Placeholder.FontStyle != text.StyleItalic {
		t.Fatalf("placeholder = %+v", s.Placeholder)
	}
	if s.Selection == nil || !s.Selection.HasBackground || s.Selection.Background != th.Colors["pink-300"] || s.Selection.HasColor {
		t.Errorf("selection = %+v", s.Selection)
	}
	f := Compute(cx, Input{Kind: KindInput, Classes: []*Classes{cl}, State: StateFocus})
	if f.Placeholder == nil || f.Placeholder.Color != th.Colors["red-500"] {
		t.Errorf("focused placeholder = %+v", f.Placeholder)
	}
	// Without placeholder: classes there is no placeholder style; the
	// selection colors are inherited by descendants.
	plain := Compute(cx, Input{Kind: KindInput, Parent: s})
	if plain.Placeholder != nil {
		t.Error("placeholder style without placeholder: classes")
	}
	if plain.Selection != s.Selection {
		t.Error("selection colors are not inherited")
	}
	// A child's own selection: classes keep the inherited colors they do
	// not set.
	own := Compute(cx, Input{Kind: KindDiv, Parent: s, Classes: []*Classes{mustCompile(t, "selection:text-white")}})
	if own.Selection == nil || !own.Selection.HasBackground || !own.Selection.HasColor || own.Selection.Color != gfx.White {
		t.Errorf("own selection = %+v", own.Selection)
	}
}

func TestFieldStateVariants(t *testing.T) {
	cx := &Context{}
	th := DefaultTheme()
	cl := mustCompile(t, "read-only:bg-slate-100 placeholder-shown:bg-red-100 invalid:border-red-500 peer-invalid:block group-placeholder-shown:p-2")
	if cl.Err != nil {
		t.Fatal(cl.Err)
	}
	s := Compute(cx, Input{Kind: KindInput, Classes: []*Classes{cl}, State: StateReadOnly | StateInvalid})
	if s.BackgroundColor != th.Colors["slate-100"] || s.Border.Top.Color != th.Colors["red-500"] {
		t.Errorf("read-only/invalid not applied: %v %v", s.BackgroundColor, s.Border.Top.Color)
	}
	s = Compute(cx, Input{Kind: KindInput, Classes: []*Classes{cl}, State: StatePlaceholderShown})
	eq(t, "placeholder-shown", s.BackgroundColor, th.Colors["red-100"])
}
