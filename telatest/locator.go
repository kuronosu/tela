package telatest

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/internal/semantics"
)

// Locator describes the element an action or expectation works on, like a
// Playwright locator: it is resolved against the live semantics tree each
// time it is used, so it keeps working when the app rebuilds. Make one
// with [ByRole], [ByText], [ByClass] or [ByTestID] and narrow it with
// [Locator.Filter] and [Locator.Nth].
//
// Hidden elements (display: none, visibility other than visible,
// AriaHidden, and their descendants) are never matched. An action needs the
// locator to match exactly one element (strictness, as in Playwright) and
// fails with the aria snapshot and the closest names when it matches none
// or several.
type Locator struct {
	kind    locKind
	role    string
	name    string
	text    string
	exact   bool
	classes []string
	testID  string
	// hasText are the texts every match must contain (Filter), and
	// hasClass the class names it must have.
	hasText  []string
	hasClass []string
	// nth selects one match when hasNth (negative counts from the end).
	nth    int
	hasNth bool
}

type locKind uint8

const (
	locNone locKind = iota
	locRole
	locText
	locClass
	locTestID
)

// ByRole locates the elements with an ARIA role (explicit with h.Role or
// implicit: button, heading, link, checkbox...) and, when name is not
// empty, exactly this accessible name (whitespace normalized, case
// sensitive). Elements without a role of their own (Div, Span) are never
// matched by role, as in Playwright. An empty name matches any name.
//
//	telatest.ByRole("button", "Increment")
//	telatest.ByRole("heading", "")
func ByRole(role, name string) Locator { return Locator{kind: locRole, role: role, name: name} }

// TextOption configures [ByText].
type TextOption func(*Locator)

// Exact makes [ByText] match the whole text, case sensitive, instead of a
// case-insensitive substring.
func Exact() TextOption { return func(l *Locator) { l.exact = true } }

// ByText locates the elements that contain text, like Playwright's
// getByText: it matches the text content of an element (whitespace
// normalized, hidden descendants left out, block boundaries counting as
// spaces) and picks the deepest elements that match, so the button that
// holds the text rather than the page that holds the button. The match is a
// case-insensitive substring unless [Exact] is given.
//
//	telatest.ByText("Count: 1")
//	telatest.ByText("Save", telatest.Exact())
func ByText(text string, opts ...TextOption) Locator {
	l := Locator{kind: locText, text: text}
	for _, o := range opts {
		o(&l)
	}
	return l
}

// ByClass locates the elements that have all the given Tailwind class
// names. It is an escape hatch for elements without a role or text, and
// ties the test to the styling: prefer [ByRole] and [ByText].
//
//	telatest.ByClass("rounded-full", "shadow-sm")
func ByClass(classes ...string) Locator {
	var names []string
	for _, c := range classes {
		names = append(names, strings.Fields(c)...)
	}
	return Locator{kind: locClass, classes: names}
}

// ByTestID locates the element that carries the h.TestID with this exact
// id, like Playwright's getByTestId. It is the way to find an element a
// user cannot name (a card, a chart, a drop zone); the id is not part of
// the semantics tree or the aria snapshot, so it is never visible to
// assistive technologies. Hidden elements are not matched, like with the
// other locators.
//
//	telatest.ByTestID("sidebar")
func ByTestID(id string) Locator { return Locator{kind: locTestID, testID: id} }

// FilterOption narrows a locator in [Locator.Filter].
type FilterOption func(*Locator)

// HasText keeps the elements whose text content contains text (a
// case-insensitive substring, whitespace normalized), like Playwright's
// filter({hasText}).
func HasText(text string) FilterOption {
	return func(l *Locator) { l.hasText = append(l.hasText, text) }
}

// HasClass keeps the elements that have all the given Tailwind class names.
// Like [ByClass] it ties the test to the styling.
func HasClass(classes ...string) FilterOption {
	return func(l *Locator) {
		for _, c := range classes {
			l.hasClass = append(l.hasClass, strings.Fields(c)...)
		}
	}
}

// Filter returns a locator that matches only the elements of l that satisfy
// the options.
//
//	telatest.ByRole("button", "").Filter(telatest.HasText("System"))
func (l Locator) Filter(opts ...FilterOption) Locator {
	l.hasText = slices.Clone(l.hasText)
	l.hasClass = slices.Clone(l.hasClass)
	for _, o := range opts {
		o(&l)
	}
	return l
}

// Nth returns a locator for the i-th match of l (0 is the first; a
// negative i counts from the end, so -1 is the last). It turns a locator
// that matches several elements into one that matches one.
func (l Locator) Nth(i int) Locator {
	l.nth, l.hasNth = i, true
	return l
}

