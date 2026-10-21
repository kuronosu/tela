package style

import (
	"errors"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

func TestConditionMatches(t *testing.T) {
	tests := []struct {
		name string
		cond Condition
		ctx  VariantContext
		want bool
	}{
		{"zero always holds", Condition{}, VariantContext{}, true},
		{"hover needs hover", Condition{State: StateHover}, VariantContext{}, false},
		{"hover held", Condition{State: StateHover}, VariantContext{State: StateHover | StateFocus}, true},
		{"two states", Condition{State: StateHover | StateActive}, VariantContext{State: StateHover}, false},
		{"group", Condition{Group: StateHover}, VariantContext{Group: StateHover}, true},
		{"group is not self", Condition{Group: StateHover}, VariantContext{State: StateHover}, false},
		{"peer", Condition{Peer: StateChecked}, VariantContext{Peer: StateChecked}, true},
		{"peer missing", Condition{Peer: StateChecked}, VariantContext{}, false},
		{"enabled", Condition{Enabled: true}, VariantContext{}, true},
		{"enabled but disabled", Condition{Enabled: true}, VariantContext{State: StateDisabled}, false},
		{"dark off", Condition{Dark: true}, VariantContext{}, false},
		{"dark on", Condition{Dark: true}, VariantContext{Dark: true}, true},
		{"motion-reduce", Condition{MotionReduce: true}, VariantContext{ReducedMotion: true}, true},
		{"motion-reduce off", Condition{MotionReduce: true}, VariantContext{}, false},
		{"motion-safe", Condition{MotionSafe: true}, VariantContext{}, true},
		{"motion-safe reduced", Condition{MotionSafe: true}, VariantContext{ReducedMotion: true}, false},
		{"starting off", Condition{Starting: true}, VariantContext{}, false},
		{"starting on", Condition{Starting: true}, VariantContext{Starting: true}, true},
		{"plain in the starting style", Condition{}, VariantContext{Starting: true}, true},
		{"starting dark", Condition{Starting: true, Dark: true}, VariantContext{Starting: true}, false},
		{"rtl in ltr", Condition{RTL: true}, VariantContext{}, false},
		{"rtl in rtl", Condition{RTL: true}, VariantContext{RTL: true}, true},
		{"ltr in ltr", Condition{LTR: true}, VariantContext{}, true},
		{"ltr in rtl", Condition{LTR: true}, VariantContext{RTL: true}, false},
		{"breakpoint below", Condition{MinWidth: 768}, VariantContext{ViewportWidth: 767}, false},
		{"breakpoint at", Condition{MinWidth: 768}, VariantContext{ViewportWidth: 768}, true},
		{"stacked", Condition{Dark: true, State: StateHover, MinWidth: 640}, VariantContext{Dark: true, State: StateHover, ViewportWidth: 700}, true},
		{"stacked missing one", Condition{Dark: true, State: StateHover, MinWidth: 640}, VariantContext{Dark: true, State: StateHover, ViewportWidth: 500}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cond.Matches(tt.ctx); got != tt.want {
				t.Errorf("Matches = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVariantParsing(t *testing.T) {
	th := DefaultTheme()
	tests := []struct {
		variant string
		want    Condition
	}{
		{"hover", Condition{State: StateHover}},
		{"active", Condition{State: StateActive}},
		{"focus", Condition{State: StateFocus}},
		{"focus-visible", Condition{State: StateFocusVisible}},
		{"focus-within", Condition{State: StateFocusWithin}},
		{"disabled", Condition{State: StateDisabled}},
		{"checked", Condition{State: StateChecked}},
		{"enabled", Condition{Enabled: true}},
		{"first", Condition{State: StateFirst}},
		{"last", Condition{State: StateLast}},
		{"odd", Condition{State: StateOdd}},
		{"even", Condition{State: StateEven}},
		{"only", Condition{State: StateOnly}},
		{"group-hover", Condition{Group: StateHover}},
		{"group-focus-visible", Condition{Group: StateFocusVisible}},
		{"peer-checked", Condition{Peer: StateChecked}},
		{"dark", Condition{Dark: true}},
		{"motion-reduce", Condition{MotionReduce: true}},
		{"motion-safe", Condition{MotionSafe: true}},
		{"starting", Condition{Starting: true}},
		{"rtl", Condition{RTL: true}},
		{"ltr", Condition{LTR: true}},
		{"sm", Condition{MinWidth: 640}},
		{"md", Condition{MinWidth: 768}},
		{"lg", Condition{MinWidth: 1024}},
		{"xl", Condition{MinWidth: 1280}},
		{"2xl", Condition{MinWidth: 1536}},
	}
	for _, tt := range tests {
		got, err := parseVariant(th, tt.variant)
		if err != nil {
			t.Errorf("%s: %v", tt.variant, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%s = %+v, want %+v", tt.variant, got, tt.want)
		}
	}
}

func TestVariantErrors(t *testing.T) {
	th := DefaultTheme()
	for _, v := range []string{"hoverr", "focu", "group-hoverr", "mdd", "drak"} {
		_, err := parseVariant(th, v)
		var u *UnknownClassError
		if !errors.As(err, &u) || !u.Variant {
			t.Errorf("%s: err = %v, want an unknown variant", v, err)
		}
	}
	if _, err := parseVariant(th, "hoverr"); err == nil || err.Error() != `unknown variant "hoverr"; did you mean "hover"?` {
		t.Errorf("message = %v", err)
	}
	for _, v := range []string{"before", "after", "marker", "print", "group-hover/item", "data-open", "has-checked", "@lg", "aria-[sort=ascending]", "aria-[checked=false]", "aria-[checked]", "group-aria-[sort=ascending]"} {
		_, err := parseVariant(th, v)
		var u *UnsupportedClassError
		if !errors.As(err, &u) {
			t.Errorf("%s: err = %v, want unsupported", v, err)
		}
	}
}

func TestVariantRules(t *testing.T) {
	c := mustCompile(t, "bg-white dark:hover:bg-black md:p-8 group-hover:underline")
	if len(c.Rules) != 4 {
		t.Fatalf("Rules = %d", len(c.Rules))
	}
	if !c.Rules[0].Cond.IsZero() {
		t.Error("a plain class has a condition")
	}
	eq(t, "stacked", c.Rules[1].Cond, Condition{Dark: true, State: StateHover})
	eq(t, "md", c.Rules[2].Cond, Condition{MinWidth: 768})
	eq(t, "group", c.Rules[3].Cond, Condition{Group: StateHover})
	eq(t, "class text", c.Rules[1].Class, "dark:hover:bg-black")
}

// TestStartingStyle: starting: rules apply only to the starting style
// (CSS @starting-style), in order of appearance like every other rule.
func TestStartingStyle(t *testing.T) {
	c := mustCompile(t, "opacity-100 scale-100 starting:opacity-0 starting:scale-95 dark:starting:opacity-50")
	eq(t, "stacked", c.Rules[4].Cond, Condition{Dark: true, Starting: true})
	cx := &Context{Viewport: gfx.Size{W: 800, H: 600}}
	s := Compute(cx, Input{Kind: KindDiv, Classes: []*Classes{c}})
	eq(t, "opacity", s.Opacity, float32(1))
	eq(t, "scale", s.Transform.ScaleX, float32(1))
	st := Compute(cx, Input{Kind: KindDiv, Classes: []*Classes{c}, Starting: true})
	eq(t, "starting opacity", st.Opacity, float32(0))
	eq(t, "starting scale", st.Transform.ScaleX, float32(0.95))
	cx.Dark = true
	st = Compute(cx, Input{Kind: KindDiv, Classes: []*Classes{c}, Starting: true})
	eq(t, "dark starting opacity", st.Opacity, float32(0.5))
}

func TestVariantEvaluationInCompute(t *testing.T) {
	cl := mustCompile(t, "bg-white hover:bg-red-500 dark:bg-black dark:hover:bg-blue-500 md:p-8 group-hover:text-red-500 peer-checked:opacity-50 motion-reduce:duration-0 first:mt-2 disabled:cursor-not-allowed")
	th := DefaultTheme().Colors
	run := func(cx Context, in Input) *ComputedStyle {
		in.Classes = []*Classes{cl}
		return Compute(&cx, in)
	}
	base := Context{Viewport: gfx.Size{W: 500, H: 800}}

	s := run(base, Input{})
	eq(t, "plain", s.BackgroundColor, gfx.White)
	eq(t, "padding", s.Padding.Top, PxLP(0))

	s = run(base, Input{State: StateHover})
	eq(t, "hover", s.BackgroundColor, th["red-500"])

	dark := base
	dark.Dark = true
	s = run(dark, Input{})
	eq(t, "dark", s.BackgroundColor, gfx.Black)
	s = run(dark, Input{State: StateHover})
	eq(t, "dark:hover", s.BackgroundColor, th["blue-500"])

	wide := Context{Viewport: gfx.Size{W: 800, H: 600}}
	eq(t, "md", run(wide, Input{}).Padding.Top, PxLP(32))
	eq(t, "below md", run(base, Input{}).Padding.Top, PxLP(0))

	eq(t, "group", run(base, Input{Group: StateHover}).Text.Color, th["red-500"])
	eq(t, "group needs ancestor", run(base, Input{State: StateHover}).Text.Color, gfx.Black)
	eq(t, "peer", run(base, Input{Peer: StateChecked}).Opacity, 0.5)

	rm := base
	rm.ReducedMotion = true
	s = run(rm, Input{})
	eq(t, "motion-reduce duration", s.Transition.Duration, 0)

	eq(t, "first", run(base, Input{State: StateFirst}).Margin.Top, DimPx(8))
	eq(t, "not first", run(base, Input{}).Margin.Top, DimPx(0))
	eq(t, "disabled", run(base, Input{State: StateDisabled}).Cursor, CursorNotAllowed)
}

func TestVariantOrderLastWins(t *testing.T) {
	// A variant rule that comes later overrides; one that comes earlier is overridden.
	late := style2(t, "bg-white hover:bg-black", Input{State: StateHover})
	eq(t, "late", late.BackgroundColor, gfx.Black)
	early := style2(t, "hover:bg-black bg-white", Input{State: StateHover})
	eq(t, "early", early.BackgroundColor, gfx.White)
}

func style2(t *testing.T, classes string, in Input) *ComputedStyle {
	t.Helper()
	in.Classes = []*Classes{mustCompile(t, classes)}
	return Compute(testCx, in)
}

func TestGroupPeerMarkers(t *testing.T) {
	c := mustCompile(t, "group flex")
	if !c.Group || c.Peer {
		t.Errorf("Group, Peer = %v, %v", c.Group, c.Peer)
	}
	c = mustCompile(t, "peer hover:bg-white")
	if c.Group || !c.Peer {
		t.Errorf("Group, Peer = %v, %v", c.Group, c.Peer)
	}
	if c := mustCompile(t, "flex"); c.Group || c.Peer {
		t.Error("markers on a plain string")
	}
	if c := Compile("hover:group"); c.Group {
		t.Error("a variant group counts as a marker")
	}
}

func TestWidthVariantParsing(t *testing.T) {
	th := DefaultTheme()
	tests := []struct {
		variant string
		want    Condition
	}{
		{"max-sm", Condition{MaxWidth: 640, HasMaxWidth: true}},
		{"max-md", Condition{MaxWidth: 768, HasMaxWidth: true}},
		{"max-lg", Condition{MaxWidth: 1024, HasMaxWidth: true}},
		{"max-xl", Condition{MaxWidth: 1280, HasMaxWidth: true}},
		{"max-2xl", Condition{MaxWidth: 1536, HasMaxWidth: true}},
		{"min-md", Condition{MinWidth: 768}},
		{"min-[600px]", Condition{MinWidth: 600}},
		{"max-[600px]", Condition{MaxWidth: 600, HasMaxWidth: true}},
		{"min-[40rem]", Condition{MinWidth: 640}},
		{"max-[48rem]", Condition{MaxWidth: 768, HasMaxWidth: true}},
		{"max-[30.5em]", Condition{MaxWidth: 488, HasMaxWidth: true}},
		{"max-[0px]", Condition{MaxWidth: 0, HasMaxWidth: true}},
	}
	for _, tt := range tests {
		got, err := parseVariant(th, tt.variant)
		if err != nil {
			t.Errorf("%s: %v", tt.variant, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%s = %+v, want %+v", tt.variant, got, tt.want)
		}
	}
}

func TestWidthVariantErrors(t *testing.T) {
	th := DefaultTheme()
	for variant, want := range map[string]string{
		"max-smm":    `unknown variant "max-smm"; did you mean "max-sm"?`,
		"min-mdd":    `unknown variant "min-mdd"; did you mean "min-md"?`,
		"max-":       `unknown variant "max-"`,
		"max-[600]":  `invalid class "max-[600]": the width must be a length in px, rem or em, such as the variants min-[600px] and max-[40rem]`,
		"min-[wide]": `invalid class "min-[wide]": the width must be a length in px, rem or em, such as the variants min-[600px] and max-[40rem]`,
		"max-[-5px]": `invalid class "max-[-5px]": the width must be a length in px, rem or em, such as the variants min-[600px] and max-[40rem]`,
		"max-[50vw]": `invalid class "max-[50vw]": the width must be a length in px, rem or em, such as the variants min-[600px] and max-[40rem]`,
		"min-[600px": `the arbitrary width variant needs a closing bracket, as in min-[600px]`,
	} {
		_, err := parseVariant(th, variant)
		if err == nil || !strings.Contains(err.Error(), want) && err.Error() != want {
			t.Errorf("%s: err = %v, want %q", variant, err, want)
		}
	}
}

func TestWidthVariantMatches(t *testing.T) {
	c := mustCompile(t, "p-1 md:max-xl:p-2 max-sm:p-3 min-[900px]:max-[1000px]:p-4 max-[0px]:p-5")
	pad := func(w float32) LP {
		return Compute(&Context{Viewport: gfx.Size{W: w, H: 600}}, Input{Kind: KindDiv, Classes: []*Classes{c}}).Padding.Top
	}
	eq(t, "stacked rules", c.Rules[1].Cond, Condition{MinWidth: 768, MaxWidth: 1280, HasMaxWidth: true})
	for _, tt := range []struct {
		w    float32
		want float32
	}{
		{500, 3}, {639, 3}, {640, 1}, {767, 1}, {768, 2}, {899, 2}, {900, 4}, {999, 4}, {1000, 2}, {1279, 2}, {1280, 1},
	} {
		eq(t, "width", pad(tt.w), PxLP(tt.want*4))
	}
	// max-[0px] never matches: no width is below 0.
	if (Condition{MaxWidth: 0, HasMaxWidth: true}).Matches(VariantContext{ViewportWidth: 0}) {
		t.Error("max-[0px] matches width 0")
	}
}

func TestAriaVariantParsing(t *testing.T) {
	th := DefaultTheme()
	for name, flag := range map[string]StateFlags{
		"busy": StateAriaBusy, "checked": StateAriaChecked, "disabled": StateAriaDisabled,
		"expanded": StateAriaExpanded, "hidden": StateAriaHidden, "pressed": StateAriaPressed,
		"readonly": StateAriaReadOnly, "required": StateAriaRequired, "selected": StateAriaSelected,
	} {
		for _, tt := range []struct {
			variant string
			want    Condition
		}{
			{"aria-" + name, Condition{State: flag}},
			{"aria-[" + name + "=true]", Condition{State: flag}},
			{"group-aria-" + name, Condition{Group: flag}},
			{"group-aria-[" + name + "=true]", Condition{Group: flag}},
			{"peer-aria-" + name, Condition{Peer: flag}},
			{"peer-aria-[" + name + "=true]", Condition{Peer: flag}},
		} {
			got, err := parseVariant(th, tt.variant)
			if err != nil {
				t.Errorf("%s: %v", tt.variant, err)
			} else if got != tt.want {
				t.Errorf("%s = %+v, want %+v", tt.variant, got, tt.want)
			}
		}
	}
}

func TestAriaVariantErrors(t *testing.T) {
	th := DefaultTheme()
	for variant, want := range map[string]string{
		"aria-chekced":                `unknown variant "aria-chekced"; did you mean "aria-checked"?`,
		"group-aria-expandd":          `unknown variant "group-aria-expandd"; did you mean "group-aria-expanded"?`,
		"aria-[sort=ascending]":       `Tela has no prop for the attribute selector [sort=ascending]`,
		"aria-[checked=false]":        `Tela has no prop for the attribute selector [checked=false]`,
		"peer-aria-[sort=descending]": `Tela has no prop for the attribute selector [sort=descending]`,
	} {
		_, err := parseVariant(th, variant)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want it to contain %q", variant, err, want)
		}
	}
	_, err := parseVariant(th, "aria-[sort=ascending]")
	var u *UnsupportedClassError
	if !errors.As(err, &u) || !strings.Contains(u.Reason, "AriaRequired") || !strings.Contains(u.Reason, "aria-checked:") {
		t.Errorf("err = %v, want an unsupported variant that lists the supported ones", err)
	}
}

func TestAriaVariantEvaluationInCompute(t *testing.T) {
	cl := mustCompile(t, "opacity-100 aria-busy:opacity-50 aria-expanded:p-4 group-aria-checked:font-bold peer-aria-selected:italic aria-[pressed=true]:m-2")
	run := func(in Input) *ComputedStyle {
		in.Classes = []*Classes{cl}
		return Compute(&Context{Viewport: gfx.Size{W: 500, H: 800}}, in)
	}
	eq(t, "none", run(Input{}).Opacity, float32(1))
	eq(t, "busy", run(Input{State: StateAriaBusy}).Opacity, float32(0.5))
	eq(t, "expanded", run(Input{State: StateAriaExpanded}).Padding.Top, PxLP(16))
	eq(t, "other state", run(Input{State: StateAriaChecked}).Padding.Top, PxLP(0))
	eq(t, "pressed", run(Input{State: StateAriaPressed}).Margin.Top, DimPx(8))
	if run(Input{Group: StateAriaChecked}).Text.FontWeight == run(Input{State: StateAriaChecked}).Text.FontWeight {
		t.Error("group-aria-checked: must test the group ancestor, not the element")
	}
	if run(Input{Peer: StateAriaSelected}).Text.FontStyle == run(Input{}).Text.FontStyle {
		t.Error("peer-aria-selected: must test the peer sibling")
	}
}
