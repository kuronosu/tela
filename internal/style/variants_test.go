package style

import (
	"errors"
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
	for _, v := range []string{"before", "after", "marker", "print", "group-hover/item", "rtl", "aria-checked", "data-open", "has-checked", "max-md", "@lg"} {
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
