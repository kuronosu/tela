package telatest

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/semantics"
)

// The accessibility audit (ADR-306): rules after axe-core's, run on the
// semantics tree the OS accessibility bridges receive (and, for the rules
// about what is on screen, on the laid-out tree), reported as violations
// that name the rule, the element's line of the aria snapshot, its source
// when the app is instrumented and the fix. [App.ExpectAccessible] fails
// the test with them; [App.Audit] returns them, as axe.analyze() does, and
// the options follow @axe-core/playwright's AxeBuilder: [WithRules],
// [WithTags], [DisableRules] and [Exclude].

// Rule describes one rule of the audit.
type Rule struct {
	// ID names the rule, in axe-core's kebab-case ("aria-command-name",
	// "color-contrast", "target-size"), as WithRules and DisableRules take it.
	ID string
	// Description is what the rule checks, in one sentence.
	Description string
	// Impact is axe's: "minor", "moderate", "serious" or "critical".
	Impact string
	// Tags select rules (WithTags): axe's "wcag2a", "wcag2aa", "wcag22aa",
	// "wcag412"... for the success criterion, "aria" for the rules about
	// ARIA semantics, "layout" for the rules about what is on screen
	// that no guideline names, and "tela" for the rules that check what
	// Tela's own bridges need.
	Tags []string
	// Default reports that [App.ExpectAccessible] and [App.Audit] run the
	// rule when no WithRules or WithTags narrows them. The rules that
	// are not default move the focus or are about layout, not
	// accessibility (focus-visible, text-clipped, text-overlap,
	// target-overlap); `tela check` runs them all.
	Default bool
}

// Violation is one element that breaks a rule.
type Violation struct {
	// Rule is the ID of the rule and Impact its impact.
	Rule, Impact string
	// Message says what is wrong and Fix what to change.
	Message, Fix string
	// Element is the element's line of the aria snapshot without its
	// bullet, such as `button "Save" [disabled]`; for text without a role
	// it is the text, quoted, within its nearest element that has one.
	Element string
	// Source is where the app made the element, "file.go:42", when the
	// app was mounted with [Instrument] in dev mode ("" otherwise).
	Source string
	// Bounds is the element's box in window coordinates, in logical
	// pixels.
	Bounds tela.Rect
}

// String formats the violation on four lines.
func (v Violation) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s): %s\n", v.Rule, v.Impact, v.Message)
	el := v.Element
	if v.Source != "" {
		el += " [src=" + v.Source + "]"
	}
	fmt.Fprintf(&b, "    element: %s\n", el)
	fmt.Fprintf(&b, "    fix: %s", v.Fix)
	return b.String()
}

// jsonViolation is the serialized form of a Violation: the rectangle as
// {x, y, width, height}.
type jsonViolation struct {
	Rule    string  `json:"rule"`
	Impact  string  `json:"impact"`
	Message string  `json:"message"`
	Fix     string  `json:"fix"`
	Element string  `json:"element"`
	Source  string  `json:"src,omitempty"`
	X       float32 `json:"x"`
	Y       float32 `json:"y"`
	Width   float32 `json:"width"`
	Height  float32 `json:"height"`
}

// MarshalJSON writes the violation with lower-case names and its bounds
// as x, y, width and height.
func (v Violation) MarshalJSON() ([]byte, error) {
	return json.Marshal(jsonViolation{v.Rule, v.Impact, v.Message, v.Fix, v.Element, v.Source,
		v.Bounds.X, v.Bounds.Y, v.Bounds.Width, v.Bounds.Height})
}

// UnmarshalJSON reads what MarshalJSON writes.
func (v *Violation) UnmarshalJSON(b []byte) error {
	var j jsonViolation
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	*v = Violation{j.Rule, j.Impact, j.Message, j.Fix, j.Element, j.Source,
		tela.Rect{X: j.X, Y: j.Y, Width: j.Width, Height: j.Height}}
	return nil
}

// AuditOption narrows or configures [App.Audit] and [App.ExpectAccessible].
type AuditOption func(*auditConfig)

type auditConfig struct {
	rules, tags, disabled []string
	exclude               []Locator
}

// WithRules runs only the rules with these IDs (axe's withRules), the
// non-default ones included. An unknown ID fails the test, with the closest
// ones.
//
//	app.ExpectAccessible(telatest.WithRules("color-contrast"))
func WithRules(ids ...string) AuditOption {
	return func(c *auditConfig) { c.rules = append(c.rules, ids...) }
}

// WithTags runs only the rules that have one of these tags (axe's
// withTags), the non-default ones included: "wcag2aa" for the AA success
// criteria, "wcag22aa" for 2.2's, "aria", "layout"... [Rules] lists them.
//
//	app.ExpectAccessible(telatest.WithTags("wcag2a", "wcag2aa"))
func WithTags(tags ...string) AuditOption {
	return func(c *auditConfig) { c.tags = append(c.tags, tags...) }
}