// String describes the locator the way Playwright does.
func (l Locator) String() string {
	var s string
	switch l.kind {
	case locRole:
		s = "getByRole(" + strconv.Quote(l.role)
		if l.name != "" {
			s += ", name=" + strconv.Quote(l.name)
		}
		s += ")"
	case locText:
		s = "getByText(" + strconv.Quote(l.text)
		if l.exact {
			s += ", exact"
		}
		s += ")"
	case locClass:
		s = "getByClass(" + strconv.Quote(strings.Join(l.classes, " ")) + ")"
	case locTestID:
		s = "getByTestId(" + strconv.Quote(l.testID) + ")"
	default:
		s = "(empty locator)"
	}
	for _, t := range l.hasText {
		s += ".filter(hasText=" + strconv.Quote(t) + ")"
	}
	if len(l.hasClass) > 0 {
		s += ".filter(hasClass=" + strconv.Quote(strings.Join(l.hasClass, " ")) + ")"
	}
	if l.hasNth {
		s += ".nth(" + strconv.Itoa(l.nth) + ")"
	}
	return s
}

// problem reports a locator that can never match, with the fix.
func (l Locator) problem() string {
	switch l.kind {
	case locRole:
		if l.role == "" {
			return `ByRole needs a role such as "button" or "heading"; to find elements by their text use ByText`
		}
		if !semantics.ValidRole(l.role) {
			msg := fmt.Sprintf("ByRole: unknown role %q", l.role)
			if s := semantics.SuggestRole(l.role); s != "" {
				msg += fmt.Sprintf("; did you mean %q?", s)
			}
			return msg
		}
	case locText:
		if normalize(l.text) == "" {
			return "ByText with an empty text matches every element; pass the text to look for"
		}
	case locClass:
		if len(l.classes) == 0 {
			return "ByClass needs at least one class name, such as ByClass(\"rounded-full\")"
		}
	case locTestID:
		if l.testID == "" {
			return "ByTestID needs the id given to h.TestID, such as ByTestID(\"sidebar\")"
		}
	default:
		return "the zero Locator matches nothing; make one with ByRole, ByText, ByClass or ByTestID"
	}
	for _, t := range l.hasText {
		if normalize(t) == "" {
			return "HasText with an empty text keeps every element; pass the text to look for"
		}
	}
	return ""
}

// ---------------------------------------------------------------------
// Resolution.

// matchesText reports whether the normalized element text s satisfies the
// ByText or HasText matcher.
func matchText(s, want string, exact bool) bool {
	want = normalize(want)
	if exact {
		return s == want
	}
	return strings.Contains(strings.ToLower(s), strings.ToLower(want))
}

func normalize(s string) string { return semantics.NormalizeWhiteSpace(s) }

// find resolves l against the app's current semantics tree: the matching
// nodes in tree order, after the filters and the Nth selection.
func (a *App) find(l Locator) []*semantics.Node {
	root := a.tree.Semantics()
	var nodes []*semantics.Node
	switch l.kind {
	case locRole:
		root.Walk(func(n *semantics.Node) bool {
			if n.Has(semantics.StateHidden) {
				return false
			}
			if n.Kind == semantics.KindElement && !n.Flattened() && n.Role == l.role && (l.name == "" || n.Name == l.name) {
				nodes = append(nodes, n)
			}
			return true
		})
	case locClass:
		root.Walk(func(n *semantics.Node) bool {
			if n.Has(semantics.StateHidden) {
				return false
			}
			if n.Kind == semantics.KindElement && hasClasses(a.tree.NodeClasses(n.ID), l.classes) {
				nodes = append(nodes, n)
			}
			return true
		})
	case locTestID:
		root.Walk(func(n *semantics.Node) bool {
			if n.Has(semantics.StateHidden) {
				return false
			}
			if n.Kind == semantics.KindElement && a.tree.NodeTestID(n.ID) == l.testID {
				nodes = append(nodes, n)
			}
			return true
		})
	case locText:
		nodes = findText(root, l)
	}
	if len(l.hasText) > 0 {
		tc := newTexts()
		nodes = slices.DeleteFunc(nodes, func(n *semantics.Node) bool {
			s := tc.norm(n)
			for _, t := range l.hasText {
				if !matchText(s, t, false) {
					return true
				}
			}
			return false
		})
	}
	if len(l.hasClass) > 0 {
		nodes = slices.DeleteFunc(nodes, func(n *semantics.Node) bool {
			return !hasClasses(a.tree.NodeClasses(n.ID), l.hasClass)
		})
	}
	if l.hasNth {
		i := l.nth
		if i < 0 {
			i += len(nodes)
		}
		if i < 0 || i >= len(nodes) {
			return nil
		}
		return nodes[i : i+1]
	}
	return nodes
}

func hasClasses(have, want []string) bool {
	for _, w := range want {
		if !slices.Contains(have, w) {
			return false
		}
	}
	return true
}

