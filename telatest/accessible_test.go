package telatest_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// ExpectAccessible and Audit (ADR-306).

// goodScreen has a named button, a labelled field and a heading.
func goodScreen() tela.Node {
	return Div(Class("flex flex-col gap-3 p-4"),
		H1(Text("Settings")),
		Button(Class("h-9 rounded bg-stone-900 px-4 text-white"), Text("Save")),
		Input(AriaLabel("Email")),
	)
}

// nameless has a button with no text and no label.
func nameless() tela.Node {
	return Div(Class("p-4"),
		Button(Class("size-9 rounded bg-stone-900")),
		Button(Class("h-9 rounded bg-stone-900 px-4 text-white"), Text("Save")),
	)
}

func TestExpectAccessiblePasses(t *testing.T) {
	t.Parallel()
	app := telatest.MountFunc(t, goodScreen, telatest.Size(400, 300))
	app.ExpectAccessible()
	if v := app.Audit(); len(v) != 0 {
		t.Fatalf("a good screen has violations: %v", v)
	}
}

func TestExpectAccessibleFailsWithTheViolation(t *testing.T) {
	t.Parallel()
	mustFail(t, func(tb testing.TB) {
		app := telatest.MountFunc(tb, nameless, telatest.Size(400, 300), telatest.Instrument())
		app.ExpectAccessible()
	},
		"aria-command-name",        // the rule
		`button`,                   // the element's aria snapshot line
		"[src=accessible_test.go:", // its source, when instrumented
		"AriaLabel",                // the fix
		"1 accessibility violation",
	)
}

func TestAuditViolationFields(t *testing.T) {
	t.Parallel()
	app := telatest.MountFunc(t, nameless, telatest.Size(400, 300))
	vs := app.Audit()
	if len(vs) != 1 {
		t.Fatalf("got %d violations, want 1: %v", len(vs), vs)
	}
	v := vs[0]
	if v.Rule != "aria-command-name" || v.Impact != "critical" || !strings.Contains(v.Element, "button") ||
		v.Message == "" || v.Fix == "" || v.Bounds.Width == 0 {
		t.Errorf("violation %+v", v)
	}
	if v.Source != "" {
		t.Errorf("source %q without Instrument", v.Source)
	}
}

func TestAuditDisableRules(t *testing.T) {
	t.Parallel()
	app := telatest.MountFunc(t, nameless, telatest.Size(400, 300))
	if v := app.Audit(telatest.DisableRules("aria-command-name")); len(v) != 0 {
		t.Errorf("a disabled rule reported %v", v)
	}
	mustPass(t, func(tb testing.TB) {
		app := telatest.MountFunc(tb, nameless, telatest.Size(400, 300))
		app.ExpectAccessible(telatest.DisableRules("aria-command-name"))
	})
}

func TestAuditWithRulesAndTags(t *testing.T) {
	t.Parallel()
	app := telatest.MountFunc(t, nameless, telatest.Size(400, 300))
	if v := app.Audit(telatest.WithRules("target-size")); len(v) != 0 {
		t.Errorf("only target-size, got %v", v)
	}
	if v := app.Audit(telatest.WithTags("wcag412")); len(v) != 1 {
		t.Errorf("tag wcag412: %v", v)
	}
}

func TestAuditExclude(t *testing.T) {
	t.Parallel()
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(
			Div(TestID("legacy"), Button(Class("size-9 bg-stone-900"))),
			Button(Class("h-9 px-4"), Text("Ok")))
	}, telatest.Size(400, 300))
	if v := app.Audit(); len(v) != 1 {
		t.Fatalf("got %v", v)
	}
	if v := app.Audit(telatest.Exclude(telatest.ByTestID("legacy"))); len(v) != 0 {
		t.Errorf("an excluded subtree reported %v", v)
	}
}

func TestAuditUnknownRule(t *testing.T) {
	t.Parallel()
	mustFail(t, func(tb testing.TB) {
		app := telatest.MountFunc(tb, goodScreen, telatest.Size(400, 300))
		app.ExpectAccessible(telatest.DisableRules("aria-command-nam"))
	}, `unknown rule "aria-command-nam"`, "did you mean aria-command-name")
}

func TestRulesListed(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, r := range telatest.Rules() {
		if r.ID == "" || r.Description == "" || r.Impact == "" || len(r.Tags) == 0 {
			t.Errorf("incomplete rule %+v", r)
		}
		if seen[r.ID] {
			t.Errorf("rule %s listed twice", r.ID)
		}
		seen[r.ID] = true
	}
	for _, id := range []string{"aria-command-name", "aria-required-attr", "target-size"} {
		if !seen[id] {
			t.Errorf("no rule %s", id)
		}
	}
}

func TestTargetSize(t *testing.T) {
	t.Parallel()
	// Two 16 px targets 4 px apart: each one's 24 px circle reaches the
	// other (WCAG 2.5.8), so both fail; with 24 px between them they pass.
	crowded := func(gap string) func() tela.Node {
		return func() tela.Node {
			return Div(Class("flex "+gap),
				Button(Class("size-4 bg-stone-900"), AriaLabel("A")),
				Button(Class("size-4 bg-stone-900"), AriaLabel("B")))
		}
	}
	app := telatest.MountFunc(t, crowded("gap-1"), telatest.Size(200, 100))
	vs := app.Audit(telatest.WithRules("target-size"))
	if len(vs) != 2 || vs[0].Rule != "target-size" || !strings.Contains(vs[0].Message, "16x16") {
		t.Errorf("crowded targets: %v", vs)
	}
	app = telatest.MountFunc(t, crowded("gap-8"), telatest.Size(200, 100))
	if vs := app.Audit(telatest.WithRules("target-size")); len(vs) != 0 {
		t.Errorf("spaced targets: %v", vs)
	}
	// A disabled target is exempt (WCAG 2.5.8: inactive controls).
	app = telatest.MountFunc(t, func() tela.Node {
		return Div(Class("flex gap-1"),
			Button(Class("size-4 bg-stone-900"), AriaLabel("A"), Disabled(true)),
			Button(Class("size-4 bg-stone-900"), AriaLabel("B"), Disabled(true)))
	}, telatest.Size(200, 100))
	if vs := app.Audit(telatest.WithRules("target-size")); len(vs) != 0 {
		t.Errorf("disabled targets: %v", vs)
	}
}