// DisableRules leaves these rules out of the run (axe's disableRules): the
// way to accept a violation of one rule for one call, the rest of the
// audit still running. An unknown ID fails the test.
//
//	app.ExpectAccessible(telatest.DisableRules("target-size"))
func DisableRules(ids ...string) AuditOption {
	return func(c *auditConfig) { c.disabled = append(c.disabled, ids...) }
}

// Exclude leaves out of the audit the elements l matches and everything
// inside them (axe's exclude), for a part of the screen that is not the
// app's to fix, such as an embedded third-party widget. A locator that
// matches nothing excludes nothing.
//
//	app.ExpectAccessible(telatest.Exclude(telatest.ByTestID("legacy-chart")))
func Exclude(l Locator) AuditOption {
	return func(c *auditConfig) { c.exclude = append(c.exclude, l) }
}

// ruleDef is a rule with the code that checks it.
type ruleDef struct {
	Rule
	// node checks one visible element node of the semantics tree.
	node func(c *auditCtx, n *semantics.Node)
	// all checks the whole screen, for the rules that read more than the
	// nodes (what is on screen, the focus).
	all func(c *auditCtx)
}

// auditCtx is what the rules read and where they report.
type auditCtx struct {
	a      *App
	rule   *ruleDef
	root   *semantics.Node
	byID   map[semantics.NodeID]*semantics.Node
	parent map[*semantics.Node]*semantics.Node
	// excluded are the nodes of Exclude; hits the violations so far.
	excluded map[semantics.NodeID]bool
	hits     []Violation
	// lines caches the aria snapshot line of a node.
	lines map[semantics.NodeID]string
}

// report adds a violation of the current rule on node n.
func (c *auditCtx) report(n *semantics.Node, message, fix string) {
	c.reportAt(n, c.describe(n), "", n.Bounds, message, fix)
}

// reportAt adds a violation with the element described as element, the
// source of node n (src, when it is not n's own) and bounds.
func (c *auditCtx) reportAt(n *semantics.Node, element, src string, bounds gfx.Rect, message, fix string) {
	if src == "" && n != nil && c.a.tree.Instrumented() {
		src = c.a.tree.AppSite(c.a.tree.ElementOfNode(n.ID)).Rel()
	}
	c.hits = append(c.hits, Violation{
		Rule: c.rule.ID, Impact: c.rule.Impact, Message: message, Fix: fix,
		Element: element, Source: src, Bounds: tela.Rect{X: bounds.X0, Y: bounds.Y0, Width: bounds.Width(), Height: bounds.Height()},
	})
}

// describe returns n's line of the aria snapshot, without its bullet: its
// role, name and states. A node the snapshot leaves out (a generic
// container, say) is its role.
func (c *auditCtx) describe(n *semantics.Node) string {
	if s, ok := c.lines[n.ID]; ok {
		return s
	}
	one := *n
	one.Children = nil
	line := strings.TrimPrefix(strings.TrimSpace(semantics.Snapshot(&one)), "- ")
	if line == "" || strings.HasPrefix(line, "text") {
		line = n.Role
		if n.Name != "" {
			line += " " + fmt.Sprintf("%q", n.Name)
		}
	}
	// A key ends at the first colon that is not in the quoted name: what
	// follows is the value.
	if i := strings.Index(line, `":`); i > 0 {
		line = line[:i+1]
	}
	c.lines[n.ID] = line
	return line
}

// isExcluded reports whether n or an ancestor is excluded.
func (c *auditCtx) isExcluded(n *semantics.Node) bool {
	for ; n != nil; n = c.parent[n] {
		if c.excluded[n.ID] {
			return true
		}
	}
	return false
}

// disabled reports whether n or an ancestor is disabled.
func (c *auditCtx) disabled(n *semantics.Node) bool {
	for ; n != nil; n = c.parent[n] {
		if n.Has(semantics.StateDisabled) {
			return true
		}
	}
	return false
}

// rules is the registry of the audit, in the order it reports.
var rules []*ruleDef

// register adds rules to the registry (the init of each rules file).
func register(rs ...*ruleDef) { rules = append(rules, rs...) }

// Rules lists the rules of the audit with their IDs, impacts and tags,
// in the order a run reports them.
func Rules() []Rule {
	out := make([]Rule, len(rules))
	for i, r := range rules {
		out[i] = r.Rule
		out[i].Tags = slices.Clone(r.Tags)
	}
	return out
}

func ruleIDs() []string {
	ids := make([]string, len(rules))
	for i, r := range rules {
		ids[i] = r.ID
	}
	return ids
}