// texts caches the text content of nodes.
type texts struct {
	raw map[*semantics.Node]string
	nrm map[*semantics.Node]string
}

func newTexts() *texts {
	return &texts{raw: map[*semantics.Node]string{}, nrm: map[*semantics.Node]string{}}
}

// content returns the text content of n, whitespace not collapsed: the
// texts of its visible descendants in order, with a space around block
// elements (a Playwright text is textContent, which has no such spaces; the
// space keeps "Total" and "$29" in two blocks apart, as in names and in
// innerText).
func (c *texts) content(n *semantics.Node) string {
	if s, ok := c.raw[n]; ok {
		return s
	}
	var s string
	switch {
	case n.Has(semantics.StateHidden):
	case n.Kind == semantics.KindText:
		s = n.Text
	default:
		var b strings.Builder
		for _, ch := range n.Children {
			t := c.content(ch)
			if ch.Kind == semantics.KindElement && !ch.Inline && t != "" {
				b.WriteByte(' ')
				b.WriteString(t)
				b.WriteByte(' ')
			} else {
				b.WriteString(t)
			}
		}
		s = b.String()
	}
	c.raw[n] = s
	return s
}

// norm returns the text of n with its white space normalized.
func (c *texts) norm(n *semantics.Node) string {
	if s, ok := c.nrm[n]; ok {
		return s
	}
	s := normalize(c.content(n))
	c.nrm[n] = s
	return s
}

// findText returns the deepest visible elements whose text matches l
// (Playwright's "self" matches: the element matches and none of its element
// children does).
func findText(root *semantics.Node, l Locator) []*semantics.Node {
	tc := newTexts()
	var out []*semantics.Node
	var visit func(n *semantics.Node) bool // reports whether n matches
	visit = func(n *semantics.Node) bool {
		if n.Has(semantics.StateHidden) {
			return false
		}
		matches := matchText(tc.norm(n), l.text, l.exact)
		if !matches {
			// A child can match when the parent does not only for an
			// exact match (the parent holds more text); search below.
			for _, ch := range n.Children {
				if ch.Kind == semantics.KindElement {
					visit(ch)
				}
			}
			return false
		}
		childMatches := false
		for _, ch := range n.Children {
			if ch.Kind == semantics.KindElement && visit(ch) {
				childMatches = true
			}
		}
		if !childMatches {
			out = append(out, n)
		}
		return true
	}
	visit(root)
	return out
}

// pageText returns the normalized text content of the whole window.
func (a *App) pageText() string {
	return newTexts().norm(a.tree.Semantics())
}

// ---------------------------------------------------------------------
// Failure messages.

// describeNode names a node for messages: `button "Increment"`.
func describeNode(n *semantics.Node) string {
	switch {
	case n.Kind == semantics.KindRoot:
		return "the window"
	case n.Kind == semantics.KindText:
		return "text " + strconv.Quote(n.Text)
	}
	s := n.Role
	if n.Name != "" {
		s += " " + strconv.Quote(n.Name)
	}
	return s
}

// describeMatch names a node found by a locator, with its text when that
// tells more than its role.
func (a *App) describeMatch(n *semantics.Node) string {
	s := describeNode(n)
	if n.Name == "" && n.Kind == semantics.KindElement {
		if t := newTexts().norm(n); t != "" {
			if len(t) > 50 {
				t = t[:47] + "..."
			}
			s += " " + strconv.Quote(t)
		}
		if cl := a.tree.NodeClasses(n.ID); len(cl) > 0 {
			c := strings.Join(cl, " ")
			if len(c) > 40 {
				c = c[:37] + "..."
			}
			s += " (class " + strconv.Quote(c) + ")"
		}
	}
	if n.Kind == semantics.KindElement {
		if id := a.tree.NodeTestID(n.ID); id != "" {
			s += " (testid " + strconv.Quote(id) + ")"
		}
	}
	return s
}

// strictMessage explains a locator that matched several elements.
func (a *App) strictMessage(l Locator, nodes []*semantics.Node) string {
	var b strings.Builder
	fmt.Fprintf(&b, "strict mode violation: %s matches %d elements:", l, len(nodes))
	for i, n := range nodes {
		if i == 8 {
			fmt.Fprintf(&b, "\n  ... and %d more", len(nodes)-i)
			break
		}
		fmt.Fprintf(&b, "\n  %d) %s", i+1, a.describeMatch(n))
	}
	b.WriteString("\nuse .Nth(i) to pick one, or a more specific name, ByText or Filter(telatest.HasText(...))")
	return b.String()
}