// selectRules returns the rules a configuration runs, or an error for an
// ID that is not a rule.
func (cfg *auditConfig) selectRules() ([]*ruleDef, error) {
	for _, id := range slices.Concat(cfg.rules, cfg.disabled) {
		if !slices.Contains(ruleIDs(), id) {
			msg := fmt.Sprintf("unknown rule %q", id)
			if c := closest(id, ruleIDs()); len(c) > 0 {
				msg += "; did you mean " + strings.Join(c, " or ") + "?"
			} else {
				msg += "; telatest.Rules() lists them"
			}
			return nil, fmt.Errorf("%s", msg)
		}
	}
	var out []*ruleDef
	for _, r := range rules {
		if slices.Contains(cfg.disabled, r.ID) {
			continue
		}
		narrowed := len(cfg.rules) > 0 || len(cfg.tags) > 0
		switch {
		case !narrowed && !r.Default:
			continue
		case narrowed && !slices.Contains(cfg.rules, r.ID) && !slices.ContainsFunc(r.Tags, func(t string) bool { return slices.Contains(cfg.tags, t) }):
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// Audit runs the rules of the audit on the app as it is now, after
// settling it, and returns what it found: one [Violation] per element and
// rule, in tree order within each rule (axe.analyze()'s violations, one
// per node). With no options it runs the default rules, [Rules] with
// Default set: the accessibility ones that only read the screen. A rule
// is selected with [WithRules] or [WithTags], left out with
// [DisableRules], and a part of the screen with [Exclude].
//
// Audit does not fail the test; [App.ExpectAccessible] does. An app
// mounted with [Instrument] also gets the source of each violation. A
// rule that moves the focus (focus-visible) leaves it where it found it.
func (a *App) Audit(opts ...AuditOption) []Violation {
	a.tb.Helper()
	vs, err := a.audit(opts)
	if err != nil {
		a.fatalf("Audit: %v", err)
	}
	return vs
}

// ExpectAccessible fails the test, listing each violation with its rule,
// the element's line of the aria snapshot, its source when the app is
// instrumented and the fix, unless the screen as it is now passes the
// rules [App.Audit] runs. It settles the app first, as the other
// expectations do. Call it in each state worth checking: with an overlay
// open, a form with its errors, the empty list.
//
//	app := telatest.Mount(t, tela.C(Settings), telatest.Instrument())
//	app.ExpectAccessible()
//	app.Click(telatest.ByRole("button", "Delete account"))
//	app.ExpectAccessible(telatest.DisableRules("target-size"))
//
// A failure reads:
//
//	telatest: ExpectAccessible: 1 accessibility violation:
//
//	1. aria-command-name (critical): a button needs an accessible name
//	    element: button [src=settings.go:42]
//	    fix: give it text content, h.AriaLabel("...") or h.AriaLabelledBy(...)
func (a *App) ExpectAccessible(opts ...AuditOption) {
	a.tb.Helper()
	vs, err := a.audit(opts)
	if err != nil {
		a.fatalf("ExpectAccessible: %v", err)
	}
	if len(vs) == 0 {
		return
	}
	var b strings.Builder
	noun := "violations"
	if len(vs) == 1 {
		noun = "violation"
	}
	fmt.Fprintf(&b, "telatest: ExpectAccessible: %d accessibility %s:\n", len(vs), noun)
	for i, v := range vs {
		fmt.Fprintf(&b, "\n%d. %s\n", i+1, v)
	}
	a.tb.Fatal(strings.TrimRight(b.String(), "\n"))
}

// audit is Audit returning the error of a bad option.
func (a *App) audit(opts []AuditOption) ([]Violation, error) {
	a.tb.Helper()
	a.mustBeOpen()
	var cfg auditConfig
	for _, o := range opts {
		o(&cfg)
	}
	rs, err := cfg.selectRules()
	if err != nil {
		return nil, err
	}
	a.settleQuery()
	a.mustBeOpen()
	c := &auditCtx{
		a: a, root: a.tree.Semantics(),
		byID:     map[semantics.NodeID]*semantics.Node{},
		parent:   map[*semantics.Node]*semantics.Node{},
		excluded: map[semantics.NodeID]bool{},
		lines:    map[semantics.NodeID]string{},
	}
	var walk func(n, p *semantics.Node)
	walk = func(n, p *semantics.Node) {
		c.byID[n.ID] = n
		c.parent[n] = p
		for _, k := range n.Children {
			walk(k, n)
		}
	}
	walk(c.root, nil)
	for _, l := range cfg.exclude {
		for _, n := range a.find(l) {
			c.excluded[n.ID] = true
		}
	}
	for _, r := range rs {
		c.rule = r
		if r.node != nil {
			c.root.Walk(func(n *semantics.Node) bool {
				if n.Has(semantics.StateHidden) || c.excluded[n.ID] {
					return false
				}
				if n.Kind == semantics.KindElement {
					r.node(c, n)
				}
				return true
			})
		}
		if r.all != nil {
			r.all(c)
		}
	}
	return c.hits, nil
}