// missMessage explains a locator that matched nothing, with the closest
// candidates.
func (a *App) missMessage(l Locator) string {
	if p := l.problem(); p != "" {
		return p
	}
	var b strings.Builder
	fmt.Fprintf(&b, "no element matches %s", l)
	root := a.tree.Semantics()
	switch l.kind {
	case locRole:
		var names []string
		var roles []string
		root.Walk(func(n *semantics.Node) bool {
			if n.Has(semantics.StateHidden) {
				return false
			}
			if n.Kind != semantics.KindElement || n.Flattened() {
				return true
			}
			if n.Role == l.role {
				names = append(names, n.Name)
			} else if !slices.Contains(roles, n.Role) && n.Role != "generic" {
				roles = append(roles, n.Role)
			}
			return true
		})
		if len(l.hasText) > 0 || len(l.hasClass) > 0 || l.hasNth {
			// The base locator may match; the narrowing removed it.
			base := l
			base.hasText, base.hasClass, base.hasNth = nil, nil, false
			if got := a.find(base); len(got) > 0 {
				fmt.Fprintf(&b, "\n%s alone matches %d element(s); the filter or Nth removed them", base, len(got))
				return b.String()
			}
		}
		switch {
		case len(names) == 0:
			fmt.Fprintf(&b, "\nthere is no %s on screen", l.role)
			if len(roles) > 0 {
				fmt.Fprintf(&b, "; roles on screen: %s", strings.Join(roles, ", "))
			}
		default:
			if sug := closest(l.name, names); len(sug) > 0 {
				fmt.Fprintf(&b, "\ndid you mean %s?", quoteList(l.role, sug))
			} else {
				fmt.Fprintf(&b, "\n%ss on screen: %s", l.role, quoteAll(names))
			}
		}
		// The name may belong to another role.
		if l.name != "" {
			var others []string
			root.Walk(func(n *semantics.Node) bool {
				if n.Has(semantics.StateHidden) {
					return false
				}
				if n.Kind == semantics.KindElement && !n.Flattened() && n.Role != l.role && n.Name == l.name {
					others = append(others, n.Role)
				}
				return true
			})
			if len(others) > 0 {
				fmt.Fprintf(&b, "\nan element named %q has the role %s instead", l.name, strings.Join(others, ", "))
			}
		}
	case locTestID:
		var ids []string
		root.Walk(func(n *semantics.Node) bool {
			if n.Has(semantics.StateHidden) {
				return false
			}
			if id := a.tree.NodeTestID(n.ID); n.Kind == semantics.KindElement && id != "" && !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
			return true
		})
		switch {
		case len(ids) == 0:
			b.WriteString("\nno visible element has an h.TestID")
		default:
			if sug := closest(l.testID, ids); len(sug) > 0 {
				fmt.Fprintf(&b, "\ndid you mean %s?", quoteAll(sug))
			} else {
				fmt.Fprintf(&b, "\ntest ids on screen: %s", quoteAll(ids))
			}
		}
	case locText:
		tc := newTexts()
		var cands []string
		root.Walk(func(n *semantics.Node) bool {
			if n.Has(semantics.StateHidden) {
				return false
			}
			if n.Kind == semantics.KindElement || n.Kind == semantics.KindRoot {
				leaf := true
				for _, ch := range n.Children {
					if ch.Kind == semantics.KindElement {
						leaf = false
					}
				}
				if t := tc.norm(n); leaf && t != "" && len(t) <= 80 {
					cands = append(cands, t)
				}
			}
			return true
		})
		if sug := closest(l.text, cands); len(sug) > 0 {
			fmt.Fprintf(&b, "\ndid you mean %s?", quoteAll(sug))
		}
	}
	return b.String()
}

func quoteAll(ss []string) string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = strconv.Quote(s)
	}
	return strings.Join(out, ", ")
}

func quoteList(role string, ss []string) string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = role + " " + strconv.Quote(s)
	}
	return strings.Join(out, " or ")
}

// closest returns up to three of the candidates closest to want, by
// case-insensitive edit distance or containment, closest first.
func closest(want string, cands []string) []string {
	lw := strings.ToLower(normalize(want))
	type scored struct {
		s string
		d int
	}
	var list []scored
	seen := map[string]bool{}
	for _, c := range cands {
		if seen[c] || c == "" {
			continue
		}
		seen[c] = true
		lc := strings.ToLower(c)
		d := editDistance(lw, lc)
		limit := max(2, len(lw)/3)
		switch {
		case lw == "":
			d = 0
		case strings.Contains(lc, lw) || strings.Contains(lw, lc):
			d = min(d, 1)
		case d > limit:
			continue
		}
		list = append(list, scored{c, d})
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].d < list[j].d })
	var out []string
	for _, s := range list {
		if len(out) == 3 {
			break
		}
		out = append(out, s.s)
	}
	return out
}

// editDistance is the Levenshtein distance of a and b over their runes.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
